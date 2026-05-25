<script lang="ts">
  import { page } from '$app/stores';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { createConnectTransport } from '@connectrpc/connect-web';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';

  const transport = createConnectTransport({ baseUrl: 'http://localhost:8080', useBinaryFormat: false });
  const client = createClient(LotService, transport);

  const lotId = $derived($page.params.id);

  const lotQuery = createQuery({
    queryKey: ['lot', lotId],
    queryFn: () => client.getLot({ lotId })
  });

  const statusLabels: Record<number, string> = {
    1: 'Draft', 2: 'Pending QC', 3: 'AI Processing', 4: 'QC Review',
    5: 'QC Approved', 6: 'QC Rejected', 7: 'Warehouse Assigned', 8: 'Ready for Production', 9: 'Blocked'
  };

  const materialLabels: Record<number, string> = {
    1: 'Raw Botanical', 2: 'Extract', 3: 'Powder', 4: 'Other'
  };

  const tempLabels: Record<number, string> = {
    1: 'Ambient (15–25 °C)', 2: 'Cold (2–8 °C)', 3: 'Deep Freeze (−20 to −4 °C)'
  };

  const hazardLabels: Record<number, string> = {
    1: 'None', 2: 'IBC', 3: 'IPPC'
  };
</script>

<div class="max-w-3xl space-y-6">
  {#if $lotQuery.isLoading}
    <p class="text-gray-500">Loading...</p>
  {:else if $lotQuery.isError}
    <p class="text-red-500">Failed to load lot</p>
  {:else if $lotQuery.data?.lot}
    {@const lot = $lotQuery.data.lot}
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold">{lot.lotNumber}</h1>
        <p class="text-gray-500 text-sm">{lot.materialName} — {lot.supplierName}</p>
      </div>
      <span class="px-3 py-1 rounded-full text-sm font-medium bg-blue-100 text-blue-700">
        {statusLabels[lot.status] ?? 'Unknown'}
      </span>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <div class="border rounded-lg p-4 space-y-3">
        <h2 class="font-semibold text-sm text-gray-500 uppercase">Lot Details</h2>
        <dl class="space-y-2 text-sm">
          <div class="flex justify-between"><dt class="text-gray-500">Material Type</dt><dd>{materialLabels[lot.materialType]}</dd></div>
          <div class="flex justify-between"><dt class="text-gray-500">Quantity</dt><dd>{lot.quantity} {lot.unit}</dd></div>
          <div class="flex justify-between"><dt class="text-gray-500">Arrival Date</dt><dd>{lot.arrivalDate}</dd></div>
          <div class="flex justify-between"><dt class="text-gray-500">Created By</dt><dd>{lot.createdBy}</dd></div>
          <div class="flex justify-between"><dt class="text-gray-500">Created</dt><dd>{lot.createdAt?.toDate().toLocaleString()}</dd></div>
        </dl>
      </div>

      <div class="border rounded-lg p-4 space-y-3">
        <h2 class="font-semibold text-sm text-gray-500 uppercase">Storage Requirement</h2>
        <dl class="space-y-2 text-sm">
          <div class="flex justify-between"><dt class="text-gray-500">Temperature</dt><dd>{tempLabels[lot.storageRequirement?.temperatureRange ?? 0] ?? '—'}</dd></div>
          <div class="flex justify-between"><dt class="text-gray-500">Hazard/Drum Class</dt><dd>{hazardLabels[lot.storageRequirement?.hazardClass ?? 0] ?? '—'}</dd></div>
        </dl>
      </div>
    </div>

    <!-- Timeline placeholder -->
    <div class="border rounded-lg p-4">
      <h2 class="font-semibold text-sm text-gray-500 uppercase mb-3">Timeline</h2>
      <p class="text-gray-400 text-sm">Audit trail will appear here (Task 13).</p>
    </div>

    <a href="/lots" class="inline-block text-sm text-blue-600 hover:underline">← Back to lots</a>
  {/if}
</div>
