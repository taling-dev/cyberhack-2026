<script lang="ts">
  import { t } from 'svelte-i18n';
  import { page } from '$app/stores';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { highlightOnChange } from '$lib/actions/highlightOnChange.svelte';

  const client = createClient(LotService, transport);

  let statusFilter = $state(0);
  let materialFilter = $state(0);
  let pageToken = $state('');
  let pageHistory = $state<string[]>(['']);

  const lotsQuery = createQuery(() => ({
    queryKey: ['lots', statusFilter, materialFilter, pageToken],
    queryFn: () => client.listLots({
      pageSize: 50,
      pageToken,
      statusFilter,
      materialTypeFilter: materialFilter,
    }),
  }));

  const statusMeta: Record<number, { tone: string; dot: string }> = {
    1: { tone: 'bg-slate-100 text-slate-700 ring-slate-200', dot: 'bg-slate-400' },
    2: { tone: 'bg-yellow-100 text-yellow-800 ring-yellow-200', dot: 'bg-yellow-500' },
    3: { tone: 'bg-blue-100 text-blue-700 ring-blue-200', dot: 'bg-blue-500' },
    4: { tone: 'bg-orange-100 text-orange-700 ring-orange-200', dot: 'bg-orange-500' },
    5: { tone: 'bg-emerald-100 text-emerald-700 ring-emerald-200', dot: 'bg-emerald-500' },
    6: { tone: 'bg-red-100 text-red-700 ring-red-200', dot: 'bg-red-500' },
    7: { tone: 'bg-purple-100 text-purple-700 ring-purple-200', dot: 'bg-purple-500' },
    8: { tone: 'bg-green-100 text-green-700 ring-green-200', dot: 'bg-green-500' },
    9: { tone: 'bg-red-100 text-red-800 ring-red-200', dot: 'bg-red-600' },
  };

  const statusOptions = [1, 2, 3, 4, 5, 6, 7, 8, 9];
  const materialOptions = [1, 2, 3, 4];

  function nextPage() {
    const next = lotsQuery.data?.nextPageToken;
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

  function resetPaging() {
    pageToken = '';
    pageHistory = [''];
  }

  function clearFilters() {
    statusFilter = 0;
    materialFilter = 0;
    resetPaging();
  }

  function formatDate(seconds?: bigint | number | string) {
    if (!seconds) return '-';
    return new Date(Number(seconds) * 1000).toLocaleDateString('en-US', {
      month: '2-digit',
      day: '2-digit',
      year: 'numeric',
    });
  }

  function formatQuantity(value: number) {
    return Number.isInteger(value)
      ? value.toLocaleString('en-US')
      : value.toLocaleString('en-US', { maximumFractionDigits: 2 });
  }

  function statusClass(status: number) {
    return statusMeta[status]?.tone ?? statusMeta[1].tone;
  }

  function statusDot(status: number) {
    return statusMeta[status]?.dot ?? statusMeta[1].dot;
  }

  // AI Score badge tone from the lot's latest QC recommendation.
  function qcTone(rec: string) {
    if (rec === 'PASS') return 'bg-emerald-100 text-emerald-700';
    if (rec === 'REVIEW') return 'bg-orange-100 text-orange-700';
    if (rec === 'FAIL') return 'bg-red-100 text-red-700';
    return 'bg-slate-100 text-slate-500';
  }

  const lots = $derived(lotsQuery.data?.lots ?? []);
  const hasNext = $derived(!!lotsQuery.data?.nextPageToken);
  const hasPrev = $derived(pageHistory.length > 1);
  const filtersActive = $derived(statusFilter !== 0 || materialFilter !== 0);

  const canCreate = $derived(
    ($page.data.user?.roles ?? []).some((role: string) => role === 'OPERATOR' || role === 'ADMIN')
  );
</script>

<div class="space-y-5">
  <header class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
    <div>
      <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">Operations</p>
      <h1 class="mt-1 text-[28px] font-bold tracking-normal text-slate-950">{$t('lot.list_title')}</h1>
    </div>

    <div class="flex items-center gap-3">
      <span class="rounded-md border border-slate-200 bg-white px-3 py-2 text-xs font-medium text-slate-500 shadow-sm">
        {lots.length} {lots.length === 1 ? $t('common.result') : $t('common.results')} on this page
      </span>
      {#if canCreate}
        <a href="/lots/new" class="inline-flex h-10 items-center gap-2 rounded-md bg-blue-600 px-4 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-blue-700">
          <svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
            <path d="M12 5v14" />
            <path d="M5 12h14" />
          </svg>
          {$t('lot.create_button').replace('+ ', '')}
        </a>
      {/if}
    </div>
  </header>

  <section class="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
    <div class="flex flex-col gap-3 xl:flex-row xl:items-center">
      <div class="flex min-w-0 flex-1 flex-col gap-3 sm:flex-row">
        <div class="min-w-[190px]">
          <label for="status-filter" class="mb-1 block text-xs font-semibold text-slate-500">Status</label>
          <select
            id="status-filter"
            bind:value={statusFilter}
            onchange={resetPaging}
            class="h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-700 shadow-sm outline-none transition-colors hover:bg-slate-50 focus:border-blue-400 focus:ring-2 focus:ring-blue-100"
          >
            <option value={0}>{$t('lot.filter_all_statuses')}</option>
            {#each statusOptions as status}
              <option value={status}>{$t(`lot_status.${status}`)}</option>
            {/each}
          </select>
        </div>

        <div class="min-w-[210px]">
          <label for="material-filter" class="mb-1 block text-xs font-semibold text-slate-500">Material Type</label>
          <select
            id="material-filter"
            bind:value={materialFilter}
            onchange={resetPaging}
            class="h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-700 shadow-sm outline-none transition-colors hover:bg-slate-50 focus:border-blue-400 focus:ring-2 focus:ring-blue-100"
          >
            <option value={0}>{$t('lot.filter_all_materials')}</option>
            {#each materialOptions as material}
              <option value={material}>{$t(`material_type.${material}`)}</option>
            {/each}
          </select>
        </div>
      </div>

      <div class="flex items-end gap-2">
        {#if filtersActive}
          <button
            type="button"
            onclick={clearFilters}
            class="h-10 rounded-md border border-slate-200 bg-white px-3 text-sm font-medium text-slate-600 shadow-sm transition-colors hover:bg-slate-50 hover:text-slate-950"
          >
            {$t('common.clear_filters')}
          </button>
        {/if}
        <button
          type="button"
          onclick={() => lotsQuery.refetch()}
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
    </div>
  </section>

  <section class="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
    {#if lotsQuery.isLoading}
      <div class="border-b border-slate-200 bg-slate-50 px-4 py-3">
        <div class="h-4 w-40 animate-pulse rounded bg-slate-200"></div>
      </div>
      <div class="divide-y divide-slate-100">
        {#each [1, 2, 3, 4, 5, 6] as _}
          <div class="grid grid-cols-[1.2fr_1fr_.85fr_1fr_.65fr_.65fr_.9fr_.8fr_.65fr] gap-4 px-4 py-4">
            {#each [1, 2, 3, 4, 5, 6, 7, 8, 9] as __}
              <div class="h-4 animate-pulse rounded bg-slate-100"></div>
            {/each}
          </div>
        {/each}
      </div>
    {:else if lotsQuery.isError}
      <div class="m-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
        {$t('common.error')}: {lotsQuery.error?.message}
      </div>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full min-w-[1080px] text-left text-sm">
          <thead class="border-b border-slate-200 bg-slate-50 text-xs font-semibold text-slate-500">
            <tr>
              <th class="px-4 py-3">Lot No.</th>
              <th class="px-4 py-3">Material</th>
              <th class="px-4 py-3">Type</th>
              <th class="px-4 py-3">Supplier</th>
              <th class="px-4 py-3">Quantity</th>
              <th class="px-4 py-3">AI Score</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3">Created Date</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            {#each lots as lot}
              <tr class="transition-colors hover:bg-slate-50" use:highlightOnChange={lot.id}>
                <td class="px-4 py-3 align-middle">
                  <a href="/lots/{lot.id}" class="font-mono text-xs font-semibold text-blue-600 hover:underline">
                    {lot.lotNumber}
                  </a>
                </td>
                <td class="px-4 py-3 align-middle">
                  <div class="font-medium text-slate-950">{lot.materialName}</div>
                  <div class="mt-0.5 text-xs text-slate-500">{lot.arrivalDate || '-'}</div>
                </td>
                <td class="px-4 py-3 align-middle">
                  <span class="inline-flex rounded-md bg-slate-100 px-2 py-1 text-xs font-medium text-slate-700">
                    {$t(`material_type.${lot.materialType}`)}
                  </span>
                </td>
                <td class="px-4 py-3 align-middle text-slate-700">{lot.supplierName}</td>
                <td class="whitespace-nowrap px-4 py-3 align-middle text-slate-700">
                  {formatQuantity(lot.quantity)} {lot.unit}
                </td>
                <td class="px-4 py-3 align-middle">
                  {#if lot.qcRecommendation}
                    <span class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-semibold {qcTone(lot.qcRecommendation)}">
                      {lot.qcRecommendation}
                      <span class="font-mono">{Math.round(lot.qcConfidence * 100)}%</span>
                    </span>
                  {:else}
                    <span class="text-xs text-slate-400">—</span>
                  {/if}
                </td>
                <td class="px-4 py-3 align-middle">
                  <span class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-semibold ring-1 ring-inset {statusClass(lot.status)}">
                    <span class="size-1.5 rounded-full {statusDot(lot.status)}"></span>
                    {$t(`lot_status.${lot.status}`)}
                  </span>
                </td>
                <td class="whitespace-nowrap px-4 py-3 align-middle text-xs text-slate-500">
                  {formatDate(lot.createdAt?.seconds)}
                </td>
                <td class="px-4 py-3 text-right align-middle">
                  <a href="/lots/{lot.id}" class="inline-flex h-8 items-center rounded-md border border-slate-200 bg-white px-3 text-xs font-semibold text-blue-600 shadow-sm transition-colors hover:bg-blue-50">
                    View
                  </a>
                </td>
              </tr>
            {:else}
              <tr>
                <td colspan="9" class="px-4 py-16">
                  <div class="mx-auto flex max-w-sm flex-col items-center text-center">
                    <div class="flex size-12 items-center justify-center rounded-lg bg-slate-100 text-slate-500">
                      <svg viewBox="0 0 24 24" class="size-6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                        <path d="m12 3 8 4.5v9L12 21l-8-4.5v-9L12 3Z" />
                        <path d="m4 7.5 8 4.5 8-4.5" />
                        <path d="M12 12v9" />
                      </svg>
                    </div>
                    <h2 class="mt-3 text-sm font-semibold text-slate-950">{$t('lot.no_lots')}</h2>
                    <p class="mt-1 text-sm text-slate-500">Try clearing filters or create a new incoming lot.</p>
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
          Showing {lots.length} {lots.length === 1 ? $t('common.result') : $t('common.results')}
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
