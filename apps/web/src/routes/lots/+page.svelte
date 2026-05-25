<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { createConnectTransport } from '@connectrpc/connect-web';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';

  const transport = createConnectTransport({ baseUrl: 'http://localhost:8080', useBinaryFormat: false });
  const client = createClient(LotService, transport);

  let statusFilter = $state('');
  let materialFilter = $state('');

  const lotsQuery = createQuery({
    queryKey: ['lots', statusFilter, materialFilter],
    queryFn: () => client.listLots({
      pageSize: 50,
      statusFilter: statusFilter ? Number(statusFilter) : 0,
      materialTypeFilter: materialFilter ? Number(materialFilter) : 0
    })
  });

  const statusLabels: Record<number, string> = {
    1: 'Draft', 2: 'Pending QC', 3: 'AI Processing', 4: 'QC Review',
    5: 'QC Approved', 6: 'QC Rejected', 7: 'Warehouse Assigned', 8: 'Ready', 9: 'Blocked'
  };

  const materialLabels: Record<number, string> = {
    1: 'Raw Botanical', 2: 'Extract', 3: 'Powder', 4: 'Other'
  };

  const statusColors: Record<number, string> = {
    1: 'bg-gray-100 text-gray-700', 2: 'bg-yellow-100 text-yellow-700',
    3: 'bg-blue-100 text-blue-700', 4: 'bg-orange-100 text-orange-700',
    5: 'bg-green-100 text-green-700', 6: 'bg-red-100 text-red-700',
    7: 'bg-purple-100 text-purple-700', 8: 'bg-emerald-100 text-emerald-700',
    9: 'bg-red-200 text-red-800'
  };
</script>

<div class="space-y-4">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold">{$t('nav.lots')}</h1>
    <a href="/lots/new" class="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 text-sm">
      + Create Lot
    </a>
  </div>

  <!-- Filters -->
  <div class="flex gap-3">
    <select bind:value={statusFilter} class="border rounded-md px-3 py-1.5 text-sm">
      <option value="">All Statuses</option>
      {#each Object.entries(statusLabels) as [val, label]}
        <option value={val}>{label}</option>
      {/each}
    </select>
    <select bind:value={materialFilter} class="border rounded-md px-3 py-1.5 text-sm">
      <option value="">All Materials</option>
      {#each Object.entries(materialLabels) as [val, label]}
        <option value={val}>{label}</option>
      {/each}
    </select>
  </div>

  <!-- Table -->
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
            <th class="px-4 py-3 text-left font-medium">Qty</th>
            <th class="px-4 py-3 text-left font-medium">{$t('common.status')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('common.created')}</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each $lotsQuery.data?.lots ?? [] as lot}
            <tr class="hover:bg-gray-50">
              <td class="px-4 py-3">
                <a href="/lots/{lot.id}" class="text-blue-600 hover:underline font-mono text-xs">{lot.lotNumber}</a>
              </td>
              <td class="px-4 py-3">{lot.materialName}</td>
              <td class="px-4 py-3">
                <span class="px-2 py-0.5 rounded text-xs bg-gray-100">{materialLabels[lot.materialType] ?? 'Unknown'}</span>
              </td>
              <td class="px-4 py-3">{lot.supplierName}</td>
              <td class="px-4 py-3">{lot.quantity} {lot.unit}</td>
              <td class="px-4 py-3">
                <span class="px-2 py-0.5 rounded text-xs {statusColors[lot.status] ?? ''}">{statusLabels[lot.status] ?? 'Unknown'}</span>
              </td>
              <td class="px-4 py-3 text-gray-500 text-xs">{lot.createdAt?.toDate().toLocaleDateString()}</td>
            </tr>
          {:else}
            <tr><td colspan="7" class="px-4 py-8 text-center text-gray-400">No lots found</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
