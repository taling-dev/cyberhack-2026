<script lang="ts">
  import { page } from '$app/stores';
  import { t } from 'svelte-i18n';
  import { createQuery, getQueryClientContext } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { AdminService } from '$lib/gen/simaops/admin/v1/admin_pb';
  import { focusTrap } from '$lib/actions/focusTrap.svelte';

  const client = createClient(AdminService, transport);
  const queryClient = getQueryClientContext();

  // Role enum values 1..5 (proto Role). ADMIN/MANAGER carry the most authority.
  const ALL_ROLES = [1, 2, 3, 4, 5] as const;
  const roleNames: Record<number, string> = {
    1: 'OPERATOR', 2: 'QC_SUPERVISOR', 3: 'WAREHOUSE_STAFF', 4: 'MANAGER', 5: 'ADMIN'
  };
  const roleTone: Record<number, string> = {
    1: 'bg-blue-100 text-blue-700 ring-blue-200',
    2: 'bg-orange-100 text-orange-700 ring-orange-200',
    3: 'bg-purple-100 text-purple-700 ring-purple-200',
    4: 'bg-emerald-100 text-emerald-700 ring-emerald-200',
    5: 'bg-red-100 text-red-700 ring-red-200'
  };

  let roleFilter = $state(0); // 0 = all

  const usersQuery = createQuery(() => ({
    queryKey: ['admin-users', roleFilter],
    queryFn: () => client.listUsers({ pageSize: 50, roleFilter }),
  }));

  const rolesQuery = createQuery(() => ({
    queryKey: ['admin-roles'],
    queryFn: () => client.listRoles({}),
  }));

  const currentUsername = $derived($page.data.user?.username ?? '');

  // ── Manage-roles modal ────────────────────────────────────────────
  type AdminUser = { id: string; username: string; fullName: string; email: string; roles: number[]; active: boolean };
  let editing = $state<AdminUser | null>(null);
  let working = $state<number | null>(null); // role currently being toggled
  let modalError = $state('');

  function openManage(user: AdminUser) {
    editing = { ...user, roles: [...(user.roles ?? [])] };
    modalError = '';
  }

  const isSelf = $derived(editing?.username === currentUsername);

  async function toggleRole(role: number, has: boolean) {
    if (!editing) return;
    // Guard: don't let an admin strip their own ADMIN role (lock-out risk),
    // and don't remove a user's last remaining role.
    if (has) {
      if (isSelf && role === 5) { modalError = $t('admin.guard_self_admin'); return; }
      if (editing.roles.length <= 1) { modalError = $t('admin.guard_last_role'); return; }
    }
    working = role;
    modalError = '';
    try {
      if (has) {
        await client.revokeRole({ userId: editing.id, role, idempotencyKey: crypto.randomUUID() });
        editing.roles = editing.roles.filter((r) => r !== role);
      } else {
        await client.assignRole({ userId: editing.id, role, idempotencyKey: crypto.randomUUID() });
        editing.roles = [...editing.roles, role];
      }
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
    } catch {
      modalError = $t('admin.role_update_failed');
    } finally {
      working = null;
    }
  }
</script>

<div class="space-y-5">
  <header class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
    <div>
      <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">{$t('admin.eyebrow')}</p>
      <h1 class="mt-1 text-[28px] font-bold tracking-normal text-slate-950">{$t('nav.admin')}</h1>
      <p class="mt-1 text-sm text-slate-600">{$t('admin.subtitle')}</p>
    </div>
    <label class="flex items-center gap-2 text-sm">
      <span class="text-slate-500">{$t('admin.filter_by_role')}</span>
      <select
        bind:value={roleFilter}
        class="h-9 rounded-md border border-slate-200 bg-white px-2 text-sm text-slate-950 shadow-sm focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-100"
      >
        <option value={0}>{$t('admin.all_roles')}</option>
        {#each ALL_ROLES as r}
          <option value={r}>{roleNames[r]}</option>
        {/each}
      </select>
    </label>
  </header>

  <!-- Users -->
  <section class="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
    <div class="flex items-center justify-between border-b border-slate-200 px-4 py-3">
      <div>
        <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">{$t('admin.users_label')}</h2>
        <p class="mt-1 text-xs text-slate-500">{$t('admin.users_hint')}</p>
      </div>
      <span class="rounded-md bg-slate-100 px-2.5 py-1 text-xs font-semibold text-slate-700">{usersQuery.data?.totalCount ?? 0}</span>
    </div>

    {#if usersQuery.isLoading}
      <div class="divide-y divide-slate-100">
        {#each [1, 2, 3, 4] as _}
          <div class="grid grid-cols-[1fr_1fr_1.5fr_.6fr] gap-4 px-4 py-4">
            {#each [1, 2, 3, 4] as __}
              <div class="h-4 animate-pulse rounded bg-slate-100"></div>
            {/each}
          </div>
        {/each}
      </div>
    {:else if usersQuery.isError}
      <div class="m-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
        {usersQuery.error?.message || $t('common.error')}
      </div>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full min-w-[820px] text-left text-sm">
          <thead class="border-b border-slate-200 bg-slate-50 text-xs font-semibold text-slate-500">
            <tr>
              <th class="px-4 py-3">{$t('admin.username')}</th>
              <th class="px-4 py-3">{$t('admin.email')}</th>
              <th class="px-4 py-3">{$t('admin.roles_label')}</th>
              <th class="px-4 py-3">{$t('common.status')}</th>
              <th class="px-4 py-3 text-right">{$t('common.actions')}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            {#each usersQuery.data?.users ?? [] as user}
              <tr class="transition-colors hover:bg-slate-50">
                <td class="px-4 py-3 align-middle">
                  <div class="font-medium text-slate-950">{user.fullName || user.username}</div>
                  <div class="mt-0.5 font-mono text-xs text-slate-500">{user.username}</div>
                </td>
                <td class="px-4 py-3 align-middle text-slate-700">{user.email}</td>
                <td class="px-4 py-3 align-middle">
                  <div class="flex flex-wrap gap-1">
                    {#each user.roles ?? [] as role}
                      <span class="inline-flex rounded-md px-2 py-0.5 text-xs font-semibold ring-1 ring-inset {roleTone[role] ?? 'bg-slate-100 text-slate-700 ring-slate-200'}">{roleNames[role] ?? role}</span>
                    {:else}
                      <span class="text-xs text-slate-400">{$t('admin.no_roles')}</span>
                    {/each}
                  </div>
                </td>
                <td class="px-4 py-3 align-middle">
                  <span class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-semibold ring-1 ring-inset {user.active ? 'bg-emerald-100 text-emerald-700 ring-emerald-200' : 'bg-slate-100 text-slate-600 ring-slate-200'}">
                    <span class="size-1.5 rounded-full {user.active ? 'bg-emerald-500' : 'bg-slate-400'}"></span>
                    {user.active ? $t('admin.active') : $t('admin.inactive')}
                  </span>
                </td>
                <td class="px-4 py-3 text-right align-middle">
                  <button
                    onclick={() => openManage(user as AdminUser)}
                    class="inline-flex h-8 items-center rounded-md border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 shadow-sm transition-colors hover:bg-slate-50"
                  >
                    {$t('admin.manage_roles')}
                  </button>
                </td>
              </tr>
            {:else}
              <tr><td colspan="5" class="px-4 py-12 text-center text-sm text-slate-400">{$t('admin.no_users')}</td></tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>

  <!-- Roles reference -->
  <section class="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
    <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">{$t('admin.roles_label')}</h2>
    <p class="mt-1 text-xs text-slate-500">{$t('admin.roles_hint')}</p>
    <div class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-2">
      {#each rolesQuery.data?.roles ?? [] as role}
        <div class="rounded-lg border border-slate-200 bg-slate-50 p-3">
          <span class="inline-flex rounded-md px-2 py-0.5 text-xs font-semibold ring-1 ring-inset {roleTone[role.role] ?? 'bg-slate-100 text-slate-700 ring-slate-200'}">{role.name}</span>
          <p class="mt-2 text-xs text-slate-600">{role.description}</p>
        </div>
      {/each}
    </div>
  </section>
</div>

<!-- Manage roles modal -->
{#if editing}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/55 p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="manage-title"
    tabindex="-1"
    use:focusTrap
    onclick={(e) => { if (e.target === e.currentTarget) editing = null; }}
    onkeydown={(e) => { if (e.key === 'Escape') editing = null; }}
  >
    <div class="max-h-[90vh] w-full max-w-md overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl">
      <div class="border-b border-slate-200 px-5 py-4">
        <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">{$t('admin.manage_roles')}</p>
        <h2 id="manage-title" class="mt-1 text-lg font-bold text-slate-950">{editing.fullName || editing.username}</h2>
        <p class="mt-0.5 font-mono text-xs text-slate-500">{editing.username}</p>
      </div>

      <div class="space-y-2 px-5 py-4">
        {#each ALL_ROLES as role}
          {@const has = editing.roles.includes(role)}
          <div class="flex items-center justify-between rounded-lg border border-slate-200 px-3 py-2.5">
            <span class="inline-flex rounded-md px-2 py-0.5 text-xs font-semibold ring-1 ring-inset {roleTone[role]}">{roleNames[role]}</span>
            <button
              onclick={() => toggleRole(role, has)}
              disabled={working !== null}
              class="inline-flex h-8 min-w-[88px] items-center justify-center rounded-md px-3 text-xs font-semibold shadow-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50
                {has ? 'border border-red-200 bg-white text-red-700 hover:bg-red-50' : 'bg-blue-600 text-white hover:bg-blue-700'}"
            >
              {working === role ? $t('admin.saving') : has ? $t('admin.revoke') : $t('admin.assign')}
            </button>
          </div>
        {/each}

        {#if modalError}
          <p class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{modalError}</p>
        {/if}
        <p class="text-xs text-slate-400">{$t('admin.kc_sync_note')}</p>
      </div>

      <div class="flex justify-end border-t border-slate-200 bg-slate-50 px-5 py-3">
        <button onclick={() => editing = null} class="h-9 rounded-md border border-slate-200 bg-white px-4 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50">
          {$t('common.close')}
        </button>
      </div>
    </div>
  </div>
{/if}
