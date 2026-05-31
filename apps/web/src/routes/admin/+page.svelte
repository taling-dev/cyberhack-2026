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

  const builtinTone: Record<string, string> = {
    OPERATOR: 'bg-blue-100 text-blue-700 ring-blue-200',
    QC_SUPERVISOR: 'bg-orange-100 text-orange-700 ring-orange-200',
    WAREHOUSE_STAFF: 'bg-purple-100 text-purple-700 ring-purple-200',
    MANAGER: 'bg-emerald-100 text-emerald-700 ring-emerald-200',
    ADMIN: 'bg-red-100 text-red-700 ring-red-200'
  };
  const toneFor = (name: string) => builtinTone[name] ?? 'bg-slate-100 text-slate-700 ring-slate-200';

  // Friendly label for an RPC path: "Service/Method" -> "Method (Service)".
  function procLabel(path: string) {
    const tail = path.split('/').pop() ?? path;
    const svc = (path.match(/\.(\w+)Service\//) ?? [])[1] ?? '';
    const method = tail.replace(/([a-z])([A-Z])/g, '$1 $2');
    return svc ? `${method} · ${svc}` : method;
  }

  const usersQuery = createQuery(() => ({
    queryKey: ['admin-users'],
    queryFn: () => client.listUsers({ pageSize: 100 }),
  }));
  const rolesQuery = createQuery(() => ({
    queryKey: ['admin-roles'],
    queryFn: () => client.listRoles({}),
  }));
  const proceduresQuery = createQuery(() => ({
    queryKey: ['admin-procedures'],
    queryFn: () => client.listProcedures({}),
  }));

  const currentUsername = $derived($page.data.user?.username ?? '');
  const allRoleNames = $derived((rolesQuery.data?.roles ?? []).map((r) => r.name));

  function refetchAll() {
    queryClient.invalidateQueries({ queryKey: ['admin-users'] });
    queryClient.invalidateQueries({ queryKey: ['admin-roles'] });
  }

  // ── Manage-roles modal ────────────────────────────────────────────
  type AdminUser = { id: string; username: string; fullName: string; email: string; roleNames: string[]; active: boolean };
  let editing = $state<AdminUser | null>(null);
  let working = $state<string | null>(null);
  let modalError = $state('');
  const isSelf = $derived(editing?.username === currentUsername);

  function openManage(u: any) {
    editing = { id: u.id, username: u.username, fullName: u.fullName, email: u.email, roleNames: [...(u.roleNames ?? [])], active: u.active };
    modalError = '';
  }

  async function toggleRole(roleName: string, has: boolean) {
    if (!editing) return;
    if (has) {
      if (isSelf && roleName === 'ADMIN') { modalError = $t('admin.guard_self_admin'); return; }
      if (editing.roleNames.length <= 1) { modalError = $t('admin.guard_last_role'); return; }
    }
    working = roleName;
    modalError = '';
    try {
      if (has) {
        await client.revokeRole({ userId: editing.id, roleName, idempotencyKey: crypto.randomUUID() });
        editing.roleNames = editing.roleNames.filter((r) => r !== roleName);
      } else {
        await client.assignRole({ userId: editing.id, roleName, idempotencyKey: crypto.randomUUID() });
        editing.roleNames = [...editing.roleNames, roleName];
      }
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
    } catch {
      modalError = $t('admin.role_update_failed');
    } finally {
      working = null;
    }
  }

  // ── Create user ───────────────────────────────────────────────────
  let showCreateUser = $state(false);
  let cuUsername = $state(''); let cuEmail = $state(''); let cuName = $state(''); let cuPassword = $state('');
  let cuRoles = $state<string[]>([]); let cuBusy = $state(false); let cuError = $state('');

  function openCreateUser() {
    cuUsername = ''; cuEmail = ''; cuName = ''; cuPassword = ''; cuRoles = []; cuError = '';
    showCreateUser = true;
  }
  async function submitCreateUser() {
    if (!cuUsername.trim() || !cuEmail.trim() || !cuPassword) { cuError = $t('admin.cu_required'); return; }
    cuBusy = true; cuError = '';
    try {
      await client.createUser({ username: cuUsername.trim(), email: cuEmail.trim(), fullName: cuName.trim(), tempPassword: cuPassword, roleNames: cuRoles });
      showCreateUser = false;
      refetchAll();
    } catch (e: any) { cuError = e?.message || $t('admin.cu_failed'); } finally { cuBusy = false; }
  }

  // ── Create role ───────────────────────────────────────────────────
  let showCreateRole = $state(false);
  let crName = $state(''); let crDesc = $state(''); let crPerms = $state<string[]>([]);
  let crBusy = $state(false); let crError = $state('');

  function openCreateRole() {
    crName = ''; crDesc = ''; crPerms = []; crError = '';
    showCreateRole = true;
  }
  function togglePerm(p: string) {
    crPerms = crPerms.includes(p) ? crPerms.filter((x) => x !== p) : [...crPerms, p];
  }
  async function submitCreateRole() {
    if (!crName.trim()) { crError = $t('admin.cr_required'); return; }
    crBusy = true; crError = '';
    try {
      await client.createRole({ name: crName.trim(), description: crDesc.trim(), permissions: crPerms });
      showCreateRole = false;
      refetchAll();
    } catch (e: any) { crError = e?.message || $t('admin.cr_failed'); } finally { crBusy = false; }
  }

  async function deleteRole(roleId: string, name: string) {
    if (!confirm($t('admin.delete_role_confirm') + ' ' + name)) return;
    try {
      await client.deleteRole({ roleId });
      refetchAll();
    } catch { /* surfaced via list refetch */ }
  }
</script>

<div class="space-y-5">
  <header class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
    <div>
      <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">{$t('admin.eyebrow')}</p>
      <h1 class="mt-1 text-[28px] font-bold tracking-normal text-slate-950">{$t('nav.admin')}</h1>
      <p class="mt-1 text-sm text-slate-600">{$t('admin.subtitle')}</p>
    </div>
    <div class="flex flex-wrap gap-2">
      <button onclick={openCreateUser} class="inline-flex h-9 items-center rounded-md bg-blue-600 px-3 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-blue-700">
        {$t('admin.create_user')}
      </button>
      <button onclick={openCreateRole} class="inline-flex h-9 items-center rounded-md border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 shadow-sm transition-colors hover:bg-slate-50">
        {$t('admin.create_role')}
      </button>
    </div>
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
            {#each [1, 2, 3, 4] as __}<div class="h-4 animate-pulse rounded bg-slate-100"></div>{/each}
          </div>
        {/each}
      </div>
    {:else if usersQuery.isError}
      <div class="m-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{usersQuery.error?.message || $t('common.error')}</div>
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
                    {#each user.roleNames ?? [] as rn}
                      <span class="inline-flex rounded-md px-2 py-0.5 text-xs font-semibold ring-1 ring-inset {toneFor(rn)}">{rn}</span>
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
                  <button onclick={() => openManage(user)} class="inline-flex h-8 items-center rounded-md border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 shadow-sm transition-colors hover:bg-slate-50">{$t('admin.manage_roles')}</button>
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

  <!-- Roles -->
  <section class="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
    <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">{$t('admin.roles_label')}</h2>
    <p class="mt-1 text-xs text-slate-500">{$t('admin.roles_hint')}</p>
    <div class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-2">
      {#each rolesQuery.data?.roles ?? [] as role}
        <div class="rounded-lg border border-slate-200 bg-slate-50 p-3">
          <div class="flex items-start justify-between gap-2">
            <span class="inline-flex rounded-md px-2 py-0.5 text-xs font-semibold ring-1 ring-inset {toneFor(role.name)}">{role.name}</span>
            {#if role.isSystem}
              <span class="text-[10px] font-semibold uppercase tracking-normal text-slate-400">{$t('admin.system_role')}</span>
            {:else}
              <button onclick={() => deleteRole(role.id, role.name)} class="text-xs font-medium text-red-600 hover:underline">{$t('admin.delete')}</button>
            {/if}
          </div>
          <p class="mt-2 text-xs text-slate-600">{role.description}</p>
          {#if (role.permissions?.length ?? 0) > 0}
            <p class="mt-2 text-[11px] text-slate-400">{role.permissions.length} {$t('admin.permissions')}</p>
          {/if}
        </div>
      {/each}
    </div>
  </section>
</div>

<!-- Manage roles modal -->
{#if editing}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/55 p-4" role="dialog" aria-modal="true" aria-labelledby="manage-title" tabindex="-1" use:focusTrap onclick={(e) => { if (e.target === e.currentTarget) editing = null; }} onkeydown={(e) => { if (e.key === 'Escape') editing = null; }}>
    <div class="max-h-[90vh] w-full max-w-md overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl">
      <div class="border-b border-slate-200 px-5 py-4">
        <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">{$t('admin.manage_roles')}</p>
        <h2 id="manage-title" class="mt-1 text-lg font-bold text-slate-950">{editing.fullName || editing.username}</h2>
        <p class="mt-0.5 font-mono text-xs text-slate-500">{editing.username}</p>
      </div>
      <div class="max-h-[60vh] space-y-2 overflow-y-auto px-5 py-4">
        {#each allRoleNames as roleName}
          {@const has = editing.roleNames.includes(roleName)}
          <div class="flex items-center justify-between rounded-lg border border-slate-200 px-3 py-2.5">
            <span class="inline-flex rounded-md px-2 py-0.5 text-xs font-semibold ring-1 ring-inset {toneFor(roleName)}">{roleName}</span>
            <button onclick={() => toggleRole(roleName, has)} disabled={working !== null} class="inline-flex h-8 min-w-[88px] items-center justify-center rounded-md px-3 text-xs font-semibold shadow-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50 {has ? 'border border-red-200 bg-white text-red-700 hover:bg-red-50' : 'bg-blue-600 text-white hover:bg-blue-700'}">
              {working === roleName ? $t('admin.saving') : has ? $t('admin.revoke') : $t('admin.assign')}
            </button>
          </div>
        {/each}
        {#if modalError}<p class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{modalError}</p>{/if}
        <p class="text-xs text-slate-400">{$t('admin.kc_sync_note')}</p>
      </div>
      <div class="flex justify-end border-t border-slate-200 bg-slate-50 px-5 py-3">
        <button onclick={() => editing = null} class="h-9 rounded-md border border-slate-200 bg-white px-4 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50">{$t('common.close')}</button>
      </div>
    </div>
  </div>
{/if}

<!-- Create user modal -->
{#if showCreateUser}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/55 p-4" role="dialog" aria-modal="true" tabindex="-1" use:focusTrap onclick={(e) => { if (e.target === e.currentTarget) showCreateUser = false; }} onkeydown={(e) => { if (e.key === 'Escape') showCreateUser = false; }}>
    <div class="max-h-[90vh] w-full max-w-md overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl">
      <div class="border-b border-slate-200 px-5 py-4"><h2 class="text-lg font-bold text-slate-950">{$t('admin.create_user')}</h2></div>
      <div class="max-h-[62vh] space-y-3 overflow-y-auto px-5 py-4 text-sm">
        <label class="block"><span class="mb-1 block font-medium text-slate-700">{$t('admin.username')}</span><input bind:value={cuUsername} class="w-full rounded-md border border-slate-200 px-3 py-2 focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-100" /></label>
        <label class="block"><span class="mb-1 block font-medium text-slate-700">{$t('admin.email')}</span><input bind:value={cuEmail} type="email" class="w-full rounded-md border border-slate-200 px-3 py-2 focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-100" /></label>
        <label class="block"><span class="mb-1 block font-medium text-slate-700">{$t('admin.name')}</span><input bind:value={cuName} class="w-full rounded-md border border-slate-200 px-3 py-2 focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-100" /></label>
        <label class="block"><span class="mb-1 block font-medium text-slate-700">{$t('admin.temp_password')}</span><input bind:value={cuPassword} type="text" class="w-full rounded-md border border-slate-200 px-3 py-2 font-mono focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-100" /><span class="mt-1 block text-xs text-slate-400">{$t('admin.temp_password_hint')}</span></label>
        <div>
          <span class="mb-1 block font-medium text-slate-700">{$t('admin.initial_roles')}</span>
          <div class="flex flex-wrap gap-2">
            {#each allRoleNames as rn}
              <label class="inline-flex items-center gap-1.5 rounded-md border border-slate-200 px-2 py-1 text-xs">
                <input type="checkbox" checked={cuRoles.includes(rn)} onchange={() => cuRoles = cuRoles.includes(rn) ? cuRoles.filter((x) => x !== rn) : [...cuRoles, rn]} class="rounded border-slate-300 text-blue-600 focus:ring-blue-500" />
                {rn}
              </label>
            {/each}
          </div>
        </div>
        {#if cuError}<p class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-red-700" role="alert">{cuError}</p>{/if}
      </div>
      <div class="flex justify-end gap-2 border-t border-slate-200 bg-slate-50 px-5 py-3">
        <button onclick={() => showCreateUser = false} class="h-9 rounded-md border border-slate-200 bg-white px-4 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50">{$t('common.cancel')}</button>
        <button onclick={submitCreateUser} disabled={cuBusy} class="h-9 rounded-md bg-blue-600 px-4 text-sm font-semibold text-white shadow-sm hover:bg-blue-700 disabled:opacity-50">{cuBusy ? $t('admin.saving') : $t('admin.create_user')}</button>
      </div>
    </div>
  </div>
{/if}

<!-- Create role modal -->
{#if showCreateRole}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/55 p-4" role="dialog" aria-modal="true" tabindex="-1" use:focusTrap onclick={(e) => { if (e.target === e.currentTarget) showCreateRole = false; }} onkeydown={(e) => { if (e.key === 'Escape') showCreateRole = false; }}>
    <div class="max-h-[90vh] w-full max-w-lg overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl">
      <div class="border-b border-slate-200 px-5 py-4"><h2 class="text-lg font-bold text-slate-950">{$t('admin.create_role')}</h2><p class="mt-0.5 text-xs text-slate-500">{$t('admin.create_role_hint')}</p></div>
      <div class="max-h-[62vh] space-y-3 overflow-y-auto px-5 py-4 text-sm">
        <label class="block"><span class="mb-1 block font-medium text-slate-700">{$t('admin.role_name')}</span><input bind:value={crName} placeholder="e.g. AUDITOR" class="w-full rounded-md border border-slate-200 px-3 py-2 focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-100" /></label>
        <label class="block"><span class="mb-1 block font-medium text-slate-700">{$t('admin.description')}</span><input bind:value={crDesc} class="w-full rounded-md border border-slate-200 px-3 py-2 focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-100" /></label>
        <div>
          <span class="mb-1 block font-medium text-slate-700">{$t('admin.permissions')}</span>
          <div class="space-y-1.5 rounded-lg border border-slate-200 p-2">
            {#each proceduresQuery.data?.procedures ?? [] as p}
              <label class="flex items-center gap-2 rounded px-1.5 py-1 hover:bg-slate-50">
                <input type="checkbox" checked={crPerms.includes(p)} onchange={() => togglePerm(p)} class="rounded border-slate-300 text-blue-600 focus:ring-blue-500" />
                <span class="text-xs text-slate-700">{procLabel(p)}</span>
              </label>
            {/each}
          </div>
          <p class="mt-1 text-xs text-slate-400">{$t('admin.perm_note')}</p>
        </div>
        {#if crError}<p class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-red-700" role="alert">{crError}</p>{/if}
      </div>
      <div class="flex justify-end gap-2 border-t border-slate-200 bg-slate-50 px-5 py-3">
        <button onclick={() => showCreateRole = false} class="h-9 rounded-md border border-slate-200 bg-white px-4 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50">{$t('common.cancel')}</button>
        <button onclick={submitCreateRole} disabled={crBusy} class="h-9 rounded-md bg-blue-600 px-4 text-sm font-semibold text-white shadow-sm hover:bg-blue-700 disabled:opacity-50">{crBusy ? $t('admin.saving') : $t('admin.create_role')}</button>
      </div>
    </div>
  </div>
{/if}
