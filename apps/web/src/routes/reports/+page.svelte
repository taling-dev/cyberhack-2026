<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { DashboardService } from '$lib/gen/simaops/dashboard/v1/dashboard_pb';
  import DashboardIcon from '$lib/components/dashboard/DashboardIcon.svelte';
  import KpiCard from '$lib/components/dashboard/KpiCard.svelte';
  import StatusDistributionCard from '$lib/components/dashboard/StatusDistributionCard.svelte';
  import WarehouseCapacityCard from '$lib/components/dashboard/WarehouseCapacityCard.svelte';

  const dashboardClient = createClient(DashboardService, transport);

  const windowOptions = [
    { label: '24h', hours: 24 },
    { label: '7d', hours: 168 },
    { label: '30d', hours: 720 },
  ];

  let reportHours = $state(24);

  const opsQuery = createQuery(() => ({
    queryKey: ['dashboard-ops'],
    queryFn: () => dashboardClient.getOpsDashboard({}),
    refetchInterval: 30_000,
  }));

  const qcQuery = createQuery(() => ({
    queryKey: ['dashboard-qc', reportHours],
    queryFn: () => dashboardClient.getQCMetrics({ hours: reportHours }),
    refetchInterval: 30_000,
  }));

  const warehouseQuery = createQuery(() => ({
    queryKey: ['dashboard-warehouse'],
    queryFn: () => dashboardClient.getWarehouseMetrics({}),
    refetchInterval: 30_000,
  }));

  const warehouseUtilization = $derived(
    warehouseQuery.data?.totalCapacity
      ? warehouseQuery.data.totalOccupied / warehouseQuery.data.totalCapacity
      : null
  );

  const warehouseAssigned = $derived(statusCount('WAREHOUSE_ASSIGNED'));
  const blockedLots = $derived(statusCount('BLOCKED'));
  const readyLots = $derived(opsQuery.data?.lotsReadyForProduction ?? 0);
  const waitingQc = $derived(opsQuery.data?.lotsAwaitingQc ?? 0);
  const readinessTotal = $derived(readyLots + warehouseAssigned + blockedLots + waitingQc);

  const lastUpdated = $derived(
    [opsQuery.dataUpdatedAt, qcQuery.dataUpdatedAt, warehouseQuery.dataUpdatedAt]
      .filter(Boolean)
      .reduce((max, time) => Math.max(max, time), 0)
  );
  const lastUpdatedLabel = $derived(lastUpdated ? new Date(lastUpdated).toLocaleTimeString('en-US') : '-');
  const anyError = $derived(opsQuery.isError || qcQuery.isError || warehouseQuery.isError);
  const selectedWindowLabel = $derived(windowOptions.find((option) => option.hours === reportHours)?.label ?? '24h');

  function statusCount(status: string) {
    return opsQuery.data?.lotsByStatus?.find((item) => item.status === status)?.count ?? 0;
  }

  function formatNumber(value?: number | null) {
    return value == null ? '-' : value.toLocaleString('en-US');
  }

  function formatPercentRatio(value?: number | null) {
    return value == null ? '-' : `${Math.round(value * 1000) / 10}%`;
  }

  function ratioPercent(value: number, total: number) {
    return total > 0 ? Math.round((value / total) * 100) : 0;
  }

  function refreshAll() {
    opsQuery.refetch();
    qcQuery.refetch();
    warehouseQuery.refetch();
  }
</script>

<div class="space-y-5">
  <header class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
    <div>
      <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">Management Analytics</p>
      <h1 class="mt-1 text-[28px] font-bold tracking-normal text-slate-950">{$t('nav.reports')}</h1>
      <p class="mt-1 text-sm text-slate-600">Analyze QC performance, warehouse utilization, and production readiness.</p>
    </div>

    <div class="flex flex-wrap items-center gap-3">
      <span class="rounded-md border border-slate-200 bg-white px-3 py-2 text-xs font-medium text-slate-500 shadow-sm">
        Updated {lastUpdatedLabel}
      </span>
      <label class="flex h-10 items-center gap-2 rounded-md border border-slate-200 bg-white px-3 text-sm font-medium text-slate-600 shadow-sm">
        <span>QC window</span>
        <select
          bind:value={reportHours}
          class="h-8 rounded-md border border-slate-200 bg-slate-50 px-2 text-sm font-semibold text-slate-800 outline-none transition-colors focus:border-blue-400 focus:ring-2 focus:ring-blue-100"
          aria-label="QC metric window"
        >
          {#each windowOptions as option}
            <option value={option.hours}>{option.label}</option>
          {/each}
        </select>
      </label>
      <button
        type="button"
        onclick={refreshAll}
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

  {#if anyError}
    <div class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
      Reports summary unavailable: {opsQuery.error?.message || qcQuery.error?.message || warehouseQuery.error?.message || $t('common.error')}
    </div>
  {/if}

  <section class="grid grid-cols-2 gap-3 lg:grid-cols-4">
    <KpiCard title="QC Pass Rate" value={formatPercentRatio(qcQuery.data?.passRate)} icon="percent" tone="blue" loading={qcQuery.isLoading} />
    <KpiCard title="Avg AI Confidence" value={formatPercentRatio(qcQuery.data?.averageConfidence)} icon="bot" tone="purple" loading={qcQuery.isLoading} />
    <KpiCard title="Warehouse Utilization" value={formatPercentRatio(warehouseUtilization)} icon="warehouse" tone="orange" loading={warehouseQuery.isLoading} />
    <KpiCard title="Production Ready" value={formatNumber(opsQuery.data?.lotsReadyForProduction)} icon="check-circle" tone="green" loading={opsQuery.isLoading} />
  </section>

  <div class="grid gap-4 xl:grid-cols-[1.05fr_.95fr]">
    <section class="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
      <div class="flex items-start justify-between gap-3">
        <div>
          <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">QC Performance Summary</h2>
          <p class="mt-1 text-xs text-slate-500">Current {selectedWindowLabel} AI inspection outcomes from the QC metrics API.</p>
        </div>
        <DashboardIcon name="chart" class="size-5 text-slate-400" />
      </div>

      {#if qcQuery.isLoading}
        <div class="mt-5 space-y-4">
          {#each [1, 2, 3] as _}
            <div class="h-12 animate-pulse rounded-lg bg-slate-100"></div>
          {/each}
        </div>
      {:else if qcQuery.isError}
        <div class="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {qcQuery.error?.message || $t('common.error')}
        </div>
      {:else}
        {@const qcTotal = qcQuery.data?.totalJobs ?? 0}
        <div class="mt-5 grid gap-3 sm:grid-cols-3">
          <article class="rounded-lg border border-emerald-200 bg-emerald-50 p-3">
            <div class="flex items-center justify-between gap-3">
              <span class="text-sm font-semibold text-emerald-800">Approved</span>
              <span class="text-xl font-bold text-slate-950">{formatNumber(qcQuery.data?.passCount)}</span>
            </div>
            <div class="mt-3 h-2 overflow-hidden rounded-full bg-white">
              <div class="h-full rounded-full bg-emerald-500" style="width: {ratioPercent(qcQuery.data?.passCount ?? 0, qcTotal)}%"></div>
            </div>
          </article>
          <article class="rounded-lg border border-orange-200 bg-orange-50 p-3">
            <div class="flex items-center justify-between gap-3">
              <span class="text-sm font-semibold text-orange-800">Review</span>
              <span class="text-xl font-bold text-slate-950">{formatNumber(qcQuery.data?.reviewCount)}</span>
            </div>
            <div class="mt-3 h-2 overflow-hidden rounded-full bg-white">
              <div class="h-full rounded-full bg-orange-500" style="width: {ratioPercent(qcQuery.data?.reviewCount ?? 0, qcTotal)}%"></div>
            </div>
          </article>
          <article class="rounded-lg border border-red-200 bg-red-50 p-3">
            <div class="flex items-center justify-between gap-3">
              <span class="text-sm font-semibold text-red-800">Rejected</span>
              <span class="text-xl font-bold text-slate-950">{formatNumber(qcQuery.data?.failCount)}</span>
            </div>
            <div class="mt-3 h-2 overflow-hidden rounded-full bg-white">
              <div class="h-full rounded-full bg-red-500" style="width: {ratioPercent(qcQuery.data?.failCount ?? 0, qcTotal)}%"></div>
            </div>
          </article>
        </div>

        <div class="mt-4 rounded-lg border border-slate-200 bg-slate-50 px-4 py-3">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p class="text-sm font-semibold text-slate-950">{formatNumber(qcTotal)} total QC jobs</p>
              <p class="mt-1 text-xs text-slate-500">Trend history is not exposed by the current API, so this report shows the selected window snapshot only.</p>
            </div>
            <span class="rounded-md bg-purple-100 px-2.5 py-1 text-xs font-semibold text-purple-700">
              {formatPercentRatio(qcQuery.data?.averageConfidence)} confidence
            </span>
          </div>
        </div>
      {/if}
    </section>

    <StatusDistributionCard
      statuses={opsQuery.data?.lotsByStatus ?? []}
      total={opsQuery.data?.totalLots ?? 0}
      loading={opsQuery.isLoading}
    />
  </div>

  <div class="grid gap-4 xl:grid-cols-[.95fr_1.05fr]">
    <WarehouseCapacityCard
      zones={warehouseQuery.data?.zones ?? []}
      totalCapacity={warehouseQuery.data?.totalCapacity ?? 0}
      totalOccupied={warehouseQuery.data?.totalOccupied ?? 0}
      loading={warehouseQuery.isLoading}
    />

    <section class="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
      <div class="flex items-start justify-between gap-3">
        <div>
          <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">Production Readiness</h2>
          <p class="mt-1 text-xs text-slate-500">Operational readiness from current lot status counts.</p>
        </div>
        <DashboardIcon name="activity" class="size-5 text-slate-400" />
      </div>

      {#if opsQuery.isLoading}
        <div class="mt-5 space-y-3">
          {#each [1, 2, 3, 4] as _}
            <div class="h-11 animate-pulse rounded-lg bg-slate-100"></div>
          {/each}
        </div>
      {:else if opsQuery.isError}
        <div class="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {opsQuery.error?.message || $t('common.error')}
        </div>
      {:else if readinessTotal === 0}
        <div class="mt-4 flex min-h-48 flex-col items-center justify-center rounded-lg bg-slate-50 px-4 text-center">
          <div class="flex size-12 items-center justify-center rounded-lg bg-slate-100 text-slate-500">
            <DashboardIcon name="database" class="size-6" />
          </div>
          <h3 class="mt-3 text-sm font-semibold text-slate-950">No readiness data yet</h3>
          <p class="mt-1 max-w-sm text-sm text-slate-500">Lot status metrics will appear here once operational activity is recorded.</p>
        </div>
      {:else}
        <div class="mt-5 space-y-3">
          <div>
            <div class="mb-1 flex items-center justify-between text-sm">
              <span class="font-semibold text-emerald-700">Ready for production</span>
              <span class="font-bold text-slate-950">{formatNumber(readyLots)}</span>
            </div>
            <div class="h-2 overflow-hidden rounded-full bg-slate-100">
              <div class="h-full rounded-full bg-emerald-500" style="width: {ratioPercent(readyLots, readinessTotal)}%"></div>
            </div>
          </div>
          <div>
            <div class="mb-1 flex items-center justify-between text-sm">
              <span class="font-semibold text-purple-700">Warehouse assigned</span>
              <span class="font-bold text-slate-950">{formatNumber(warehouseAssigned)}</span>
            </div>
            <div class="h-2 overflow-hidden rounded-full bg-slate-100">
              <div class="h-full rounded-full bg-purple-500" style="width: {ratioPercent(warehouseAssigned, readinessTotal)}%"></div>
            </div>
          </div>
          <div>
            <div class="mb-1 flex items-center justify-between text-sm">
              <span class="font-semibold text-orange-700">Waiting QC</span>
              <span class="font-bold text-slate-950">{formatNumber(waitingQc)}</span>
            </div>
            <div class="h-2 overflow-hidden rounded-full bg-slate-100">
              <div class="h-full rounded-full bg-orange-500" style="width: {ratioPercent(waitingQc, readinessTotal)}%"></div>
            </div>
          </div>
          <div>
            <div class="mb-1 flex items-center justify-between text-sm">
              <span class="font-semibold text-red-700">Blocked lots</span>
              <span class="font-bold text-slate-950">{formatNumber(blockedLots)}</span>
            </div>
            <div class="h-2 overflow-hidden rounded-full bg-slate-100">
              <div class="h-full rounded-full bg-red-500" style="width: {ratioPercent(blockedLots, readinessTotal)}%"></div>
            </div>
          </div>
        </div>
      {/if}
    </section>
  </div>

  <section class="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">Available Report Scope</h2>
        <p class="mt-1 text-xs text-slate-500">This page uses live operational APIs only. Unsupported exports and historical trend charts are intentionally not shown.</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <span class="rounded-md bg-blue-100 px-2.5 py-1 text-xs font-semibold text-blue-700">Dashboard metrics</span>
        <span class="rounded-md bg-emerald-100 px-2.5 py-1 text-xs font-semibold text-emerald-700">Live refresh</span>
        <span class="rounded-md bg-slate-100 px-2.5 py-1 text-xs font-semibold text-slate-700">No fabricated data</span>
      </div>
    </div>
  </section>
</div>

