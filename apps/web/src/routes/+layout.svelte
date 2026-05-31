<script lang="ts">
  import '../app.css';
  import '$lib/i18n';
  import { t, locale } from 'svelte-i18n';
  import { get } from 'svelte/store';
  import { page } from '$app/stores';
  import { browser } from '$app/environment';
  import { onMount, onDestroy, untrack } from 'svelte';
  import { QueryClient, createQuery } from '@tanstack/svelte-query';
  import { setQueryClientContext } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import Toaster, { pushToast } from '$lib/components/Toaster.svelte';
  import SessionExpiredModal from '$lib/components/SessionExpiredModal.svelte';
  import { connectRealtime, type RealtimeHandle } from '$lib/realtime.svelte';
  import { dispatchToast } from '$lib/realtime/toastDispatch';
  import { sweepOldDrafts } from '$lib/forms/draft.svelte';
  import { authState, markRecovered } from '$lib/auth/recover.svelte';

  let { children, data } = $props();

  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: 1,
        refetchOnWindowFocus: false,
        staleTime: 30_000,
      },
    },
  });
  setQueryClientContext(queryClient);

  const allNavItems = [
    { href: '/dashboard', icon: 'dashboard', key: 'nav.dashboard', roles: ['OPERATOR', 'QC_SUPERVISOR', 'WAREHOUSE_STAFF', 'MANAGER', 'ADMIN'] },
    { href: '/lots', icon: 'lots', key: 'nav.lots', roles: ['OPERATOR', 'QC_SUPERVISOR', 'WAREHOUSE_STAFF', 'MANAGER', 'ADMIN'] },
    { href: '/qc', icon: 'qc', key: 'nav.qc', roles: ['QC_SUPERVISOR', 'MANAGER', 'ADMIN'] },
    { href: '/warehouse', icon: 'warehouse', key: 'nav.warehouse', roles: ['WAREHOUSE_STAFF', 'MANAGER', 'ADMIN'] },
    { href: '/dispatch', icon: 'dispatch', key: 'nav.dispatch', roles: ['WAREHOUSE_STAFF', 'MANAGER', 'ADMIN'] },
    { href: '/audit', icon: 'audit', key: 'nav.audit', roles: ['MANAGER', 'ADMIN'] },
    { href: '/reports', icon: 'reports', key: 'nav.reports', roles: ['MANAGER', 'ADMIN'] },
    { href: '/admin', icon: 'settings', key: 'nav.admin', roles: ['ADMIN'] },
  ];

  function toggleLocale() {
    locale.set($locale === 'en' ? 'id' : 'en');
  }

  const user = $derived(data.user);
  const userRoles = $derived(user?.roles ?? []);
  const navItems = $derived(
    allNavItems.filter((item) => userRoles.some((role: string) => item.roles.includes(role))),
  );

  const lotClient = createClient(LotService, transport);

  const qcBadgeEnabled = $derived(
    !!user && userRoles.some((role: string) => ['QC_SUPERVISOR', 'MANAGER', 'ADMIN'].includes(role)),
  );
  const qcBadgeQuery = createQuery(() => ({
    queryKey: ['nav-badges', 'qc-review'],
    queryFn: () => lotClient.listLots({ pageSize: 1, statusFilter: 4 }),
    enabled: qcBadgeEnabled,
    staleTime: 30_000,
  }));
  const qcBadgeCount = $derived(qcBadgeQuery.data?.totalCount ?? 0);

  const whBadgeEnabled = $derived(
    !!user && userRoles.some((role: string) => ['WAREHOUSE_STAFF', 'MANAGER', 'ADMIN'].includes(role)),
  );
  const whBadgeQuery = createQuery(() => ({
    queryKey: ['nav-badges', 'warehouse-pending'],
    queryFn: () => lotClient.listLots({ pageSize: 1, statusFilter: 5 }),
    enabled: whBadgeEnabled,
    staleTime: 30_000,
  }));
  const whBadgeCount = $derived(whBadgeQuery.data?.totalCount ?? 0);

  function badgeFor(href: string): number {
    if (href === '/qc') return qcBadgeCount;
    if (href === '/warehouse') return whBadgeCount;
    return 0;
  }

  function isActive(href: string): boolean {
    return $page.url.pathname.startsWith(href);
  }

  let realtime = $state<RealtimeHandle | null>(null);
  let realtimeStatus = $derived(realtime?.state.status ?? 'idle');

  function statusLabelKey(status: string): string {
    switch (status) {
      case 'live':
        return 'realtime.live';
      case 'reconnecting':
      case 'connecting':
        return 'realtime.reconnecting';
      case 're-authenticating':
        return 'realtime.re_authenticating';
      case 'session-expired':
        return 'realtime.session_expired';
      default:
        return 'realtime.live';
    }
  }

  function statusDotClass(status: string): string {
    switch (status) {
      case 'live':
        return 'bg-green-500';
      case 'reconnecting':
      case 'connecting':
        return 'bg-blue-500 animate-pulse';
      case 're-authenticating':
        return 'bg-amber-500 animate-pulse';
      case 'session-expired':
        return 'bg-red-500';
      default:
        return 'bg-gray-400';
    }
  }

  function translateFn(key: string, opts?: { values?: Record<string, any> }) {
    return get(t)(key, opts);
  }

  function initials(name: string): string {
    return (
      name
        .split(/\s+/)
        .filter(Boolean)
        .slice(0, 2)
        .map((part) => part[0]?.toUpperCase())
        .join('') || 'SO'
    );
  }

  onMount(() => {
    if (!browser) return;
    sweepOldDrafts();
  });

  $effect(() => {
    const u = user;
    untrack(() => {
      realtime?.disconnect();
      realtime = null;
      if (!browser || !u) return;
      realtime = connectRealtime(queryClient, {
        onEvent: (event) => {
          dispatchToast(event, {
            userSub: u.sub,
            roles: u.roles ?? [],
            t: translateFn,
          });
        },
        onSessionEnding: () => {
          pushToast({
            title: translateFn('auth.session_ending_soon.title'),
            body: translateFn('auth.session_ending_soon.body'),
            variant: 'warning',
            timeoutMs: 0,
          });
        },
      });
    });
  });

  onDestroy(() => {
    realtime?.disconnect();
    realtime = null;
  });

  function reconnectRealtime() {
    if (!user || !browser) return;
    realtime?.disconnect();
    realtime = connectRealtime(queryClient, {
      onEvent: (event) =>
        dispatchToast(event, {
          userSub: user.sub,
          roles: user.roles ?? [],
          t: translateFn,
        }),
      onSessionEnding: () => {
        pushToast({
          title: translateFn('auth.session_ending_soon.title'),
          body: translateFn('auth.session_ending_soon.body'),
          variant: 'warning',
          timeoutMs: 0,
        });
      },
    });
  }
</script>

<svelte:head>
  <title>{$t('app.name')}</title>
</svelte:head>

<div class="flex h-screen bg-slate-50 text-slate-950">
  <aside
    class="flex w-[76px] shrink-0 flex-col bg-[linear-gradient(180deg,#06162d_0%,#071326_54%,#081226_100%)] text-white shadow-xl md:w-[240px]"
    aria-label="Main navigation"
  >
    <div class="px-3 pb-5 pt-8 md:px-5">
      <div class="flex items-center justify-center gap-3 md:justify-start">
        <span class="flex size-9 shrink-0 items-center justify-center text-green-500">
          <svg viewBox="0 0 40 40" class="size-9" fill="currentColor" aria-hidden="true">
            <path d="M34.7 7.2C21.2 8.1 10.5 14.4 5.3 25.4c4.1-2.8 8.6-4.2 13.3-4.2-4.2 2.4-7.4 6.1-9.5 11.1 12.7-1.4 21.7-10.4 25.6-25.1Z" />
          </svg>
        </span>
        <div class="hidden min-w-0 md:block">
          <h1 class="truncate text-[22px] font-bold tracking-normal">{$t('app.name')}</h1>
          <p class="mt-0.5 truncate text-xs text-slate-400">{$t('app.tagline')}</p>
        </div>
      </div>
      <div class="mt-5 hidden h-px bg-white/20 md:block"></div>
    </div>

    <nav class="flex-1 space-y-1 px-3 md:px-3.5">
      {#if user}
        {#each navItems as item}
          <a
            href={item.href}
            aria-current={isActive(item.href) ? 'page' : undefined}
            class="group relative flex h-12 items-center justify-center gap-3 rounded px-3 text-sm font-medium transition-colors md:justify-start
              {isActive(item.href) ? 'bg-indigo-700 text-white shadow-sm shadow-indigo-950/30' : 'text-slate-200 hover:bg-white/10 hover:text-white'}"
          >
            <span class="flex size-6 shrink-0 items-center justify-center" aria-hidden="true">
              <svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                {#if item.icon === 'dashboard'}
                  <path d="M4 4h7v7H4z" />
                  <path d="M13 4h7v4h-7z" />
                  <path d="M13 10h7v10h-7z" />
                  <path d="M4 13h7v7H4z" />
                {:else if item.icon === 'lots'}
                  <path d="m12 3 8 4.5v9L12 21l-8-4.5v-9L12 3Z" />
                  <path d="m4 7.5 8 4.5 8-4.5" />
                  <path d="M12 12v9" />
                {:else if item.icon === 'qc'}
                  <path d="M9 3v6l-4.5 8A3 3 0 0 0 7.1 21h9.8a3 3 0 0 0 2.6-4L15 9V3" />
                  <path d="M8 3h8" />
                  <path d="M7 15h10" />
                {:else if item.icon === 'warehouse'}
                  <path d="M3 10.5 12 5l9 5.5" />
                  <path d="M5 10v9h14v-9" />
                  <path d="M9 19v-6h6v6" />
                {:else if item.icon === 'dispatch'}
                  <path d="M3 7h11v9H3z" />
                  <path d="M14 10h3l4 4v2h-7z" />
                  <circle cx="7" cy="18" r="2" />
                  <circle cx="17" cy="18" r="2" />
                {:else if item.icon === 'audit'}
                  <path d="M8 4h8" />
                  <path d="M9 2h6v4H9z" />
                  <path d="M6 4H5a2 2 0 0 0-2 2v14h18V6a2 2 0 0 0-2-2h-1" />
                  <path d="M8 12h8" />
                  <path d="M8 16h5" />
                {:else if item.icon === 'reports'}
                  <path d="M4 19V5" />
                  <path d="M4 19h16" />
                  <path d="M8 15v-4" />
                  <path d="M12 15V8" />
                  <path d="M16 15v-7" />
                  <path d="M8 11l4-3 4 1" />
                {:else if item.icon === 'settings'}
                  <circle cx="12" cy="12" r="3" />
                  <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.1 2.1-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V20h-3v-.2a1.7 1.7 0 0 0-1-1.5 1.7 1.7 0 0 0-1.9.3l-.1.1L6.6 16.6l.1-.1A1.7 1.7 0 0 0 7 14.6a1.7 1.7 0 0 0-1.5-1H5v-3h.5A1.7 1.7 0 0 0 7 9.5a1.7 1.7 0 0 0-.3-1.9l-.1-.1 2.1-2.1.1.1a1.7 1.7 0 0 0 1.9.3 1.7 1.7 0 0 0 1-1.5V4h3v.2a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.9-.3l.1-.1 2.1 2.1-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.5 1h.5v3h-.5a1.7 1.7 0 0 0-1.5 1Z" />
                {/if}
              </svg>
            </span>
            <span class="hidden min-w-0 flex-1 truncate md:block">{$t(item.key)}</span>
            {#if badgeFor(item.href) > 0}
              <span
                class="absolute right-1 top-1 min-w-[1.35rem] rounded-full bg-orange-500 px-1.5 py-0.5 text-center text-[11px] font-bold leading-none text-white md:static md:ml-auto"
                aria-label="{badgeFor(item.href)} pending"
              >
                {badgeFor(item.href)}
              </span>
            {/if}
          </a>
        {/each}
      {:else}
        <p class="hidden px-3 py-2 text-sm text-slate-400 md:block">{$t('nav.login_required')}</p>
      {/if}
    </nav>

    {#if user}
      <div class="p-3 md:p-4">
        <div class="rounded-full bg-white/[0.08] p-1.5 md:flex md:items-center md:gap-3 md:rounded-xl md:border md:border-white/10 md:p-3">
          <div class="relative mx-auto flex size-10 shrink-0 items-center justify-center rounded-full bg-lime-500 text-sm font-bold text-slate-950 md:mx-0">
            {initials(user.name)}
            <span class="absolute bottom-0 right-0 size-2.5 rounded-full border-2 border-[#071326] bg-green-500"></span>
          </div>
          <div class="hidden min-w-0 flex-1 md:block">
            <div class="truncate text-sm font-bold text-white">{user.name}</div>
            <div class="mt-0.5 truncate text-[11px] font-semibold uppercase tracking-normal text-slate-300">
              {userRoles[0] ?? 'USER'}
            </div>
            <div class="mt-2 flex items-center gap-1.5 text-[11px] text-slate-300">
              <span class="size-2 rounded-full bg-green-500"></span>
              Online
            </div>
          </div>
        </div>
      </div>
    {/if}
  </aside>

  <div class="flex min-w-0 flex-1 flex-col overflow-hidden">
    <header class="flex h-16 shrink-0 items-center justify-between border-b border-slate-200 bg-white px-4 shadow-sm md:px-6">
      <div class="min-w-0 truncate text-sm font-medium text-slate-500">
        {$t('app.name')}
      </div>
      <div class="flex min-w-0 items-center gap-2 md:gap-3">
        {#if user && realtimeStatus !== 'idle'}
          <span class="hidden items-center gap-1.5 text-xs font-medium text-slate-500 sm:flex" aria-live="polite">
            <span class="inline-block size-2 rounded-full {statusDotClass(realtimeStatus)}" aria-hidden="true"></span>
            {$t(statusLabelKey(realtimeStatus))}
          </span>
        {/if}
        <button
          onclick={toggleLocale}
          class="h-9 rounded-md border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700 shadow-sm transition-colors hover:bg-slate-50"
          aria-label="Switch language"
          title="Switch language"
        >
          {$t(`locale.${$locale === 'en' ? 'id' : 'en'}`)}
        </button>
        {#if user}
          <a href="/auth/logout" class="h-9 rounded-md border border-slate-200 bg-white px-3 py-2 text-sm font-medium leading-5 text-slate-700 shadow-sm transition-colors hover:border-red-200 hover:bg-red-50 hover:text-red-700">
            {$t('nav.logout')}
          </a>
        {:else}
          <a href="/auth/login" class="h-9 rounded-md bg-blue-600 px-3 py-2 text-sm font-medium leading-5 text-white shadow-sm transition-colors hover:bg-blue-700">
            {$t('nav.login')}
          </a>
        {/if}
      </div>
    </header>

    <main class="flex-1 overflow-auto bg-slate-50 p-4 md:p-6">
      {#if !user && !$page.url.pathname.startsWith('/auth/')}
        <div class="mx-auto mt-20 max-w-md space-y-4 text-center">
          <div class="text-5xl text-slate-400" aria-hidden="true">
            <svg viewBox="0 0 24 24" class="mx-auto size-14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect x="5" y="11" width="14" height="10" rx="2" />
              <path d="M8 11V8a4 4 0 1 1 8 0v3" />
            </svg>
          </div>
          <h2 class="text-xl font-bold">{$t('auth.required_title')}</h2>
          <p class="text-slate-500">{$t('auth.required_body')}</p>
          <a href="/auth/login" class="inline-block rounded-md bg-blue-600 px-6 py-2 text-white hover:bg-blue-700">
            {$t('nav.login')}
          </a>
        </div>
      {:else}
        {@render children()}
      {/if}
    </main>
  </div>
</div>

<Toaster />
<SessionExpiredModal
  open={realtimeStatus === 'session-expired' || authState.sessionExpired}
  onSignedIn={() => {
    markRecovered();
    reconnectRealtime();
  }}
/>
