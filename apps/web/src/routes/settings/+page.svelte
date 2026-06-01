<script lang="ts">
  import { page } from '$app/stores';
  import { t, locale } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { AdminService } from '$lib/gen/simaops/admin/v1/admin_pb';
  import DashboardIcon from '$lib/components/dashboard/DashboardIcon.svelte';

  const adminClient = createClient(AdminService, transport);

  const roleNames: Record<number, string> = {
    1: 'OPERATOR',
    2: 'QC_SUPERVISOR',
    3: 'WAREHOUSE_STAFF',
    4: 'MANAGER',
    5: 'ADMIN',
  };

  const user = $derived($page.data.user);
  const roles = $derived(user?.roles ?? []);
  const isAdmin = $derived(roles.includes('ADMIN'));
  const primaryRole = $derived(
    ['ADMIN', 'MANAGER', 'QC_SUPERVISOR', 'WAREHOUSE_STAFF', 'OPERATOR'].find((r) => roles.includes(r)) ?? roles[0] ?? 'USER'
  );

  // AI QC thresholds
  const thresholdsQuery = createQuery(() => ({
    queryKey: ['qc-thresholds'],
    queryFn: () => adminClient.getQCThresholds({}),
  }));
  let passMin = $state(75);
  let reviewMin = $state(40);
  let thBusy = $state(false);
  let thError = $state('');
  let thSaved = $state(false);
  // Sync local editable state when the query loads.
  $effect(() => {
    const d = thresholdsQuery.data;
    if (d && !thBusy) { passMin = d.passMin; reviewMin = d.reviewMin; }
  });
  async function saveThresholds(p: number, r: number) {
    thBusy = true; thError = ''; thSaved = false;
    try {
      const res = await adminClient.updateQCThresholds({ passMin: p, reviewMin: r });
      passMin = res.passMin; reviewMin = res.reviewMin; thSaved = true;
    } catch (e: any) { thError = e?.message || $t('settings.th_error'); } finally { thBusy = false; }
  }
  function resetThresholds() {
    const d = thresholdsQuery.data;
    if (d) saveThresholds(d.defaultPassMin, d.defaultReviewMin);
  }

  const usersQuery = createQuery(() => ({
    queryKey: ['admin-users', 'settings'],
    queryFn: () => adminClient.listUsers({ pageSize: 50 }),
    enabled: isAdmin,
  }));

  const rolesQuery = createQuery(() => ({
    queryKey: ['admin-roles', 'settings'],
    queryFn: () => adminClient.listRoles({}),
    enabled: isAdmin,
  }));

  function initials(name?: string | null) {
    return (
      (name ?? '')
        .split(/\s+/)
        .filter(Boolean)
        .slice(0, 2)
        .map((part) => part[0]?.toUpperCase())
        .join('') || 'SO'
    );
  }

  function setLanguage(value: string) {
    locale.set(value);
  }
</script>

<div class="space-y-5">
  <header class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
    <div>
      <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">Workspace Preferences</p>
      <h1 class="mt-1 text-[28px] font-bold tracking-normal text-slate-950">{$t('nav.settings')}</h1>
    </div>

    <div class="flex flex-wrap items-center gap-3">
      <span class="inline-flex h-10 items-center gap-2 rounded-md border border-emerald-200 bg-emerald-50 px-3 text-sm font-semibold text-emerald-700 shadow-sm">
        <span class="size-2 rounded-full bg-emerald-500"></span>
        Session active
      </span>
      <a
        href="/auth/logout"
        class="inline-flex h-10 items-center rounded-md border border-red-200 bg-white px-3 text-sm font-semibold text-red-600 shadow-sm transition-colors hover:bg-red-50"
      >
        {$t('nav.logout')}
      </a>
    </div>
  </header>

  <div class="grid gap-4 xl:grid-cols-[.95fr_1.05fr]">
    <section class="rounded-lg border border-slate-200 bg-white p-5 shadow-sm">
      <div class="flex items-start justify-between gap-4">
        <div class="flex min-w-0 items-center gap-4">
          <div class="relative flex size-14 shrink-0 items-center justify-center rounded-full bg-lime-500 text-lg font-bold text-slate-950">
            {initials(user?.name)}
            <span class="absolute bottom-0 right-0 size-3 rounded-full border-2 border-white bg-emerald-500"></span>
          </div>
          <div class="min-w-0">
            <p class="text-xs font-semibold uppercase tracking-normal text-slate-500">Profile</p>
            <h2 class="mt-1 truncate text-xl font-bold text-slate-950">{user?.name ?? 'SimaOps User'}</h2>
            <p class="mt-1 truncate text-sm text-slate-500">{user?.email || user?.username || 'Authenticated account'}</p>
          </div>
        </div>
        <span class="rounded-md bg-blue-100 px-2.5 py-1 text-xs font-semibold text-blue-700">{primaryRole}</span>
      </div>

      <div class="mt-5 grid gap-3 sm:grid-cols-2">
        <div class="rounded-lg border border-slate-200 bg-slate-50 p-3">
          <p class="text-xs font-semibold uppercase tracking-normal text-slate-500">Username</p>
          <p class="mt-1 truncate font-mono text-sm font-semibold text-slate-950">{user?.username || '-'}</p>
        </div>
        <div class="rounded-lg border border-slate-200 bg-slate-50 p-3">
          <p class="text-xs font-semibold uppercase tracking-normal text-slate-500">User ID</p>
          <p class="mt-1 truncate font-mono text-sm font-semibold text-slate-950" title={user?.sub}>{user?.sub || '-'}</p>
        </div>
      </div>

      <div class="mt-4">
        <p class="text-xs font-semibold uppercase tracking-normal text-slate-500">Assigned roles</p>
        <div class="mt-2 flex flex-wrap gap-2">
          {#each roles as role}
            <span class="rounded-md bg-slate-100 px-2.5 py-1 text-xs font-semibold text-slate-700 ring-1 ring-inset ring-slate-200">{role}</span>
          {:else}
            <span class="text-sm text-slate-500">No role claims found.</span>
          {/each}
        </div>
      </div>
    </section>

    <section class="rounded-lg border border-slate-200 bg-white p-5 shadow-sm">
      <div class="flex items-start justify-between gap-3">
        <div>
          <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">Language</p>
          <h2 class="mt-1 text-lg font-bold text-slate-950">Interface language</h2>
          <p class="mt-1 text-sm text-slate-500">Choose the language used across SARI.</p>
        </div>
        <DashboardIcon name="globe" class="size-5 text-slate-400" />
      </div>

      <div class="mt-5 grid gap-3 sm:grid-cols-2">
        <button
          type="button"
          onclick={() => setLanguage('en')}
          class="rounded-lg border p-4 text-left shadow-sm transition-colors {$locale === 'en' ? 'border-blue-300 bg-blue-50 ring-2 ring-blue-100' : 'border-slate-200 bg-white hover:bg-slate-50'}"
          aria-pressed={$locale === 'en'}
        >
          <span class="text-sm font-bold text-slate-950">English</span>
          <span class="mt-1 block text-xs text-slate-500">Primary operational language</span>
        </button>
        <button
          type="button"
          onclick={() => setLanguage('id')}
          class="rounded-lg border p-4 text-left shadow-sm transition-colors {$locale === 'id' ? 'border-blue-300 bg-blue-50 ring-2 ring-blue-100' : 'border-slate-200 bg-white hover:bg-slate-50'}"
          aria-pressed={$locale === 'id'}
        >
          <span class="text-sm font-bold text-slate-950">Bahasa Indonesia</span>
          <span class="mt-1 block text-xs text-slate-500">Localized app labels</span>
        </button>
      </div>

      <p class="mt-4 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-500">
        The current language is saved in the existing browser cookie and reused on future visits.
      </p>
    </section>
  </div>

  <div class="grid gap-4 xl:grid-cols-3">
    <section class="rounded-lg border border-slate-200 bg-white p-5 shadow-sm">
      <div class="flex items-start justify-between gap-3">
        <div>
          <p class="text-xs font-semibold uppercase tracking-normal text-emerald-600">Session</p>
          <h2 class="mt-1 text-lg font-bold text-slate-950">Authentication</h2>
        </div>
        <DashboardIcon name="shield" class="size-5 text-slate-400" />
      </div>

      <div class="mt-5 space-y-3">
        <div class="flex items-center justify-between rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2">
          <span class="text-sm font-semibold text-emerald-800">Signed in</span>
          <span class="size-2 rounded-full bg-emerald-500"></span>
        </div>
        <p class="text-sm leading-6 text-slate-600">
          Your session refreshes automatically. If it expires, you'll be prompted to sign in again without losing your work.
        </p>
      </div>
    </section>

    <section class="rounded-lg border border-slate-200 bg-white p-5 shadow-sm">
      <div class="flex items-start justify-between gap-3">
        <div>
          <p class="text-xs font-semibold uppercase tracking-normal text-purple-600">Notifications</p>
          <h2 class="mt-1 text-lg font-bold text-slate-950">Realtime alerts</h2>
        </div>
        <DashboardIcon name="bell" class="size-5 text-slate-400" />
      </div>

      <div class="mt-5 space-y-2 text-sm text-slate-600">
        <div class="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2">You receive QC, warehouse, production, and dispatch alerts relevant to your role.</div>
      </div>
    </section>

    <section class="rounded-lg border border-slate-200 bg-white p-5 shadow-sm">
      <div class="flex items-start justify-between gap-3">
        <div>
          <p class="text-xs font-semibold uppercase tracking-normal text-orange-600">AI/QC</p>
          <h2 class="mt-1 text-lg font-bold text-slate-950">Thresholds</h2>
        </div>
        <DashboardIcon name="bot" class="size-5 text-slate-400" />
      </div>

      <div class="mt-5 space-y-4">
        <p class="text-xs text-slate-500">{$t('settings.th_hint')}</p>

        <label class="block">
          <span class="flex items-center justify-between text-sm font-medium text-slate-700">
            {$t('settings.th_pass_min')} <span class="font-mono text-slate-950">{passMin}</span>
          </span>
          <input type="range" min="1" max="100" bind:value={passMin} disabled={!isAdmin || thBusy} class="mt-1 w-full accent-emerald-600 disabled:opacity-50" />
        </label>

        <label class="block">
          <span class="flex items-center justify-between text-sm font-medium text-slate-700">
            {$t('settings.th_review_min')} <span class="font-mono text-slate-950">{reviewMin}</span>
          </span>
          <input type="range" min="1" max="100" bind:value={reviewMin} disabled={!isAdmin || thBusy} class="mt-1 w-full accent-orange-500 disabled:opacity-50" />
        </label>

        <div class="rounded-md bg-slate-50 p-3 text-xs text-slate-600">
          <span class="font-semibold text-emerald-700">PASS</span> ≥ {passMin} ·
          <span class="font-semibold text-orange-600">REVIEW</span> ≥ {reviewMin} ·
          <span class="font-semibold text-red-600">FAIL</span> &lt; {reviewMin}
        </div>

        {#if thError}<p class="text-sm text-red-600" role="alert">{thError}</p>{/if}
        {#if thSaved}<p class="text-sm text-emerald-700" role="status">{$t('settings.th_saved')}</p>{/if}

        {#if isAdmin}
          <div class="flex gap-2">
            <button onclick={() => saveThresholds(passMin, reviewMin)} disabled={thBusy || reviewMin >= passMin} class="h-9 rounded-md bg-blue-600 px-4 text-sm font-semibold text-white shadow-sm hover:bg-blue-700 disabled:opacity-50">{thBusy ? $t('admin.saving') : $t('settings.th_save')}</button>
            <button onclick={resetThresholds} disabled={thBusy} class="h-9 rounded-md border border-slate-200 bg-white px-4 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50 disabled:opacity-50">{$t('settings.th_reset')}</button>
          </div>
          {#if reviewMin >= passMin}<p class="text-xs text-red-600">{$t('settings.th_order')}</p>{/if}
        {:else}
          <p class="text-xs text-slate-400">{$t('settings.th_admin_only')}</p>
        {/if}
      </div>
    </section>
  </div>

  <section class="rounded-lg border border-slate-200 bg-white p-5 shadow-sm">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
      <div>
        <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">Roles & Access</p>
        <h2 class="mt-1 text-lg font-bold text-slate-950">Access management</h2>
        <p class="mt-1 text-sm text-slate-500">
          {isAdmin ? 'Admin users can inspect existing role and user data from the current AdminService.' : 'Your roles are managed by an administrator.'}
        </p>
      </div>
      <span class="rounded-md bg-slate-100 px-2.5 py-1 text-xs font-semibold text-slate-700">
        {isAdmin ? 'Admin view' : 'Read-only'}
      </span>
    </div>

    {#if isAdmin}
      <div class="mt-5 grid gap-4 xl:grid-cols-[1.1fr_.9fr]">
        <div class="overflow-hidden rounded-lg border border-slate-200">
          <div class="flex items-center justify-between border-b border-slate-200 bg-slate-50 px-4 py-3">
            <h3 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">Users</h3>
            <span class="rounded-md bg-blue-100 px-2.5 py-1 text-xs font-semibold text-blue-700">{usersQuery.data?.totalCount ?? 0} total</span>
          </div>

          {#if usersQuery.isLoading}
            <div class="divide-y divide-slate-100">
              {#each [1, 2, 3, 4] as _}
                <div class="grid grid-cols-[1fr_1fr_.7fr] gap-4 px-4 py-4">
                  <div class="h-4 animate-pulse rounded bg-slate-100"></div>
                  <div class="h-4 animate-pulse rounded bg-slate-100"></div>
                  <div class="h-4 animate-pulse rounded bg-slate-100"></div>
                </div>
              {/each}
            </div>
          {:else if usersQuery.isError}
            <div class="m-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
              {usersQuery.error?.message || $t('common.error')}
            </div>
          {:else}
            <div class="overflow-x-auto">
              <table class="w-full min-w-[720px] text-left text-sm">
                <thead class="border-b border-slate-200 bg-slate-50 text-xs font-semibold text-slate-500">
                  <tr>
                    <th class="px-4 py-3">User</th>
                    <th class="px-4 py-3">Roles</th>
                    <th class="px-4 py-3">Status</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-slate-100">
                  {#each usersQuery.data?.users ?? [] as account}
                    <tr class="hover:bg-slate-50">
                      <td class="px-4 py-3">
                        <div class="font-semibold text-slate-950">{account.fullName || account.username}</div>
                        <div class="mt-0.5 text-xs text-slate-500">{account.email || account.username}</div>
                      </td>
                      <td class="px-4 py-3">
                        <div class="flex flex-wrap gap-1.5">
                          {#each account.roles ?? [] as role}
                            <span class="rounded-md bg-blue-100 px-2 py-0.5 text-xs font-semibold text-blue-700">{roleNames[role] ?? role}</span>
                          {/each}
                        </div>
                      </td>
                      <td class="px-4 py-3">
                        <span class="rounded-md px-2.5 py-1 text-xs font-semibold {account.active ? 'bg-emerald-100 text-emerald-700' : 'bg-red-100 text-red-700'}">
                          {account.active ? $t('admin.active') : $t('admin.inactive')}
                        </span>
                      </td>
                    </tr>
                  {:else}
                    <tr>
                      <td colspan="3" class="px-4 py-12 text-center text-sm text-slate-500">No users returned by AdminService.</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>

        <div class="rounded-lg border border-slate-200 p-4">
          <h3 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">Role definitions</h3>

          {#if rolesQuery.isLoading}
            <div class="mt-4 space-y-3">
              {#each [1, 2, 3, 4] as _}
                <div class="h-14 animate-pulse rounded-lg bg-slate-100"></div>
              {/each}
            </div>
          {:else if rolesQuery.isError}
            <div class="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
              {rolesQuery.error?.message || $t('common.error')}
            </div>
          {:else}
            <div class="mt-4 space-y-3">
              {#each rolesQuery.data?.roles ?? [] as role}
                <article class="rounded-lg border border-slate-200 bg-slate-50 p-3">
                  <p class="text-sm font-bold text-slate-950">{role.name}</p>
                  <p class="mt-1 text-xs leading-5 text-slate-500">{role.description}</p>
                </article>
              {:else}
                <p class="text-sm text-slate-500">No role definitions returned.</p>
              {/each}
            </div>
          {/if}
        </div>
      </div>
    {:else}
      <div class="mt-5 rounded-lg border border-slate-200 bg-slate-50 p-4">
        <div class="flex flex-wrap gap-2">
          {#each roles as role}
            <span class="rounded-md bg-blue-100 px-2.5 py-1 text-xs font-semibold text-blue-700">{role}</span>
          {/each}
        </div>
        <p class="mt-3 text-sm text-slate-500">Contact an administrator if your operational role needs to change.</p>
      </div>
    {/if}
  </section>
</div>

