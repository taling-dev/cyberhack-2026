<script lang="ts">
  import '../app.css';
  import '$lib/i18n';
  import { t } from 'svelte-i18n';
  import { get } from 'svelte/store';
  import { page } from '$app/stores';
  import { invalidateAll } from '$app/navigation';
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
    { href: '/settings', icon: 'settings', key: 'nav.settings', roles: ['OPERATOR', 'QC_SUPERVISOR', 'WAREHOUSE_STAFF', 'MANAGER', 'ADMIN'] },
    { href: '/admin', icon: 'admin', key: 'nav.admin', roles: ['ADMIN'] },
  ];

  async function stopImpersonating() {
    await fetch('/api/admin/impersonate', { method: 'DELETE' });
    await invalidateAll();
    location.reload();
  }

  const user = $derived(data.user);
  const userRoles = $derived(user?.roles ?? []);
  // Highest-privilege role wins (an admin with extra roles must read as ADMIN, not roles[0]).
  const ROLE_PRIORITY = ['ADMIN', 'MANAGER', 'QC_SUPERVISOR', 'WAREHOUSE_STAFF', 'OPERATOR'];
  const primaryRole = $derived(ROLE_PRIORITY.find((r) => userRoles.includes(r)) ?? userRoles[0] ?? 'USER');
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

  // Cold-chain temperature alerts shown app-wide, but only to roles allowed to
  // view warehouse status (same gate as the warehouse nav/badge). Deduped per
  // zone+status so each transition alerts once, not on every 10s poll.
  const coldChainQuery = createQuery(() => ({
    queryKey: ['coldchain-status'],
    queryFn: async () => {
      const r = await fetch('/api/coldchain');
      if (!r.ok) throw new Error('coldchain');
      return r.json();
    },
    enabled: whBadgeEnabled,
    refetchInterval: 10_000,
  }));
  const coldChain = $derived(coldChainQuery.data?.equipment ?? []);
  const alertedZones = new Map<string, string>();
  $effect(() => {
    for (const eq of coldChain) {
      const status = eq.health?.status ?? 'NO_DATA';
      const key = eq.equipment_id;
      if (status === 'CRITICAL' || status === 'WARNING') {
        if (alertedZones.get(key) === status) continue;
        alertedZones.set(key, status);
        pushToast({
          variant: status === 'CRITICAL' ? 'error' : 'warning',
          title: `${$t('warehouse.zone')} ${key}: ${status === 'CRITICAL' ? $t('coldchain.critical') : $t('coldchain.warning')}`,
          body: eq.latest_alert?.message ?? `${eq.latest_temperature ?? '—'}°C — ${$t('coldchain.out_of_range')}`,
          timeoutMs: status === 'CRITICAL' ? 0 : 6000,
        });
      } else {
        alertedZones.delete(key);
      }
    }
  });

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
  let profileMenuOpen = $state(false);
  let profileButton = $state<HTMLButtonElement | null>(null);
  let profileMenu = $state<HTMLDivElement | null>(null);

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

    function closeProfileMenuOnOutsideClick(event: MouseEvent) {
      const target = event.target as Node;
      if (!profileMenuOpen) return;
      if (profileButton?.contains(target) || profileMenu?.contains(target)) return;
      profileMenuOpen = false;
    }

    document.addEventListener('mousedown', closeProfileMenuOnOutsideClick);
    return () => document.removeEventListener('mousedown', closeProfileMenuOnOutsideClick);
  });

  function closeProfileMenu() {
    profileMenuOpen = false;
  }

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
                {:else if item.icon === 'admin'}
                  <path d="M12 3 20 6v6c0 5-3.3 8-8 9-4.7-1-8-4-8-9V6l8-3Z" />
                  <path d="M9 12h6" />
                  <path d="M12 9v6" />
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
      <div class="relative p-3 md:p-4">
        <button
          bind:this={profileButton}
          type="button"
          class="w-full rounded-full bg-white/[0.08] p-1.5 text-left transition-colors hover:bg-white/[0.12] focus:outline-none focus:ring-2 focus:ring-white/20 md:flex md:items-center md:gap-3 md:rounded-xl md:border md:border-white/10 md:p-3"
          aria-haspopup="menu"
          aria-expanded={profileMenuOpen}
          aria-controls="sidebar-profile-menu"
          onclick={() => profileMenuOpen = !profileMenuOpen}
          onkeydown={(event) => {
            if (event.key === 'Escape') profileMenuOpen = false;
          }}
        >
          <span class="relative mx-auto flex size-10 shrink-0 items-center justify-center rounded-full bg-lime-500 text-sm font-bold text-slate-950 md:mx-0">
            {initials(user.name)}
            <span class="absolute bottom-0 right-0 size-2.5 rounded-full border-2 border-[#071326] bg-green-500"></span>
          </span>
          <span class="hidden min-w-0 flex-1 md:block">
            <span class="block truncate text-sm font-bold text-white">{user.name}</span>
            <span class="mt-0.5 block truncate text-[11px] font-semibold uppercase tracking-normal text-slate-300">
              {primaryRole}
            </span>
            <span class="mt-2 flex items-center gap-1.5 text-[11px] text-slate-300">
              <span class="size-2 rounded-full bg-green-500"></span>
              Online
            </span>
          </span>
          <span class="hidden shrink-0 text-slate-400 md:block" aria-hidden="true">
            <svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="m7 10 5 5 5-5" />
            </svg>
          </span>
        </button>

        {#if profileMenuOpen}
          <div
            bind:this={profileMenu}
            id="sidebar-profile-menu"
            role="menu"
            aria-label="Account menu"
            tabindex="-1"
            class="absolute bottom-full left-3 z-50 mb-2 w-56 overflow-hidden rounded-lg border border-slate-200 bg-white p-1 text-slate-700 shadow-xl md:left-4"
            onkeydown={(event) => {
              if (event.key === 'Escape') {
                profileMenuOpen = false;
                profileButton?.focus();
              }
            }}
          >
            <div class="border-b border-slate-100 px-3 py-2">
              <p class="truncate text-sm font-bold text-slate-950">{user.name}</p>
              <p class="mt-0.5 truncate text-xs text-slate-500">{user.email || user.username}</p>
            </div>
            <div class="py-1">
              <a
                href="/settings"
                role="menuitem"
                onclick={closeProfileMenu}
                class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-slate-50 focus:bg-slate-50 focus:outline-none"
              >
                <svg viewBox="0 0 24 24" class="size-4 text-slate-500" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <circle cx="12" cy="8" r="4" />
                  <path d="M5 21a7 7 0 0 1 14 0" />
                </svg>
                Profile
              </a>
              <a
                href="/settings"
                role="menuitem"
                onclick={closeProfileMenu}
                class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-slate-50 focus:bg-slate-50 focus:outline-none"
              >
                <svg viewBox="0 0 24 24" class="size-4 text-slate-500" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <circle cx="12" cy="12" r="3" />
                  <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.1 2.1-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V20h-3v-.2a1.7 1.7 0 0 0-1-1.5 1.7 1.7 0 0 0-1.9.3l-.1.1L6.6 16.6l.1-.1A1.7 1.7 0 0 0 7 14.6a1.7 1.7 0 0 0-1.5-1H5v-3h.5A1.7 1.7 0 0 0 7 9.5a1.7 1.7 0 0 0-.3-1.9l-.1-.1 2.1-2.1.1.1a1.7 1.7 0 0 0 1.9.3 1.7 1.7 0 0 0 1-1.5V4h3v.2a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.9-.3l.1-.1 2.1 2.1-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.5 1h.5v3h-.5a1.7 1.7 0 0 0-1.5 1Z" />
                </svg>
                {$t('nav.settings')}
              </a>
              <a
                href="/auth/logout"
                role="menuitem"
                class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-semibold text-red-600 transition-colors hover:bg-red-50 focus:bg-red-50 focus:outline-none"
              >
                <svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
                  <path d="M16 17l5-5-5-5" />
                  <path d="M21 12H9" />
                </svg>
                {$t('nav.logout')}
              </a>
            </div>
          </div>
        {/if}
      </div>
    {/if}
  </aside>

  <div class="flex min-w-0 flex-1 flex-col overflow-hidden">
    <main class="flex-1 overflow-auto bg-slate-50 p-4 md:p-6">
      {#if $page.data.impersonating}
        <div class="mb-4 flex flex-col gap-2 rounded-lg border border-amber-300 bg-amber-50 px-4 py-2.5 text-sm sm:flex-row sm:items-center sm:justify-between">
          <span class="font-medium text-amber-900">👁 {$t('impersonate.banner')} <span class="font-mono font-bold">{$page.data.impersonating}</span></span>
          <button onclick={stopImpersonating} class="self-start rounded-md border border-amber-300 bg-white px-3 py-1 text-xs font-semibold text-amber-800 shadow-sm hover:bg-amber-100 sm:self-auto">{$t('impersonate.stop')}</button>
        </div>
      {/if}
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
