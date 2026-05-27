<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { AuditService } from '$lib/gen/simaops/audit/v1/audit_pb';
  import { highlightOnChange } from '$lib/actions/highlightOnChange.svelte';

  const client = createClient(AuditService, transport);

  let pageToken = $state('');
  let pageHistory = $state<string[]>(['']);

  const logsQuery = createQuery(() => ({
    queryKey: ['audit-logs', pageToken],
    queryFn: () => client.listAuditLogs({ pageSize: 50, pageToken }),
  }));

  function nextPage() {
    const next = logsQuery.data?.nextPageToken;
    if (next) {
      pageHistory = [...pageHistory, next];
      pageToken = next;
    }
  }
  function prevPage() {
    if (pageHistory.length > 1) {
      pageHistory = pageHistory.slice(0, -1);
      pageToken = pageHistory[pageHistory.length - 1];
    }
  }

  const logs = $derived(logsQuery.data?.logs ?? []);
  const hasNext = $derived(!!logsQuery.data?.nextPageToken);
  const hasPrev = $derived(pageHistory.length > 1);

  const actionColors: Record<string, string> = {
    'lot.created': 'bg-blue-100 text-blue-700',
    'qc.job_created': 'bg-orange-100 text-orange-700',
    'qc.upload_requested': 'bg-orange-100 text-orange-700',
    'qc.reviewed': 'bg-green-100 text-green-700',
    'warehouse.assigned': 'bg-purple-100 text-purple-700',
    'lot.status_changed': 'bg-gray-100 text-gray-700',
  };
</script>

<div class="space-y-4">
  <h1 class="text-2xl font-bold">{$t('nav.audit')}</h1>

  {#if logsQuery.isLoading}
    <p class="text-gray-500">{$t('common.loading')}</p>
  {:else if logsQuery.isError}
    <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm">{logsQuery.error?.message || $t('common.error')}</div>
  {:else}
    <div class="overflow-x-auto border rounded-lg bg-white">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 border-b">
          <tr>
            <th class="px-4 py-3 text-left font-medium">{$t('audit.time')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('audit.actor')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('audit.action')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('audit.entity')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('audit.request_id')}</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each logs as log}
            <tr class="hover:bg-gray-50 transition-colors" use:highlightOnChange={log.id}>
              <td class="px-4 py-3 text-xs text-gray-500 whitespace-nowrap">{log.createdAt ? new Date(Number(log.createdAt.seconds) * 1000).toLocaleString() : ''}</td>
              <td class="px-4 py-3">
                <span class="text-xs font-medium">{log.actorUserId}</span>
                <span class="ml-1 px-1.5 py-0.5 rounded text-xs bg-gray-100">{log.actorRole}</span>
              </td>
              <td class="px-4 py-3">
                <span class="px-2 py-0.5 rounded text-xs font-mono {actionColors[log.action] ?? 'bg-gray-100'}">{log.action}</span>
              </td>
              <td class="px-4 py-3">
                <span class="text-xs">{log.entityType}</span>
                <span class="text-xs text-gray-400 ml-1 font-mono">{log.entityId?.slice(0, 8)}…</span>
              </td>
              <td class="px-4 py-3 font-mono text-xs text-gray-400">{log.requestId?.slice(0, 8) ?? '—'}</td>
            </tr>
          {:else}
            <tr><td colspan="5" class="px-4 py-12 text-center text-gray-400">{$t('audit.no_logs')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if hasPrev || hasNext}
      <div class="flex justify-center gap-2 pt-2">
        <button disabled={!hasPrev} onclick={prevPage} class="px-3 py-1.5 border rounded text-sm disabled:opacity-40">← Prev</button>
        <button disabled={!hasNext} onclick={nextPage} class="px-3 py-1.5 border rounded text-sm disabled:opacity-40">Next →</button>
      </div>
    {/if}
  {/if}
</div>
