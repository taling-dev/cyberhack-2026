<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { DashboardService } from '$lib/gen/simaops/dashboard/v1/dashboard_pb';

  const client = createClient(DashboardService, transport);

  const opsQuery = createQuery({
    queryKey: ['dashboard-ops'],
    queryFn: () => client.getOpsDashboard({})
  });

  const qcQuery = createQuery({
    queryKey: ['dashboard-qc'],
    queryFn: () => client.getQCMetrics({ hours: 24 })
  });

  const whQuery = createQuery({
    queryKey: ['dashboard-warehouse'],
    queryFn: () => client.getWarehouseMetrics({})
  });
</script>

<div class="space-y-6">
  <h1 class="text-2xl font-bold">{$t('nav.dashboard')}</h1>

  <!-- Ops KPIs -->
  <div class="grid grid-cols-4 gap-4">
    <div class="border rounded-lg p-4 bg-white">
      <p class="text-sm text-gray-500">Total Lots</p>
      <p class="text-3xl font-bold">{$opsQuery.data?.totalLots ?? '—'}</p>
    </div>
    <div class="border rounded-lg p-4 bg-white">
      <p class="text-sm text-gray-500">Awaiting QC</p>
      <p class="text-3xl font-bold text-orange-600">{$opsQuery.data?.lotsAwaitingQc ?? '—'}</p>
    </div>
    <div class="border rounded-lg p-4 bg-white">
      <p class="text-sm text-gray-500">Ready for Production</p>
      <p class="text-3xl font-bold text-green-600">{$opsQuery.data?.lotsReadyForProduction ?? '—'}</p>
    </div>
    <div class="border rounded-lg p-4 bg-white">
      <p class="text-sm text-gray-500">QC Pass Rate (24h)</p>
      <p class="text-3xl font-bold text-blue-600">
        {$qcQuery.data?.passRate != null ? `${Math.round($qcQuery.data.passRate * 100)}%` : '—'}
      </p>
    </div>
  </div>

  <div class="grid grid-cols-2 gap-6">
    <!-- QC Metrics -->
    <div class="border rounded-lg p-4 bg-white space-y-3">
      <h2 class="font-semibold text-sm text-gray-500 uppercase">QC Metrics (24h)</h2>
      {#if $qcQuery.isLoading}
        <p class="text-gray-400 text-sm">{$t('common.loading')}</p>
      {:else}
        <div class="grid grid-cols-3 gap-3 text-center">
          <div>
            <p class="text-2xl font-bold text-green-600">{$qcQuery.data?.passCount ?? 0}</p>
            <p class="text-xs text-gray-500">Pass</p>
          </div>
          <div>
            <p class="text-2xl font-bold text-yellow-600">{$qcQuery.data?.reviewCount ?? 0}</p>
            <p class="text-xs text-gray-500">Review</p>
          </div>
          <div>
            <p class="text-2xl font-bold text-red-600">{$qcQuery.data?.failCount ?? 0}</p>
            <p class="text-xs text-gray-500">Fail</p>
          </div>
        </div>
        <div class="text-sm text-gray-500">
          Avg confidence: <span class="font-medium">{$qcQuery.data?.averageConfidence != null ? `${Math.round($qcQuery.data.averageConfidence * 100)}%` : '—'}</span>
          · Pending review: <span class="font-medium">{$qcQuery.data?.pendingReviewCount ?? 0}</span>
        </div>
      {/if}
    </div>

    <!-- Warehouse Metrics -->
    <div class="border rounded-lg p-4 bg-white space-y-3">
      <h2 class="font-semibold text-sm text-gray-500 uppercase">Warehouse Capacity</h2>
      {#if $whQuery.isLoading}
        <p class="text-gray-400 text-sm">{$t('common.loading')}</p>
      {:else}
        <div class="space-y-2">
          {#each $whQuery.data?.zones ?? [] as zone}
            <div class="flex items-center justify-between text-sm">
              <span class="font-medium">{zone.zone}</span>
              <span class="text-gray-500">{zone.available} / {zone.totalCapacity} available</span>
            </div>
          {/each}
        </div>
        <div class="text-sm text-gray-500 border-t pt-2">
          Total: <span class="font-medium">{$whQuery.data?.totalAvailable ?? 0}</span> / {$whQuery.data?.totalCapacity ?? 0} slots available
        </div>
      {/if}
    </div>
  </div>

  <!-- Lot Status Distribution -->
  <div class="border rounded-lg p-4 bg-white space-y-3">
    <h2 class="font-semibold text-sm text-gray-500 uppercase">Lot Status Distribution</h2>
    {#if $opsQuery.isLoading}
      <p class="text-gray-400 text-sm">{$t('common.loading')}</p>
    {:else}
      <div class="flex flex-wrap gap-3">
        {#each $opsQuery.data?.lotsByStatus ?? [] as sc}
          <div class="px-3 py-2 rounded-md bg-gray-50 border text-sm">
            <span class="font-medium">{sc.count}</span>
            <span class="text-gray-500 ml-1">{sc.status?.replace(/_/g, ' ').toLowerCase()}</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>
