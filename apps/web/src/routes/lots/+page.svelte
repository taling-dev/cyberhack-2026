<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { highlightOnChange } from '$lib/actions/highlightOnChange.svelte';

  const client = createClient(LotService, transport);

  let statusFilter = $state(0);
  let materialFilter = $state(0);
  let pageToken = $state('');
  let pageHistory = $state<string[]>(['']); // for back navigation

  const lotsQuery = createQuery(() => ({
    queryKey: ['lots', statusFilter, materialFilter, pageToken],
    queryFn: () => client.listLots({
      pageSize: 50,
      pageToken,
      statusFilter,
      materialTypeFilter: materialFilter,
    }),
  }));

  const statusColors: Record<number, string> = {
    1: 'bg-gray-100 text-gray-700', 2: 'bg-yellow-100 text-yellow-700',
    3: 'bg-blue-100 text-blue-700', 4: 'bg-orange-100 text-orange-700',
    5: 'bg-green-100 text-green-700', 6: 'bg-red-100 text-red-700',
    7: 'bg-purple-100 text-purple-700', 8: 'bg-emerald-100 text-emerald-700',
    9: 'bg-red-200 text-red-800'
  };

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

  const lots = $derived(lotsQuery.data?.lots ?? []);
  const hasNext = $derived(!!lotsQuery.data?.nextPageToken);
  const hasPrev = $derived(pageHistory.length > 1);
</script>

<div class="space-y-4">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold">{$t('lot.list_title')}</h1>
    <a href="/lots/new" class="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 text-sm">
      {$t('lot.create_button')}
    </a>
  </div>

  <!-- Filters -->
  <div class="flex flex-wrap items-center gap-3">
    <div>
      <label for="status-filter" class="sr-only">{$t('common.status')}</label>
      <select
        id="status-filter"
        bind:value={statusFilter}
        onchange={() => { pageToken = ''; pageHistory = ['']; }}
        class="border rounded-md px-3 py-1.5 text-sm"
      >
        <option value={0}>{$t('lot.filter_all_statuses')}</option>
        {#each [1,2,3,4,5,6,7,8,9] as s}
          <option value={s}>{$t(`lot_status.${s}`)}</option>
        {/each}
      </select>
    </div>
    <div>
      <label for="material-filter" class="sr-only">{$t('lot.material')}</label>
      <select
        id="material-filter"
        bind:value={materialFilter}
        onchange={() => { pageToken = ''; pageHistory = ['']; }}
        class="border rounded-md px-3 py-1.5 text-sm"
      >
        <option value={0}>{$t('lot.filter_all_materials')}</option>
        {#each [1,2,3,4] as m}
          <option value={m}>{$t(`material_type.${m}`)}</option>
        {/each}
      </select>
    </div>
    {#if statusFilter !== 0 || materialFilter !== 0}
      <button
        onclick={() => { statusFilter = 0; materialFilter = 0; pageToken = ''; pageHistory = ['']; }}
        class="text-xs text-gray-500 hover:text-gray-900 underline"
      >
        Clear filters
      </button>
    {/if}
    <div class="ml-auto text-xs text-gray-500">
      {lots.length} {lots.length === 1 ? 'result' : 'results'} on this page
    </div>
  </div>

  <!-- Table -->
  {#if lotsQuery.isLoading}
    <div class="flex items-center justify-center py-12 text-gray-500">
      <div class="inline-block w-5 h-5 border-2 border-blue-500 border-t-transparent rounded-full animate-spin mr-3" aria-hidden="true"></div>
      {$t('common.loading')}
    </div>
  {:else if lotsQuery.isError}
    <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm">
      {$t('common.error')} — {lotsQuery.error?.message}
    </div>
  {:else}
    <div class="overflow-x-auto border rounded-lg bg-white">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 border-b">
          <tr>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.lot_number')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.material')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.type')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.supplier')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.quantity')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('common.status')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('common.created')}</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each lots as lot}
            <tr class="hover:bg-gray-50 transition-colors" use:highlightOnChange={lot.id}>
              <td class="px-4 py-3">
                <a href="/lots/{lot.id}" class="text-blue-600 hover:underline font-mono text-xs">{lot.lotNumber}</a>
              </td>
              <td class="px-4 py-3">{lot.materialName}</td>
              <td class="px-4 py-3">
                <span class="px-2 py-0.5 rounded text-xs bg-gray-100">{$t(`material_type.${lot.materialType}`)}</span>
              </td>
              <td class="px-4 py-3">{lot.supplierName}</td>
              <td class="px-4 py-3 whitespace-nowrap">{lot.quantity} {lot.unit}</td>
              <td class="px-4 py-3">
                <span class="px-2 py-0.5 rounded text-xs whitespace-nowrap {statusColors[lot.status] ?? ''}">{$t(`lot_status.${lot.status}`)}</span>
              </td>
              <td class="px-4 py-3 text-gray-500 text-xs whitespace-nowrap">{lot.createdAt ? new Date(Number(lot.createdAt.seconds) * 1000).toLocaleDateString() : ''}</td>
            </tr>
          {:else}
            <tr><td colspan="7" class="px-4 py-12 text-center text-gray-400">{$t('lot.no_lots')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>

    <!-- Pagination -->
    {#if hasPrev || hasNext}
      <div class="flex justify-center gap-2 pt-2">
        <button
          disabled={!hasPrev}
          onclick={prevPage}
          class="px-3 py-1.5 border rounded text-sm disabled:opacity-40 disabled:cursor-not-allowed hover:bg-gray-50"
        >← Prev</button>
        <button
          disabled={!hasNext}
          onclick={nextPage}
          class="px-3 py-1.5 border rounded text-sm disabled:opacity-40 disabled:cursor-not-allowed hover:bg-gray-50"
        >Next →</button>
      </div>
    {/if}
  {/if}
</div>
