<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { DashboardService } from '$lib/gen/simaops/dashboard/v1/dashboard_pb';

  const client = createClient(DashboardService, transport);

  // Auto-refresh every 30s
  const opsQuery = createQuery(() => ({
    queryKey: ['dashboard-ops'],
    queryFn: () => client.getOpsDashboard({}),
    refetchInterval: 30_000,
  }));

  const qcQuery = createQuery(() => ({
    queryKey: ['dashboard-qc'],
    queryFn: () => client.getQCMetrics({ hours: 24 }),
    refetchInterval: 30_000,
  }));

  const whQuery = createQuery(() => ({
    queryKey: ['dashboard-warehouse'],
    queryFn: () => client.getWarehouseMetrics({}),
    refetchInterval: 30_000,
  }));

  const lastUpdated = $derived(
    [opsQuery.dataUpdatedAt, qcQuery.dataUpdatedAt, whQuery.dataUpdatedAt]
      .filter(Boolean)
      .reduce((max, t) => Math.max(max, t), 0)
  );
  const lastUpdatedLabel = $derived(lastUpdated ? new Date(lastUpdated).toLocaleTimeString() : '—');

  const isLoading = $derived(opsQuery.isLoading || qcQuery.isLoading || whQuery.isLoading);
  const isError = $derived(opsQuery.isError && qcQuery.isError && whQuery.isError);
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold">{$t('nav.dashboard')}</h1>
    <div class="text-xs text-gray-500 flex items-center gap-2">
      {#if isLoading}
        <span class="inline-block w-2 h-2 rounded-full bg-blue-500 animate-pulse" aria-hidden="true"></span>
      {:else}
        <span class="inline-block w-2 h-2 rounded-full bg-green-500" aria-hidden="true"></span>
      {/if}
      {$t('dashboard.last_updated')}: {lastUpdatedLabel}
    </div>
  </div>

  {#if isError}
    <div class="p-4 bg-red-50 border border-red-200 rounded text-red-700 text-sm">
      {$t('common.error')} — {opsQuery.error?.message || qcQuery.error?.message || whQuery.error?.message}
    </div>
  {/if}

  <!-- Ops KPIs -->
  <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
    <div class="border rounded-lg p-4 bg-white">
      <p class="text-sm text-gray-500">{$t('dashboard.total_lots')}</p>
      <p class="text-3xl font-bold">{opsQuery.data?.totalLots ?? '—'}</p>
    </div>
    <div class="border rounded-lg p-4 bg-white">
      <p class="text-sm text-gray-500">{$t('dashboard.awaiting_qc')}</p>
      <p class="text-3xl font-bold text-orange-600">{opsQuery.data?.lotsAwaitingQc ?? '—'}</p>
    </div>
    <div class="border rounded-lg p-4 bg-white">
      <p class="text-sm text-gray-500">{$t('dashboard.ready_for_production')}</p>
      <p class="text-3xl font-bold text-green-600">{opsQuery.data?.lotsReadyForProduction ?? '—'}</p>
    </div>
    <div class="border rounded-lg p-4 bg-white">
      <p class="text-sm text-gray-500">{$t('dashboard.qc_pass_rate')}</p>
      <p class="text-3xl font-bold text-blue-600">
        {qcQuery.data?.passRate != null ? `${Math.round(qcQuery.data.passRate * 100)}%` : '—'}
      </p>
    </div>
  </div>

  <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
    <!-- QC Metrics -->
    <div class="border rounded-lg p-4 bg-white space-y-3">
      <h2 class="font-semibold text-sm text-gray-500 uppercase">{$t('dashboard.qc_metrics')}</h2>
      {#if qcQuery.isLoading}
        <p class="text-gray-400 text-sm">{$t('common.loading')}</p>
      {:else}
        <div class="grid grid-cols-3 gap-3 text-center">
          <div>
            <p class="text-2xl font-bold text-green-600">{qcQuery.data?.passCount ?? 0}</p>
            <p class="text-xs text-gray-500">{$t('dashboard.qc_pass')}</p>
          </div>
          <div>
            <p class="text-2xl font-bold text-yellow-600">{qcQuery.data?.reviewCount ?? 0}</p>
            <p class="text-xs text-gray-500">{$t('dashboard.qc_review')}</p>
          </div>
          <div>
            <p class="text-2xl font-bold text-red-600">{qcQuery.data?.failCount ?? 0}</p>
            <p class="text-xs text-gray-500">{$t('dashboard.qc_fail')}</p>
          </div>
        </div>
        <div class="text-sm text-gray-500">
          {$t('dashboard.avg_confidence')}:
          <span class="font-medium">{qcQuery.data?.averageConfidence != null ? `${Math.round(qcQuery.data.averageConfidence * 100)}%` : '—'}</span>
          · {$t('dashboard.pending_review')}: <span class="font-medium">{qcQuery.data?.pendingReviewCount ?? 0}</span>
        </div>
      {/if}
    </div>

    <!-- Warehouse Metrics -->
    <div class="border rounded-lg p-4 bg-white space-y-3">
      <h2 class="font-semibold text-sm text-gray-500 uppercase">{$t('dashboard.warehouse_capacity')}</h2>
      {#if whQuery.isLoading}
        <p class="text-gray-400 text-sm">{$t('common.loading')}</p>
      {:else}
        <div class="space-y-2">
          {#each whQuery.data?.zones ?? [] as zone}
            {@const pct = zone.totalCapacity > 0 ? Math.round((zone.available / zone.totalCapacity) * 100) : 0}
            <div>
              <div class="flex items-center justify-between text-sm mb-1">
                <span class="font-medium">{zone.zone}</span>
                <span class="text-gray-500">{zone.available} / {zone.totalCapacity} {$t('dashboard.available')}</span>
              </div>
              <div class="h-2 bg-gray-100 rounded-full overflow-hidden" role="progressbar" aria-valuenow={pct} aria-valuemin="0" aria-valuemax="100">
                <div class="h-full bg-blue-500 transition-all" style="width: {pct}%"></div>
              </div>
            </div>
          {/each}
        </div>
        <div class="text-sm text-gray-500 border-t pt-2">
          {$t('dashboard.total_label')}: <span class="font-medium">{whQuery.data?.totalAvailable ?? 0}</span> / {whQuery.data?.totalCapacity ?? 0} {$t('dashboard.slots_available')}
        </div>
      {/if}
    </div>
  </div>

  <!-- Lot Status Distribution -->
  <div class="border rounded-lg p-4 bg-white space-y-3">
    <h2 class="font-semibold text-sm text-gray-500 uppercase">{$t('dashboard.lot_distribution')}</h2>
    {#if opsQuery.isLoading}
      <p class="text-gray-400 text-sm">{$t('common.loading')}</p>
    {:else}
      <div class="flex flex-wrap gap-3">
        {#each opsQuery.data?.lotsByStatus ?? [] as sc}
          <div class="px-3 py-2 rounded-md bg-gray-50 border text-sm">
            <span class="font-medium">{sc.count}</span>
            <span class="text-gray-500 ml-1">{sc.status?.replace(/_/g, ' ').toLowerCase()}</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>
