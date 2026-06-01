<script lang="ts">
  import { page } from '$app/stores';
  import { t, locale } from 'svelte-i18n';
  import { onMount, tick } from 'svelte';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { DashboardService } from '$lib/gen/simaops/dashboard/v1/dashboard_pb';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { QCService } from '$lib/gen/simaops/qc/v1/qc_pb';
  import AIQueueCard from '$lib/components/dashboard/AIQueueCard.svelte';
  import CompactLotTable from '$lib/components/dashboard/CompactLotTable.svelte';
  import DashboardIcon from '$lib/components/dashboard/DashboardIcon.svelte';
  import FeatureStrip from '$lib/components/dashboard/FeatureStrip.svelte';
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

  const latestReviewLot = $derived(qcQueueQuery.data?.lots?.[0] ?? null);

  const latestQcJobQuery = createQuery(() => ({
    queryKey: ['qc-job-for-lot', latestReviewLot?.id],
    queryFn: () => qcClient.getQCJob({ lotId: latestReviewLot!.id, qcJobId: '' }),
    enabled: !!latestReviewLot?.id,
    retry: false,
    refetchInterval: 30_000,
  }));

  const latestQcResultQuery = createQuery(() => ({
    queryKey: ['qc-result', latestQcJobQuery.data?.job?.id],
    queryFn: () => qcClient.getQCResult({ qcJobId: latestQcJobQuery.data!.job!.id }),
    enabled: !!latestQcJobQuery.data?.job?.id,
    retry: false,
    refetchInterval: (query) => (query.state.data?.result ? false : 15_000),
  }));

  const latestImageUrlQuery = createQuery(() => ({
    queryKey: ['qc-image-url', latestQcJobQuery.data?.job?.imageObjectKey],
    queryFn: () => qcClient.createQCViewUrl({ objectKey: latestQcJobQuery.data!.job!.imageObjectKey }),
    enabled: !!latestQcJobQuery.data?.job?.imageObjectKey,
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
  const userName = $derived($page.data.user?.name ?? 'Operator');
  const today = new Date();
  const dateLabel = $derived(
    new Intl.DateTimeFormat($locale === 'id' ? 'id-ID' : 'en-US', {
      day: '2-digit',
      month: 'short',
      year: 'numeric',
    }).format(today)
  );
  const languageOptions = [
    { code: 'en', label: 'English', shortLabel: 'EN' },
    { code: 'id', label: 'Bahasa Indonesia', shortLabel: 'ID' },
  ];
  let languageOpen = $state(false);
  let languageButton = $state<HTMLButtonElement | null>(null);
  let languageMenu = $state<HTMLDivElement | null>(null);

  const warehouseUtilization = $derived(
    whQuery.data?.totalCapacity ? whQuery.data.totalOccupied / whQuery.data.totalCapacity : null
  );

  onMount(() => {
    function closeOnOutsideClick(event: MouseEvent) {
      const target = event.target as Node;
      if (!languageOpen) return;
      if (languageButton?.contains(target) || languageMenu?.contains(target)) return;
      languageOpen = false;
    }

    document.addEventListener('mousedown', closeOnOutsideClick);
    return () => document.removeEventListener('mousedown', closeOnOutsideClick);
  });

  function formatNumber(value?: number | null) {
    return value == null ? '-' : value.toLocaleString('en-US');
  }

  function formatPercentRatio(value?: number | null) {
    return value == null ? '-' : `${Math.round(value * 1000) / 10}%`;
  }

  async function focusLanguageOption(code = $locale) {
    await tick();
    languageMenu?.querySelector<HTMLButtonElement>(`[data-locale="${code}"]`)?.focus();
  }

  async function openLanguageMenu(focusCode = $locale) {
    languageOpen = true;
    await focusLanguageOption(focusCode);
  }

  function selectLanguage(code: string) {
    locale.set(code);
    languageOpen = false;
    languageButton?.focus();
  }

  function handleLanguageButtonKeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      openLanguageMenu();
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      openLanguageMenu(languageOptions[languageOptions.length - 1].code);
    } else if (event.key === 'Escape') {
      languageOpen = false;
    }
  }

  function handleLanguageMenuKeydown(event: KeyboardEvent) {
    const currentIndex = languageOptions.findIndex((option) => option.code === document.activeElement?.getAttribute('data-locale'));
    if (event.key === 'Escape') {
      event.preventDefault();
      languageOpen = false;
      languageButton?.focus();
    } else if (event.key === 'ArrowDown') {
      event.preventDefault();
      focusLanguageOption(languageOptions[(currentIndex + 1 + languageOptions.length) % languageOptions.length].code);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      focusLanguageOption(languageOptions[(currentIndex - 1 + languageOptions.length) % languageOptions.length].code);
    }
  }
</script>

<div class="flex min-h-screen flex-col gap-3 xl:h-full xl:min-h-0 xl:overflow-hidden">
  <header class="flex shrink-0 flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
    <div>
      <h1 class="text-[28px] font-bold tracking-normal text-slate-950">{$t('nav.dashboard')}</h1>
      <p class="mt-1 text-sm text-slate-600">Welcome back, {userName}</p>
    </div>

    <div class="flex flex-wrap items-center gap-3 text-sm text-slate-600">
      <span class="flex items-center gap-2 font-medium">
        <span class="size-2.5 rounded-full {isLoading ? 'animate-pulse bg-blue-500' : 'bg-green-500'}"></span>
        {$t('dashboard.last_updated')}: {lastUpdatedLabel}
      </span>
      <time
        datetime={today.toISOString().slice(0, 10)}
        class="flex h-10 items-center gap-3 rounded-md border border-slate-200 bg-white px-3 text-slate-900 shadow-sm"
        aria-label="Current date"
        title="Current date"
      >
        <DashboardIcon name="calendar" class="size-5 text-slate-700" />
        {dateLabel}
      </time>
      <div class="relative">
        <button
          bind:this={languageButton}
          type="button"
          onclick={() => languageOpen ? languageOpen = false : openLanguageMenu()}
          onkeydown={handleLanguageButtonKeydown}
          class="flex h-10 items-center gap-3 rounded-md border border-slate-200 bg-white px-3 text-slate-900 shadow-sm transition-colors hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-100"
          aria-haspopup="menu"
          aria-expanded={languageOpen}
          aria-controls="dashboard-language-menu"
        >
          <DashboardIcon name="globe" class="size-5 text-slate-700" />
          <span class="hidden sm:inline">{$t(`locale.${$locale}`)}</span>
          <span class="font-semibold sm:hidden">{languageOptions.find((option) => option.code === $locale)?.shortLabel ?? 'EN'}</span>
          <svg viewBox="0 0 24 24" class="size-4 text-slate-500" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="m6 9 6 6 6-6" />
          </svg>
        </button>

        {#if languageOpen}
          <div
            bind:this={languageMenu}
            id="dashboard-language-menu"
            role="menu"
            aria-label="Select language"
            tabindex="-1"
            onkeydown={handleLanguageMenuKeydown}
            class="absolute right-0 z-20 mt-2 w-56 overflow-hidden rounded-lg border border-slate-200 bg-white p-1 shadow-lg"
          >
            {#each languageOptions as option}
              <button
                type="button"
                role="menuitemradio"
                aria-checked={$locale === option.code}
                data-locale={option.code}
                onclick={() => selectLanguage(option.code)}
                class="flex w-full items-center justify-between rounded-md px-3 py-2 text-left text-sm transition-colors hover:bg-slate-50 focus:bg-slate-50 focus:outline-none {$locale === option.code ? 'font-semibold text-blue-700' : 'text-slate-700'}"
              >
                <span>{option.label}</span>
                {#if $locale === option.code}
                  <span class="size-2 rounded-full bg-blue-600" aria-hidden="true"></span>
                {/if}
              </button>
            {/each}
          </div>
        {/if}
      </div>
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

  <section class="grid shrink-0 grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
    <KpiCard title="Total Lot" value={formatNumber(opsQuery.data?.totalLots)} icon="cube" tone="purple" loading={opsQuery.isLoading} />
    <KpiCard title="Waiting for QC" value={formatNumber(opsQuery.data?.lotsAwaitingQc)} icon="clock" tone="orange" loading={opsQuery.isLoading} />
    <KpiCard title="Production Ready" value={formatNumber(opsQuery.data?.lotsReadyForProduction)} icon="check-circle" tone="green" loading={opsQuery.isLoading} />
    <KpiCard title="QC Pass Rate (24h)" value={formatPercentRatio(qcQuery.data?.passRate)} icon="percent" tone="blue" loading={qcQuery.isLoading} />
    <KpiCard title="AI Confidence Rate" value={formatPercentRatio(qcQuery.data?.averageConfidence)} icon="bot" tone="red" loading={qcQuery.isLoading} />
    <KpiCard title="Warehouse Utilization" value={formatPercentRatio(warehouseUtilization)} icon="warehouse" tone="emerald" loading={whQuery.isLoading} />
  </section>

  <div class="grid flex-1 grid-cols-1 gap-3 overflow-visible xl:min-h-0 xl:grid-rows-[minmax(0,1fr)_minmax(0,1fr)_auto] xl:overflow-hidden">
    <div class="grid min-h-0 grid-cols-1 gap-3 xl:grid-cols-[1.15fr_.95fr_.95fr]">
      <QCMetricsTrendCard
        passCount={qcQuery.data?.passCount ?? 0}
        reviewCount={qcQuery.data?.reviewCount ?? 0}
        failCount={qcQuery.data?.failCount ?? 0}
        loading={qcQuery.isLoading}
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
        lot={latestReviewLot}
        result={latestQcResultQuery.data?.result ?? null}
        imageUrl={latestImageUrlQuery.data?.viewUrl ?? ''}
        loading={qcQueueQuery.isLoading || latestQcJobQuery.isLoading || latestQcResultQuery.isLoading}
        unavailable={!latestReviewLot || latestQcJobQuery.isError || latestQcResultQuery.isError}
      />
      <CompactLotTable lots={newestLotsQuery.data?.lots ?? []} loading={newestLotsQuery.isLoading} />
    </div>

    <div class="hidden shrink-0 overflow-hidden xl:block">
      <FeatureStrip />
    </div>
  </div>
</div>
