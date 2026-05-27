<script lang="ts">
  import '../app.css';
  import '$lib/i18n';
  import { t, locale } from 'svelte-i18n';
  import { page } from '$app/stores';
  import { QueryClient } from '@tanstack/svelte-query';
  import { setQueryClientContext } from '@tanstack/svelte-query';

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
            <span>{$t(item.key)}</span>
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
