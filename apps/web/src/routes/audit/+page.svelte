<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { AuditService } from '$lib/gen/simaops/audit/v1/audit_pb';
  import DashboardIcon from '$lib/components/dashboard/DashboardIcon.svelte';
  import { highlightOnChange } from '$lib/actions/highlightOnChange.svelte';

  const client = createClient(AuditService, transport);

  let pageToken = $state('');
  let pageHistory = $state<string[]>(['']);
  let entityTypeFilter = $state('');
  let actorUserIdFilter = $state('');
  let actionFilter = $state('');
  let expandedLogId = $state('');

  const logsQuery = createQuery(() => ({
    queryKey: ['audit-logs', pageToken, entityTypeFilter, actorUserIdFilter, actionFilter],
    queryFn: () => client.listAuditLogs({
      pageSize: 50,
      pageToken,
      entityTypeFilter,
      actorUserIdFilter: actorUserIdFilter.trim(),
      actionFilter,
    }),
  }));

  const entityOptions = [
    { value: 'lot', label: 'Lot' },
    { value: 'qc_job', label: 'QC Job' },
    { value: 'dispatch', label: 'Dispatch' },
    { value: 'user', label: 'User' },
  ];

  const actionOptions = [
    'lot.created',
    'lot.status_changed',
    'qc.upload_requested',
    'qc.job_created',
    'qc.reviewed',
    'warehouse.assigned',
    'dispatch.created',
    'dispatch.status_changed',
    'admin.role_assigned',
    'admin.role_revoked',
  ];

  const actionMeta: Record<string, { tone: string; dot: string; label: string }> = {
    'lot.created': { tone: 'bg-blue-100 text-blue-700 ring-blue-200', dot: 'bg-blue-500', label: 'Created' },
    'lot.status_changed': { tone: 'bg-slate-100 text-slate-700 ring-slate-200', dot: 'bg-slate-400', label: 'Updated' },
    'qc.upload_requested': { tone: 'bg-orange-100 text-orange-700 ring-orange-200', dot: 'bg-orange-500', label: 'AI Inference' },
    'qc.job_created': { tone: 'bg-orange-100 text-orange-700 ring-orange-200', dot: 'bg-orange-500', label: 'AI Inference' },
    'qc.reviewed': { tone: 'bg-emerald-100 text-emerald-700 ring-emerald-200', dot: 'bg-emerald-500', label: 'Approved' },
    'warehouse.assigned': { tone: 'bg-purple-100 text-purple-700 ring-purple-200', dot: 'bg-purple-500', label: 'Updated' },
    'dispatch.created': { tone: 'bg-teal-100 text-teal-700 ring-teal-200', dot: 'bg-teal-500', label: 'Created' },
    'dispatch.status_changed': { tone: 'bg-teal-100 text-teal-700 ring-teal-200', dot: 'bg-teal-500', label: 'Updated' },
    'admin.role_assigned': { tone: 'bg-indigo-100 text-indigo-700 ring-indigo-200', dot: 'bg-indigo-500', label: 'System' },
    'admin.role_revoked': { tone: 'bg-indigo-100 text-indigo-700 ring-indigo-200', dot: 'bg-indigo-500', label: 'System' },
  };

  function nextPage() {
    const next = logsQuery.data?.nextPageToken;
    if (next) {
      pageHistory = [...pageHistory, next];
      pageToken = next;
      expandedLogId = '';
    }
  }

  function prevPage() {
    if (pageHistory.length > 1) {
      pageHistory = pageHistory.slice(0, -1);
      pageToken = pageHistory[pageHistory.length - 1];
      expandedLogId = '';
    }
  }

  function resetPaging() {
    pageToken = '';
    pageHistory = [''];
    expandedLogId = '';
  }

  function clearFilters() {
    entityTypeFilter = '';
    actorUserIdFilter = '';
    actionFilter = '';
    resetPaging();
  }

  function toggleDetails(id: string) {
    expandedLogId = expandedLogId === id ? '' : id;
  }

  function formatDate(seconds?: bigint | number | string) {
    if (!seconds) return '-';
    return new Date(Number(seconds) * 1000).toLocaleString('en-US', {
      month: '2-digit',
      day: '2-digit',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  }

  function shortId(value?: string) {
    return value ? `${value.slice(0, 8)}...` : '-';
  }

  function labelForEntity(value: string) {
    return entityOptions.find((option) => option.value === value)?.label ?? (value || '-');
  }

  function metaForAction(action: string) {
    return actionMeta[action] ?? { tone: 'bg-slate-100 text-slate-700 ring-slate-200', dot: 'bg-slate-400', label: 'System' };
  }

  function prettyJson(value?: string) {
    if (!value) return '-';
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }

  const logs = $derived(logsQuery.data?.logs ?? []);
  const hasNext = $derived(!!logsQuery.data?.nextPageToken);
  const hasPrev = $derived(pageHistory.length > 1);
  const filtersActive = $derived(!!entityTypeFilter || !!actorUserIdFilter.trim() || !!actionFilter);
  const totalCount = $derived(logsQuery.data?.totalCount ?? logs.length);
</script>

<div class="space-y-5">
  <header class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
    <div>
      <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">Governance</p>
      <h1 class="mt-1 text-[28px] font-bold tracking-normal text-slate-950">Audit Logs</h1>
      <p class="mt-1 text-sm text-slate-600">Track system events, user actions, and operational changes.</p>
    </div>

    <div class="flex flex-wrap items-center gap-3">
      <span class="rounded-md border border-slate-200 bg-white px-3 py-2 text-xs font-medium text-slate-500 shadow-sm">
        {totalCount.toLocaleString('en-US')} {totalCount === 1 ? 'event' : 'events'}
      </span>
      <button
        type="button"
        onclick={() => logsQuery.refetch()}
        class="inline-flex h-10 items-center gap-2 rounded-md border border-slate-200 bg-white px-3 text-sm font-medium text-slate-600 shadow-sm transition-colors hover:bg-slate-50 hover:text-slate-950"
      >
        <svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M21 12a9 9 0 0 1-15.5 6.2" />
          <path d="M3 12A9 9 0 0 1 18.5 5.8" />
          <path d="M18 2v4h4" />
          <path d="M6 22v-4H2" />
        </svg>
        {$t('common.refresh')}
      </button>
    </div>
  </header>

  <section class="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
    <div class="flex flex-col gap-3 xl:flex-row xl:items-end">
      <div class="grid min-w-0 flex-1 gap-3 sm:grid-cols-3">
        <div>
          <label for="entity-filter" class="mb-1 block text-xs font-semibold text-slate-500">Entity Type</label>
          <select
            id="entity-filter"
            bind:value={entityTypeFilter}
            onchange={resetPaging}
            class="h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-700 shadow-sm outline-none transition-colors hover:bg-slate-50 focus:border-blue-400 focus:ring-2 focus:ring-blue-100"
          >
            <option value="">All entities</option>
            {#each entityOptions as option}
              <option value={option.value}>{option.label}</option>
            {/each}
          </select>
        </div>

        <div>
          <label for="actor-filter" class="mb-1 block text-xs font-semibold text-slate-500">Actor User ID</label>
          <input
            id="actor-filter"
            bind:value={actorUserIdFilter}
            oninput={resetPaging}
            placeholder="Filter by actor"
            class="h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-700 shadow-sm outline-none transition-colors placeholder:text-slate-400 focus:border-blue-400 focus:ring-2 focus:ring-blue-100"
          />
        </div>

        <div>
          <label for="action-filter" class="mb-1 block text-xs font-semibold text-slate-500">Action</label>
          <select
            id="action-filter"
            bind:value={actionFilter}
            onchange={resetPaging}
            class="h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-700 shadow-sm outline-none transition-colors hover:bg-slate-50 focus:border-blue-400 focus:ring-2 focus:ring-blue-100"
          >
            <option value="">All actions</option>
            {#each actionOptions as action}
              <option value={action}>{action}</option>
            {/each}
          </select>
        </div>
      </div>

      <div class="flex justify-end gap-2">
        {#if filtersActive}
          <button
            type="button"
            onclick={clearFilters}
            class="h-10 rounded-md border border-slate-200 bg-white px-3 text-sm font-medium text-slate-600 shadow-sm transition-colors hover:bg-slate-50 hover:text-slate-950"
          >
            {$t('common.clear_filters')}
          </button>
        {/if}
      </div>
    </div>
  </section>

  <section class="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
    <div class="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3">
      <div>
        <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">Event Trail</h2>
        <p class="mt-1 text-xs text-slate-500">Newest audit events are listed first.</p>
      </div>
      <span class="rounded-md bg-slate-100 px-2.5 py-1 text-xs font-semibold text-slate-700">{logs.length} shown</span>
    </div>

    {#if logsQuery.isLoading}
      <div class="divide-y divide-slate-100">
        {#each [1, 2, 3, 4, 5, 6] as _}
          <div class="grid grid-cols-[1fr_.9fr_1fr_.9fr_.8fr_.7fr] gap-4 px-4 py-4">
            {#each [1, 2, 3, 4, 5, 6] as __}
              <div class="h-4 animate-pulse rounded bg-slate-100"></div>
            {/each}
          </div>
        {/each}
      </div>
    {:else if logsQuery.isError}
      <div class="m-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
        {logsQuery.error?.message || $t('common.error')}
      </div>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full min-w-[1120px] text-left text-sm">
          <thead class="border-b border-slate-200 bg-slate-50 text-xs font-semibold text-slate-500">
            <tr>
              <th class="px-4 py-3">Timestamp</th>
              <th class="px-4 py-3">Actor</th>
              <th class="px-4 py-3">Action</th>
              <th class="px-4 py-3">Resource</th>
              <th class="px-4 py-3">Request ID</th>
              <th class="px-4 py-3 text-right">Details</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            {#each logs as log}
              {@const action = metaForAction(log.action)}
              <tr class="transition-colors hover:bg-slate-50" use:highlightOnChange={log.id}>
                <td class="whitespace-nowrap px-4 py-3 align-middle text-xs text-slate-500" title={formatDate(log.createdAt?.seconds)}>
                  {formatDate(log.createdAt?.seconds)}
                </td>
                <td class="px-4 py-3 align-middle">
                  <div class="max-w-[220px] truncate text-xs font-semibold text-slate-800" title={log.actorUserId}>{log.actorUserId || '-'}</div>
                  <span class="mt-1 inline-flex rounded-md bg-slate-100 px-2 py-0.5 text-[11px] font-semibold text-slate-600">{log.actorRole || 'SYSTEM'}</span>
                </td>
                <td class="px-4 py-3 align-middle">
                  <span class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-semibold ring-1 ring-inset {action.tone}" title={log.action}>
                    <span class="size-1.5 rounded-full {action.dot}"></span>
                    {action.label}
                  </span>
                  <div class="mt-1 font-mono text-[11px] text-slate-500">{log.action}</div>
                </td>
                <td class="px-4 py-3 align-middle">
                  <div class="text-xs font-semibold text-slate-800">{labelForEntity(log.entityType)}</div>
                  <div class="mt-1 font-mono text-[11px] text-slate-500" title={log.entityId}>{shortId(log.entityId)}</div>
                </td>
                <td class="px-4 py-3 align-middle font-mono text-xs text-slate-500" title={log.requestId}>
                  {shortId(log.requestId)}
                </td>
                <td class="px-4 py-3 text-right align-middle">
                  <button
                    type="button"
                    onclick={() => toggleDetails(log.id)}
                    class="inline-flex h-8 items-center gap-2 rounded-md border border-slate-200 bg-white px-3 text-xs font-semibold text-blue-600 shadow-sm transition-colors hover:bg-blue-50"
                    aria-expanded={expandedLogId === log.id}
                  >
                    {expandedLogId === log.id ? 'Hide' : 'Details'}
                    <DashboardIcon name="arrow-right" class="size-3.5 {expandedLogId === log.id ? 'rotate-90' : ''}" />
                  </button>
                </td>
              </tr>
              {#if expandedLogId === log.id}
                <tr class="bg-slate-50/70">
                  <td colspan="6" class="px-4 py-4">
                    <div class="grid gap-4 lg:grid-cols-[.8fr_1.2fr]">
                      <div class="space-y-3 rounded-lg border border-slate-200 bg-white p-4">
                        <div>
                          <p class="text-xs font-semibold uppercase tracking-normal text-slate-500">Trace ID</p>
                          <p class="mt-1 break-all font-mono text-xs text-slate-700">{log.traceId || '-'}</p>
                        </div>
                        <div>
                          <p class="text-xs font-semibold uppercase tracking-normal text-slate-500">Request ID</p>
                          <p class="mt-1 break-all font-mono text-xs text-slate-700">{log.requestId || '-'}</p>
                        </div>
                        <div>
                          <p class="text-xs font-semibold uppercase tracking-normal text-slate-500">Entity ID</p>
                          <p class="mt-1 break-all font-mono text-xs text-slate-700">{log.entityId || '-'}</p>
                        </div>
                      </div>

                      <div class="grid gap-3 md:grid-cols-2">
                        <div class="min-w-0 rounded-lg border border-slate-200 bg-white p-4">
                          <p class="text-xs font-semibold uppercase tracking-normal text-slate-500">Before</p>
                          <pre class="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-md bg-slate-950 p-3 text-xs leading-5 text-slate-100">{prettyJson(log.beforeJson)}</pre>
                        </div>
                        <div class="min-w-0 rounded-lg border border-slate-200 bg-white p-4">
                          <p class="text-xs font-semibold uppercase tracking-normal text-slate-500">After</p>
                          <pre class="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-md bg-slate-950 p-3 text-xs leading-5 text-slate-100">{prettyJson(log.afterJson)}</pre>
                        </div>
                      </div>
                    </div>
                  </td>
                </tr>
              {/if}
            {:else}
              <tr>
                <td colspan="6" class="px-4 py-16">
                  <div class="mx-auto flex max-w-sm flex-col items-center text-center">
                    <div class="flex size-12 items-center justify-center rounded-lg bg-slate-100 text-slate-500">
                      <DashboardIcon name="shield" class="size-6" />
                    </div>
                    <h2 class="mt-3 text-sm font-semibold text-slate-950">{$t('audit.no_logs')}</h2>
                    <p class="mt-1 text-sm text-slate-500">
                      {filtersActive ? 'Try clearing filters to inspect the full audit trail.' : 'New audited actions will appear here automatically.'}
                    </p>
                    {#if filtersActive}
                      <button type="button" onclick={clearFilters} class="mt-4 rounded-md border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50">
                        {$t('common.clear_filters')}
                      </button>
                    {/if}
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <div class="flex flex-col gap-3 border-t border-slate-200 bg-slate-50 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
        <p class="text-xs text-slate-500">
          Showing {logs.length} {logs.length === 1 ? 'event' : 'events'}
        </p>
        <div class="flex justify-end gap-2">
          <button
            disabled={!hasPrev}
            onclick={prevPage}
            class="h-9 rounded-md border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700 shadow-sm transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {$t('common.prev')}
          </button>
          <button
            disabled={!hasNext}
            onclick={nextPage}
            class="h-9 rounded-md border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700 shadow-sm transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {$t('common.next')}
          </button>
        </div>
      </div>
    {/if}
  </section>
</div>
