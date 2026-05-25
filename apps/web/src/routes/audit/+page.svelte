<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { createConnectTransport } from '@connectrpc/connect-web';
  import { AuditService } from '$lib/gen/simaops/audit/v1/audit_connect';

  const transport = createConnectTransport({ baseUrl: 'http://localhost:8080', useBinaryFormat: false });
  const client = createClient(AuditService, transport);

  const logsQuery = createQuery({
    queryKey: ['audit-logs'],
    queryFn: () => client.listAuditLogs({ pageSize: 50 })
  });
</script>

<div class="space-y-4">
  <h1 class="text-2xl font-bold">{$t('nav.audit')}</h1>

  {#if $logsQuery.isLoading}
    <p class="text-gray-500">{$t('common.loading')}</p>
  {:else if $logsQuery.isError}
    <p class="text-red-500">{$t('common.error')}</p>
  {:else}
    <div class="overflow-x-auto border rounded-lg">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 border-b">
          <tr>
            <th class="px-4 py-3 text-left font-medium">Time</th>
            <th class="px-4 py-3 text-left font-medium">Actor</th>
            <th class="px-4 py-3 text-left font-medium">Action</th>
            <th class="px-4 py-3 text-left font-medium">Entity</th>
            <th class="px-4 py-3 text-left font-medium">Request ID</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each $logsQuery.data?.logs ?? [] as log}
            <tr class="hover:bg-gray-50">
              <td class="px-4 py-3 text-xs text-gray-500">{log.createdAt?.toDate().toLocaleString()}</td>
              <td class="px-4 py-3">
                <span class="text-xs">{log.actorUserId}</span>
                <span class="ml-1 px-1.5 py-0.5 rounded text-xs bg-gray-100">{log.actorRole}</span>
              </td>
              <td class="px-4 py-3 font-mono text-xs">{log.action}</td>
              <td class="px-4 py-3">
                <span class="text-xs">{log.entityType}</span>
                <span class="text-xs text-gray-400 ml-1">{log.entityId?.slice(0, 8)}...</span>
              </td>
              <td class="px-4 py-3 font-mono text-xs text-gray-400">{log.requestId?.slice(0, 8) ?? '—'}</td>
            </tr>
          {:else}
            <tr><td colspan="5" class="px-4 py-8 text-center text-gray-400">No audit logs yet</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
