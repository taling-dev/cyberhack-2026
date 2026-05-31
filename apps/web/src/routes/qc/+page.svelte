<script lang="ts">
  import { t } from 'svelte-i18n';
  import { goto } from '$app/navigation';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { DashboardService } from '$lib/gen/simaops/dashboard/v1/dashboard_pb';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import DashboardIcon from '$lib/components/dashboard/DashboardIcon.svelte';
  import KpiCard from '$lib/components/dashboard/KpiCard.svelte';
  import { highlightOnChange } from '$lib/actions/highlightOnChange.svelte';

  const dashboardClient = createClient(DashboardService, transport);
  const lotClient = createClient(LotService, transport);

  // Status 4 = QC_REVIEW
  const lotsQuery = createQuery(() => ({
    queryKey: ['qc-review-lots'],
    queryFn: () => lotClient.listLots({ pageSize: 50, statusFilter: 4 }),
    refetchInterval: 15_000,
  }));

  const qcMetricsQuery = createQuery(() => ({
    queryKey: ['dashboard-qc'],
    queryFn: () => dashboardClient.getQCMetrics({ hours: 24 }),
    refetchInterval: 30_000,
  }));

  const lots = $derived(lotsQuery.data?.lots ?? []);

  // Power-user keyboard nav: j/k or arrows move the focused row, Enter/o opens
  // it. Opening still shows the full evidence + decision screen; there is no
  // blind bulk-approve (it would defeat the evidence-beside-decision rule).
  let focusedRow = $state(-1);

  function onQueueKeydown(e: KeyboardEvent) {
    if (lots.length === 0) return;
    const tag = (e.target as HTMLElement)?.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA') return;
    if (e.key === 'j' || e.key === 'ArrowDown') {
      e.preventDefault();
      focusedRow = Math.min(focusedRow + 1, lots.length - 1);
    } else if (e.key === 'k' || e.key === 'ArrowUp') {
      e.preventDefault();
      focusedRow = Math.max(focusedRow - 1, 0);
    } else if ((e.key === 'Enter' || e.key === 'o') && focusedRow >= 0) {
      e.preventDefault();
      goto(`/qc/${lots[focusedRow].id}`);
    }
  }

  function formatPercentRatio(value?: number | null) {
    return value == null ? '-' : `${Math.round(value * 1000) / 10}%`;
  }

  function formatQuantity(value: number) {
    return Number.isInteger(value)
      ? value.toLocaleString('en-US')
      : value.toLocaleString('en-US', { maximumFractionDigits: 2 });
  }
</script>

<svelte:window onkeydown={onQueueKeydown} />

<div class="space-y-5">
  <header class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
    <div>
      <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">Quality Control</p>
      <h1 class="mt-1 text-[28px] font-bold tracking-normal text-slate-950">QC Review</h1>
      <p class="mt-1 text-sm text-slate-600">Review AI inspection results before approval.</p>
    </div>

    <div class="flex items-center gap-3">
      <span class="rounded-md border border-slate-200 bg-white px-3 py-2 text-xs font-medium text-slate-500 shadow-sm">
        {lots.length} lots awaiting review
      </span>
      <button
        type="button"
        onclick={() => {
          lotsQuery.refetch();
          qcMetricsQuery.refetch();
        }}
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

  <section class="grid grid-cols-2 gap-3 lg:grid-cols-4">
    <KpiCard title="Waiting Review" value={lotsQuery.isLoading ? '-' : lots.length.toLocaleString('en-US')} icon="clock" tone="orange" loading={lotsQuery.isLoading} />
    <KpiCard title="Average AI Confidence" value={formatPercentRatio(qcMetricsQuery.data?.averageConfidence)} icon="bot" tone="purple" loading={qcMetricsQuery.isLoading} />
    <KpiCard title="Approved (24h)" value={(qcMetricsQuery.data?.passCount ?? 0).toLocaleString('en-US')} icon="check-circle" tone="green" loading={qcMetricsQuery.isLoading} />
    <KpiCard title="Rejected (24h)" value={(qcMetricsQuery.data?.failCount ?? 0).toLocaleString('en-US')} icon="shield" tone="red" loading={qcMetricsQuery.isLoading} />
  </section>

  {#if qcMetricsQuery.isError}
    <div class="rounded-lg border border-orange-200 bg-orange-50 px-4 py-3 text-sm text-orange-700">
      QC summary unavailable: {qcMetricsQuery.error?.message || $t('common.error')}
    </div>
  {/if}

  <section class="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
    <div class="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3">
      <div>
        <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">AI Review Queue</h2>
      </div>
      <div class="flex items-center gap-3">
        <span class="hidden text-xs text-slate-400 sm:inline">
          <kbd class="rounded border border-slate-200 bg-slate-50 px-1 font-mono">j</kbd>/<kbd class="rounded border border-slate-200 bg-slate-50 px-1 font-mono">k</kbd> move ·
          <kbd class="rounded border border-slate-200 bg-slate-50 px-1 font-mono">Enter</kbd> open
        </span>
        <span class="rounded-md bg-orange-100 px-2.5 py-1 text-xs font-semibold text-orange-700">{lots.length} pending</span>
      </div>
    </div>

    {#if lotsQuery.isLoading}
      <div class="divide-y divide-slate-100">
        {#each [1, 2, 3, 4, 5] as _}
          <div class="grid grid-cols-[1fr_1.15fr_.8fr_1fr_.85fr_.75fr] gap-4 px-4 py-4">
            {#each [1, 2, 3, 4, 5, 6] as __}
              <div class="h-4 animate-pulse rounded bg-slate-100"></div>
            {/each}
          </div>
        {/each}
      </div>
    {:else if lotsQuery.isError}
      <div class="m-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
        {lotsQuery.error?.message || $t('common.error')}
      </div>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full min-w-[920px] text-left text-sm">
          <thead class="border-b border-slate-200 bg-slate-50 text-xs font-semibold text-slate-500">
            <tr>
              <th class="px-4 py-3">Lot No.</th>
              <th class="px-4 py-3">Material</th>
              <th class="px-4 py-3">Type</th>
              <th class="px-4 py-3">Supplier</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            {#each lots as lot, i}
              <tr
                class="transition-colors hover:bg-slate-50 {i === focusedRow ? 'bg-blue-50 ring-2 ring-inset ring-blue-400' : ''}"
                use:highlightOnChange={lot.id}
              >
                <td class="px-4 py-3 align-middle">
                  <a href="/qc/{lot.id}" class="font-mono text-xs font-semibold text-blue-600 hover:underline">{lot.lotNumber}</a>
                </td>
                <td class="px-4 py-3 align-middle">
                  <div class="font-medium text-slate-950">{lot.materialName}</div>
                  <div class="mt-0.5 text-xs text-slate-500">{formatQuantity(lot.quantity)} {lot.unit}</div>
                </td>
                <td class="px-4 py-3 align-middle">
                  <span class="inline-flex rounded-md bg-slate-100 px-2 py-1 text-xs font-medium text-slate-700">
                    {$t(`material_type.${lot.materialType}`)}
                  </span>
                </td>
                <td class="px-4 py-3 align-middle text-slate-700">{lot.supplierName}</td>
                <td class="px-4 py-3 align-middle">
                  <span class="inline-flex items-center gap-1.5 rounded-md bg-orange-100 px-2.5 py-1 text-xs font-semibold text-orange-700 ring-1 ring-inset ring-orange-200">
                    <span class="size-1.5 rounded-full bg-orange-500"></span>
                    {$t('lot_status.4')}
                  </span>
                </td>
                <td class="px-4 py-3 text-right align-middle">
                  <a href="/qc/{lot.id}" class="inline-flex h-8 items-center gap-2 rounded-md bg-orange-600 px-3 text-xs font-semibold text-white shadow-sm transition-colors hover:bg-orange-700">
                    Review
                    <DashboardIcon name="arrow-right" class="size-3.5" />
                  </a>
                </td>
              </tr>
            {:else}
              <tr>
                <td colspan="6" class="px-4 py-16">
                  <div class="mx-auto flex max-w-sm flex-col items-center text-center">
                    <div class="flex size-12 items-center justify-center rounded-lg bg-emerald-100 text-emerald-700">
                      <DashboardIcon name="check-circle" class="size-6" />
                    </div>
                    <h2 class="mt-3 text-sm font-semibold text-slate-950">{$t('qc.no_queue')}</h2>
                    <p class="mt-1 text-sm text-slate-500">New AI inspections will appear here when supervisor review is needed.</p>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>
</div>
