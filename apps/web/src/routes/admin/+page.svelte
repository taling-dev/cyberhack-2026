<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { AdminService } from '$lib/gen/simaops/admin/v1/admin_pb';

  const client = createClient(AdminService, transport);

  const usersQuery = createQuery(() => ({
    queryKey: ['admin-users'],
    queryFn: () => client.listUsers({ pageSize: 50 })
  }));

  const rolesQuery = createQuery(() => ({
    queryKey: ['admin-roles'],
    queryFn: () => client.listRoles({})
  }));

  const roleNames: Record<number, string> = {
    1: 'OPERATOR', 2: 'QC_SUPERVISOR', 3: 'WAREHOUSE_STAFF', 4: 'MANAGER', 5: 'ADMIN'
  };
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold">{$t('nav.admin')}</h1>

  <!-- Users -->
  <div class="border rounded-lg p-4 bg-white space-y-3">
    <h2 class="font-semibold text-sm text-gray-500 uppercase">{$t('admin.users_label')} ({usersQuery.data?.totalCount ?? 0})</h2>
    {#if usersQuery.isLoading}
      <p class="text-gray-400 text-sm">{$t('common.loading')}</p>
    {:else if usersQuery.isError}
      <div class="p-2 bg-red-50 border border-red-200 rounded text-red-700 text-sm">{usersQuery.error?.message || $t('common.error')}</div>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead class="bg-gray-50 border-b">
            <tr>
              <th class="px-4 py-2 text-left font-medium">{$t('admin.username')}</th>
              <th class="px-4 py-2 text-left font-medium">{$t('admin.name')}</th>
              <th class="px-4 py-2 text-left font-medium">{$t('admin.email')}</th>
              <th class="px-4 py-2 text-left font-medium">{$t('admin.roles_label')}</th>
              <th class="px-4 py-2 text-left font-medium">{$t('common.status')}</th>
            </tr>
          </thead>
          <tbody class="divide-y">
            {#each usersQuery.data?.users ?? [] as user}
              <tr class="hover:bg-gray-50">
                <td class="px-4 py-2 font-mono text-xs">{user.username}</td>
                <td class="px-4 py-2">{user.fullName}</td>
                <td class="px-4 py-2 text-gray-500 text-xs">{user.email}</td>
                <td class="px-4 py-2">
                  {#each user.roles ?? [] as role}
                    <span class="inline-block px-2 py-0.5 rounded text-xs bg-blue-100 text-blue-700 mr-1 mb-0.5">{roleNames[role] ?? role}</span>
                  {/each}
                </td>
                <td class="px-4 py-2">
                  <span class="px-2 py-0.5 rounded text-xs {user.active ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}">
                    {user.active ? $t('admin.active') : $t('admin.inactive')}
                  </span>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>

  <!-- Roles -->
  <div class="border rounded-lg p-4 bg-white space-y-3">
    <h2 class="font-semibold text-sm text-gray-500 uppercase">{$t('admin.roles_label')}</h2>
    {#if rolesQuery.isLoading}
      <p class="text-gray-400 text-sm">{$t('common.loading')}</p>
    {:else}
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
        {#each rolesQuery.data?.roles ?? [] as role}
          <div class="border rounded-md p-3">
            <span class="font-medium text-sm">{role.name}</span>
            <p class="text-xs text-gray-500 mt-1">{role.description}</p>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>
