<script lang="ts">
  import '../app.css';
  import '$lib/i18n';
  import { t, locale } from 'svelte-i18n';
  import { page } from '$app/stores';

  let { children } = $props();

  const navItems = [
    { href: '/dashboard', icon: '📊', key: 'nav.dashboard' },
    { href: '/lots', icon: '📦', key: 'nav.lots' },
    { href: '/qc', icon: '🔬', key: 'nav.qc' },
    { href: '/warehouse', icon: '🏭', key: 'nav.warehouse' },
    { href: '/audit', icon: '📋', key: 'nav.audit' },
    { href: '/admin', icon: '⚙️', key: 'nav.admin' }
  ];

  function toggleLocale() {
    locale.set($locale === 'en' ? 'id' : 'en');
  }
</script>

<div class="flex h-screen bg-gray-50">
  <!-- Sidebar -->
  <aside class="w-64 bg-gray-900 text-white flex flex-col">
    <div class="p-4 border-b border-gray-700">
      <h1 class="text-lg font-bold">{$t('app.name')}</h1>
      <p class="text-xs text-gray-400">{$t('app.tagline')}</p>
    </div>
    <nav class="flex-1 p-2 space-y-1">
      {#each navItems as item}
        <a
          href={item.href}
          class="flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors
            {$page.url.pathname.startsWith(item.href) ? 'bg-gray-700 text-white' : 'text-gray-300 hover:bg-gray-800 hover:text-white'}"
        >
          <span>{item.icon}</span>
          <span>{$t(item.key)}</span>
        </a>
      {/each}
    </nav>
  </aside>

  <!-- Main content -->
  <div class="flex-1 flex flex-col overflow-hidden">
    <!-- Topbar -->
    <header class="h-14 bg-white border-b border-gray-200 flex items-center justify-between px-6">
      <div class="text-sm text-gray-500">
        {$t('app.name')}
      </div>
      <div class="flex items-center gap-4">
        <!-- Locale switcher -->
        <button
          onclick={toggleLocale}
          class="text-sm px-3 py-1 rounded border border-gray-300 hover:bg-gray-100 transition-colors"
        >
          {$t(`locale.${$locale === 'en' ? 'id' : 'en'}`)}
        </button>
        <!-- User placeholder -->
        <div class="w-8 h-8 rounded-full bg-gray-300 flex items-center justify-center text-xs font-medium">
          ?
        </div>
      </div>
    </header>

    <!-- Page content -->
    <main class="flex-1 overflow-auto p-6">
      {@render children()}
    </main>
  </div>
</div>
