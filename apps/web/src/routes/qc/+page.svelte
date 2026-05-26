<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';

  const client = createClient(LotService, transport);

  // Status 4 = QC_REVIEW
  const lotsQuery = createQuery({
    queryKey: ['qc-review-lots'],
    queryFn: () => client.listLots({ pageSize: 50, statusFilter: 4 })
  });

  const materialLabels: Record<number, string> = { 1: 'Raw Botanical', 2: 'Extract', 3: 'Powder', 4: 'Other' };
</script>

<div class="space-y-4">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold">{$t('nav.qc')}</h1>
    <span class="text-sm text-gray-500">Showing lots awaiting QC review</span>
  </div>

  {#if $lotsQuery.isLoading}
    <p class="text-gray-500">{$t('common.loading')}</p>
  {:else if $lotsQuery.isError}
    <p class="text-red-500">{$t('common.error')}</p>
  {:else}
    <div class="overflow-x-auto border rounded-lg">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 border-b">
          <tr>
            <th class="px-4 py-3 text-left font-medium">Lot #</th>
            <th class="px-4 py-3 text-left font-medium">Material</th>
            <th class="px-4 py-3 text-left font-medium">Type</th>
            <th class="px-4 py-3 text-left font-medium">Supplier</th>
            <th class="px-4 py-3 text-left font-medium">{$t('common.actions')}</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each $lotsQuery.data?.lots ?? [] as lot}
            <tr class="hover:bg-gray-50">
              <td class="px-4 py-3 font-mono text-xs">{lot.lotNumber}</td>
              <td class="px-4 py-3">{lot.materialName}</td>
              <td class="px-4 py-3">
                <span class="px-2 py-0.5 rounded text-xs bg-gray-100">{materialLabels[lot.materialType] ?? 'Unknown'}</span>
              </td>
              <td class="px-4 py-3">{lot.supplierName}</td>
              <td class="px-4 py-3">
                <a href="/qc/{lot.id}" class="px-3 py-1 bg-orange-100 text-orange-700 rounded text-xs font-medium hover:bg-orange-200">
                  Review →
                </a>
              </td>
            </tr>
          {:else}
            <tr><td colspan="5" class="px-4 py-8 text-center text-gray-400">No lots awaiting QC review</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
