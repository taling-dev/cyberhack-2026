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
  import { authState, markRecovered } from '$lib/auth/recover';

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

  // Role-based navigation: each item lists who can see it
  const allNavItems = [
    { href: '/dashboard', icon: '📊', key: 'nav.dashboard', roles: ['OPERATOR', 'QC_SUPERVISOR', 'WAREHOUSE_STAFF', 'MANAGER', 'ADMIN'] },
    { href: '/lots', icon: '📦', key: 'nav.lots', roles: ['OPERATOR', 'QC_SUPERVISOR', 'WAREHOUSE_STAFF', 'MANAGER', 'ADMIN'] },
    { href: '/qc', icon: '🔬', key: 'nav.qc', roles: ['QC_SUPERVISOR', 'MANAGER', 'ADMIN'] },
    { href: '/warehouse', icon: '🏭', key: 'nav.warehouse', roles: ['WAREHOUSE_STAFF', 'MANAGER', 'ADMIN'] },
    { href: '/audit', icon: '📋', key: 'nav.audit', roles: ['MANAGER', 'ADMIN'] },
    { href: '/admin', icon: '⚙️', key: 'nav.admin', roles: ['ADMIN'] }
  ];

  function toggleLocale() {
    locale.set($locale === 'en' ? 'id' : 'en');
  }

  const user = $derived(data.user);
  const userRoles = $derived(user?.roles ?? []);
  const navItems = $derived(
    allNavItems.filter(item => userRoles.some((r: string) => item.roles.includes(r)))
  );

  // ─── Live nav badges ────────────────────────────────────────────
  const lotClient = createClient(LotService, transport);

  // QC review badge — pending QC supervisor decision (lot status QC_REVIEW = 4).
  // Only shown to users who have access to /qc.
  const qcBadgeEnabled = $derived(
    !!user && userRoles.some((r: string) => ['QC_SUPERVISOR', 'MANAGER', 'ADMIN'].includes(r)),
  );
  const qcBadgeQuery = createQuery(() => ({
    queryKey: ['nav-badges', 'qc-review'],
    queryFn: () => lotClient.listLots({ pageSize: 1, statusFilter: 4 }),
    enabled: qcBadgeEnabled,
    staleTime: 30_000,
  }));
  const qcBadgeCount = $derived(qcBadgeQuery.data?.totalCount ?? 0);

  // Warehouse pending badge — lots in QC_APPROVED (=5) waiting to be slotted.
  const whBadgeEnabled = $derived(
    !!user && userRoles.some((r: string) => ['WAREHOUSE_STAFF', 'MANAGER', 'ADMIN'].includes(r)),
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

  // ─── Realtime store ──────────────────────────────────────────────
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

  // Translator wrapper that resolves to a plain string. Uses `get(t)` for
  // a one-shot read instead of subscribing — avoids creating/disposing a
  // subscription on every dispatch (called once per SSE event by the toast
  // dispatcher).
  function translateFn(key: string, opts?: { values?: Record<string, any> }) {
    return get(t)(key, opts);
  }

  onMount(() => {
    if (!browser) return;
    sweepOldDrafts();
  });

  // (Re)open the realtime stream when the user becomes authenticated; close
  // when they log out.
  $effect(() => {
    const u = user;
    untrack(() => {
      // Close any prior handle before opening a new one.
      realtime?.disconnect();
      realtime = null;
      if (!browser || !u) return;
      realtime = connectRealtime(queryClient, {
        onEvent: (e) => {
          dispatchToast(e, {
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
            timeoutMs: 0, // sticky
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
      onEvent: (e) =>
        dispatchToast(e, {
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

<div class="flex h-screen bg-gray-50">
  <!-- Sidebar -->
  <aside class="w-64 bg-gray-900 text-white flex flex-col" aria-label="Main navigation">
    <div class="p-4 border-b border-gray-700">
      <h1 class="text-lg font-bold">{$t('app.name')}</h1>
      <p class="text-xs text-gray-400">{$t('app.tagline')}</p>
    </div>
    <nav class="flex-1 p-2 space-y-1">
      {#if user}
        {#each navItems as item}
          <a
            href={item.href}
            aria-current={$page.url.pathname.startsWith(item.href) ? 'page' : undefined}
            class="flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors
              {$page.url.pathname.startsWith(item.href) ? 'bg-gray-700 text-white' : 'text-gray-300 hover:bg-gray-800 hover:text-white'}"
          >
            <span aria-hidden="true">{item.icon}</span>
            <span class="flex-1">{$t(item.key)}</span>
            {#if badgeFor(item.href) > 0}
              <span
                class="ml-auto text-xs bg-blue-600 text-white rounded-full px-2 py-0.5 min-w-[1.25rem] text-center"
                aria-label="{badgeFor(item.href)} pending"
              >{badgeFor(item.href)}</span>
            {/if}
          </a>
        {/each}
      {:else}
        <p class="px-3 py-2 text-sm text-gray-400">{$t('nav.login_required')}</p>
      {/if}
    </nav>
    {#if user}
      <div class="p-3 border-t border-gray-700 text-xs text-gray-400">
        <div class="font-medium text-gray-200">{user.name}</div>
        <div class="text-gray-500 mt-0.5">
          {#each userRoles as role}
            <span class="inline-block px-1.5 py-0.5 rounded bg-gray-800 mr-1 mb-0.5">{role}</span>
          {/each}
        </div>
      </div>
    {/if}
  </aside>

  <!-- Main content -->
  <div class="flex-1 flex flex-col overflow-hidden">
    <!-- Topbar -->
    <header class="h-14 bg-white border-b border-gray-200 flex items-center justify-between px-6">
      <div class="text-sm text-gray-500">
        {$t('app.name')}
      </div>
      <div class="flex items-center gap-3">
        {#if user && realtimeStatus !== 'idle'}
          <span class="flex items-center gap-1.5 text-xs text-gray-500" aria-live="polite">
            <span
              class="inline-block w-2 h-2 rounded-full {statusDotClass(realtimeStatus)}"
              aria-hidden="true"
            ></span>
            {$t(statusLabelKey(realtimeStatus))}
          </span>
        {/if}
        <button
          onclick={toggleLocale}
          class="text-sm px-3 py-1 rounded border border-gray-300 hover:bg-gray-100 transition-colors"
          aria-label="Switch language"
          title="Switch language"
        >
          {$t(`locale.${$locale === 'en' ? 'id' : 'en'}`)}
        </button>
        {#if user}
          <a href="/auth/logout" class="text-sm px-3 py-1 rounded border border-gray-300 hover:bg-red-50 hover:text-red-700 hover:border-red-300 transition-colors">
            {$t('nav.logout')}
          </a>
        {:else}
          <a href="/auth/login" class="text-sm px-3 py-1 bg-blue-600 text-white rounded hover:bg-blue-700">
            {$t('nav.login')}
          </a>
        {/if}
      </div>
    </header>

    <!-- Page content -->
    <main class="flex-1 overflow-auto p-6">
      {#if !user && !$page.url.pathname.startsWith('/auth/')}
        <div class="max-w-md mx-auto mt-20 text-center space-y-4">
          <div class="text-6xl">🔒</div>
          <h2 class="text-xl font-bold">{$t('auth.required_title')}</h2>
          <p class="text-gray-500">{$t('auth.required_body')}</p>
          <a href="/auth/login" class="inline-block px-6 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
            {$t('nav.login')}
          </a>
        </div>
      {:else}
        {@render children()}
      {/if}
    </main>
  </div>
</div>

<!-- Realtime UX overlays -->
<Toaster />
<SessionExpiredModal
  open={realtimeStatus === 'session-expired' || authState.sessionExpired}
  onSignedIn={() => { markRecovered(); reconnectRealtime(); }}
/>
