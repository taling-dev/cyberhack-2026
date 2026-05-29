<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { highlightOnChange } from '$lib/actions/highlightOnChange.svelte';

  const client = createClient(LotService, transport);

  // Status 4 = QC_REVIEW
  const lotsQuery = createQuery(() => ({
    queryKey: ['qc-review-lots'],
    queryFn: () => client.listLots({ pageSize: 50, statusFilter: 4 }),
    refetchInterval: 15_000,
  }));

  const lots = $derived(lotsQuery.data?.lots ?? []);
</script>

<div class="space-y-4">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold">{$t('qc.queue_title')}</h1>
    <span class="text-sm text-gray-500">{$t('qc.queue_subtitle')} · {lots.length}</span>
  </div>

  {#if lotsQuery.isLoading}
    <div class="flex items-center justify-center py-12 text-gray-500">
      <div class="inline-block w-5 h-5 border-2 border-blue-500 border-t-transparent rounded-full animate-spin mr-3" aria-hidden="true"></div>
      {$t('common.loading')}
    </div>
  {:else if lotsQuery.isError}
    <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm">{lotsQuery.error?.message || $t('common.error')}</div>
  {:else}
    <div class="overflow-x-auto border rounded-lg bg-white">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 border-b">
          <tr>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.lot_number')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.material')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.type')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.supplier')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('common.actions')}</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each lots as lot}
            <tr class="hover:bg-gray-50 transition-colors" use:highlightOnChange={lot.id}>
              <td class="px-4 py-3 font-mono text-xs">{lot.lotNumber}</td>
              <td class="px-4 py-3">{lot.materialName}</td>
              <td class="px-4 py-3">
                <span class="px-2 py-0.5 rounded text-xs bg-gray-100">{$t(`material_type.${lot.materialType}`)}</span>
              </td>
              <td class="px-4 py-3">{lot.supplierName}</td>
              <td class="px-4 py-3">
                <a href="/qc/{lot.id}" class="px-3 py-1 bg-orange-100 text-orange-700 rounded text-xs font-medium hover:bg-orange-200">
                  {$t('qc.review_button')}
                </a>
              </td>
            </tr>
          {:else}
            <tr><td colspan="5" class="px-4 py-12 text-center text-gray-400">{$t('qc.no_queue')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
