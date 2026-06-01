<script lang="ts">
  import { t, locale } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { DashboardService } from '$lib/gen/simaops/dashboard/v1/dashboard_pb';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { QCService } from '$lib/gen/simaops/qc/v1/qc_pb';
  import AIQueueCard from '$lib/components/dashboard/AIQueueCard.svelte';
  import CompactLotTable from '$lib/components/dashboard/CompactLotTable.svelte';
  import DashboardIcon from '$lib/components/dashboard/DashboardIcon.svelte';
  import KpiCard from '$lib/components/dashboard/KpiCard.svelte';
  import LatestInspectionCard from '$lib/components/dashboard/LatestInspectionCard.svelte';
  import QCMetricsTrendCard from '$lib/components/dashboard/QCMetricsTrendCard.svelte';
  import StatusDistributionCard from '$lib/components/dashboard/StatusDistributionCard.svelte';
  import WarehouseCapacityCard from '$lib/components/dashboard/WarehouseCapacityCard.svelte';

  const dashboardClient = createClient(DashboardService, transport);
  const lotClient = createClient(LotService, transport);
  const qcClient = createClient(QCService, transport);

  // Preserve the existing DashboardService queries and query keys.
  const opsQuery = createQuery(() => ({
    queryKey: ['dashboard-ops'],
    queryFn: () => dashboardClient.getOpsDashboard({}),
    refetchInterval: 30_000,
  }));

  const qcQuery = createQuery(() => ({
    queryKey: ['dashboard-qc'],
    queryFn: () => dashboardClient.getQCMetrics({ hours: 24 }),
    refetchInterval: 30_000,
  }));

  const qcTrendQuery = createQuery(() => ({
    queryKey: ['dashboard-qc-trend'],
    queryFn: () => dashboardClient.getQCTrend({ days: 7 }),
    refetchInterval: 60_000,
  }));

  const whQuery = createQuery(() => ({
    queryKey: ['dashboard-warehouse'],
    queryFn: () => dashboardClient.getWarehouseMetrics({}),
    refetchInterval: 30_000,
  }));

  // Dashboard-only supporting reads. These use existing APIs and query-key
  // prefixes already invalidated by the realtime store.
  const newestLotsQuery = createQuery(() => ({
    queryKey: ['lots', 'dashboard-newest'],
    queryFn: () => lotClient.listLots({ pageSize: 5 }),
    refetchInterval: 30_000,
  }));

  const qcQueueQuery = createQuery(() => ({
    queryKey: ['qc-review-lots', 'dashboard'],
    queryFn: () => lotClient.listLots({ pageSize: 4, statusFilter: 4 }),
    refetchInterval: 15_000,
  }));

  const latestInspectionQuery = createQuery(() => ({
    queryKey: ['dashboard-latest-inspection'],
    queryFn: () => dashboardClient.getLatestInspection({}),
    refetchInterval: 30_000,
  }));
  const latestInspection = $derived(latestInspectionQuery.data);

  const latestImageUrlQuery = createQuery(() => ({
    queryKey: ['qc-image-url', latestInspection?.imageObjectKey],
    queryFn: () => qcClient.createQCViewUrl({ objectKey: latestInspection!.imageObjectKey }),
    enabled: !!latestInspection?.imageObjectKey,
    staleTime: 10 * 60 * 1000,
    retry: false,
  }));

  const lastUpdated = $derived(
    [opsQuery.dataUpdatedAt, qcQuery.dataUpdatedAt, whQuery.dataUpdatedAt]
      .filter(Boolean)
      .reduce((max, time) => Math.max(max, time), 0)
  );
  const lastUpdatedLabel = $derived(lastUpdated ? new Date(lastUpdated).toLocaleTimeString('en-US') : '-');

  const isLoading = $derived(opsQuery.isLoading || qcQuery.isLoading || whQuery.isLoading);
  const isError = $derived(opsQuery.isError || qcQuery.isError || whQuery.isError);

  const dateLabel = new Intl.DateTimeFormat('en-US', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
  }).format(new Date());

  const warehouseUtilization = $derived(
    whQuery.data?.totalCapacity ? whQuery.data.totalOccupied / whQuery.data.totalCapacity : null
  );

  function formatNumber(value?: number | null) {
    return value == null ? '-' : value.toLocaleString('en-US');
  }

  function formatPercentRatio(value?: number | null) {
    return value == null ? '-' : `${Math.round(value * 1000) / 10}%`;
  }
</script>

<div class="flex min-h-screen flex-col gap-3 xl:h-full xl:min-h-0 xl:overflow-hidden">
  <header class="flex shrink-0 flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
    <div>
      <h1 class="text-[28px] font-bold tracking-normal text-slate-950">{$t('nav.dashboard')}</h1>
    </div>

    <div class="flex flex-wrap items-center gap-3 text-sm text-slate-600">
      <span class="flex items-center gap-2 font-medium">
        <span class="size-2.5 rounded-full {isLoading ? 'animate-pulse bg-blue-500' : 'bg-green-500'}"></span>
        {$t('dashboard.last_updated')}: {lastUpdatedLabel}
      </span>
      <span class="flex h-10 items-center gap-3 rounded-md border border-slate-200 bg-white px-3 text-slate-900 shadow-sm">
        <DashboardIcon name="calendar" class="size-5 text-slate-700" />
        {dateLabel}
      </span>
      <span class="flex h-10 items-center gap-3 rounded-md border border-slate-200 bg-white px-3 text-slate-900 shadow-sm">
        <DashboardIcon name="globe" class="size-5 text-slate-700" />
        {$t(`locale.${$locale}`)}
      </span>
      <span class="relative flex size-10 items-center justify-center rounded-md border border-slate-200 bg-white text-slate-900 shadow-sm">
        <DashboardIcon name="bell" class="size-5" />
        {#if (qcQueueQuery.data?.lots?.length ?? 0) > 0}
          <span class="absolute right-2 top-2 size-2.5 rounded-full bg-red-500"></span>
        {/if}
      </span>
    </div>
  </header>

  {#if isError}
    <div class="shrink-0 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
      {$t('common.error')}: {opsQuery.error?.message || qcQuery.error?.message || whQuery.error?.message}
    </div>
  {/if}

  <section class="grid shrink-0 grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-7">
    <KpiCard title={$t('kpi.todays_intake')} value={formatNumber(opsQuery.data?.todayIntakeCount)} icon="cube" tone="blue" loading={opsQuery.isLoading} href="/lots" emphasis />
    <KpiCard title={$t('kpi.total_lots')} value={formatNumber(opsQuery.data?.totalLots)} icon="cube" tone="purple" loading={opsQuery.isLoading} href="/lots" emphasis />
    <KpiCard title={$t('kpi.waiting_qc')} value={formatNumber(opsQuery.data?.lotsAwaitingQc)} icon="clock" tone="orange" loading={opsQuery.isLoading} href="/qc" emphasis />
    <KpiCard title={$t('kpi.production_ready')} value={formatNumber(opsQuery.data?.lotsReadyForProduction)} icon="check-circle" tone="green" loading={opsQuery.isLoading} href="/warehouse" emphasis />
    <KpiCard title={$t('kpi.qc_pass_rate')} value={formatPercentRatio(qcQuery.data?.passRate)} icon="percent" tone="blue" loading={qcQuery.isLoading} href="/qc" />
    <KpiCard title={$t('kpi.ai_confidence')} value={formatPercentRatio(qcQuery.data?.averageConfidence)} icon="bot" tone="red" loading={qcQuery.isLoading} href="/qc" />
    <KpiCard title={$t('kpi.warehouse_util')} value={formatPercentRatio(warehouseUtilization)} icon="warehouse" tone="emerald" loading={whQuery.isLoading} href="/warehouse" />
  </section>

  <div class="grid flex-1 grid-cols-1 gap-3 overflow-visible xl:min-h-0 xl:grid-rows-[minmax(0,1fr)_minmax(0,1fr)] xl:overflow-hidden">
    <div class="grid min-h-0 grid-cols-1 gap-3 xl:grid-cols-[1.15fr_.95fr_.95fr]">
      <QCMetricsTrendCard
        days={qcTrendQuery.data?.days ?? []}
        loading={qcTrendQuery.isLoading}
      />
      <StatusDistributionCard
        statuses={opsQuery.data?.lotsByStatus ?? []}
        total={opsQuery.data?.totalLots ?? 0}
        loading={opsQuery.isLoading}
      />
      <WarehouseCapacityCard
        zones={whQuery.data?.zones ?? []}
        totalCapacity={whQuery.data?.totalCapacity ?? 0}
        totalOccupied={whQuery.data?.totalOccupied ?? 0}
        loading={whQuery.isLoading}
      />
    </div>

    <div class="grid min-h-0 grid-cols-1 gap-3 xl:grid-cols-[.75fr_.95fr_2fr]">
      <AIQueueCard lots={qcQueueQuery.data?.lots ?? []} loading={qcQueueQuery.isLoading} />
      <LatestInspectionCard
        inspection={latestInspection ?? null}
        imageUrl={latestImageUrlQuery.data?.viewUrl ?? ''}
        loading={latestInspectionQuery.isLoading}
        unavailable={latestInspectionQuery.isError}
      />
      <CompactLotTable lots={newestLotsQuery.data?.lots ?? []} loading={newestLotsQuery.isLoading} />
    </div>
  </div>
</div>
