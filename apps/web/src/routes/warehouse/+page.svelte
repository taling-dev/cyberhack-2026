<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { createConnectTransport } from '@connectrpc/connect-web';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_connect';
  import { WarehouseService } from '$lib/gen/simaops/warehouse/v1/warehouse_connect';

  const transport = createConnectTransport({ baseUrl: 'http://localhost:8080', useBinaryFormat: false });
  const lotClient = createClient(LotService, transport);
  const whClient = createClient(WarehouseService, transport);
  const queryClient = useQueryClient();

  // Lots awaiting warehouse assignment (status 5 = QC_APPROVED)
  const lotsQuery = createQuery({
    queryKey: ['warehouse-queue'],
    queryFn: () => lotClient.listLots({ pageSize: 50, statusFilter: 5 })
  });

  // Assignment modal state
  let showModal = $state(false);
  let selectedLotId = $state('');
  let selectedLotNumber = $state('');
  let recommendations = $state<any[]>([]);
  let loadingRecs = $state(false);
  let assigning = $state(false);
  let assignError = $state('');
  let assignSuccess = $state('');

  async function openAssign(lotId: string, lotNumber: string) {
    selectedLotId = lotId;
    selectedLotNumber = lotNumber;
    showModal = true;
    loadingRecs = true;
    assignError = '';
    try {
      const res = await whClient.recommendSlot({ lotId });
      recommendations = res.recommendations ?? [];
    } catch (e: any) {
      assignError = e.message || 'Failed to load recommendations';
    } finally {
      loadingRecs = false;
    }
  }

  async function doAssign(locationId: string) {
    assigning = true;
    assignError = '';
    try {
      const res = await whClient.assignSlot({
        lotId: selectedLotId,
        locationId,
        idempotencyKey: crypto.randomUUID()
      });
      showModal = false;
      assignSuccess = `${selectedLotNumber} → ${res.assignment?.locationCode}`;
      queryClient.invalidateQueries({ queryKey: ['warehouse-queue'] });
    } catch (e: any) {
      assignError = e.message || 'Assignment failed';
    } finally {
      assigning = false;
    }
  }
</script>

<div class="space-y-4">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold">{$t('nav.warehouse')}</h1>
    <span class="text-sm text-gray-500">QC-approved lots awaiting slot assignment</span>
  </div>

  {#if assignSuccess}
    <div class="p-3 bg-green-50 border border-green-200 rounded text-green-700 text-sm">
      ✓ Assigned: {assignSuccess} — lot is now READY_FOR_PRODUCTION
    </div>
  {/if}

  {#if $lotsQuery.isLoading}
    <p class="text-gray-500">{$t('common.loading')}</p>
  {:else}
    <div class="overflow-x-auto border rounded-lg">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 border-b">
          <tr>
            <th class="px-4 py-3 text-left font-medium">Lot #</th>
            <th class="px-4 py-3 text-left font-medium">Material</th>
            <th class="px-4 py-3 text-left font-medium">Storage Req</th>
            <th class="px-4 py-3 text-left font-medium">{$t('common.actions')}</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each $lotsQuery.data?.lots ?? [] as lot}
            <tr class="hover:bg-gray-50">
              <td class="px-4 py-3 font-mono text-xs">{lot.lotNumber}</td>
              <td class="px-4 py-3">{lot.materialName}</td>
              <td class="px-4 py-3 text-xs text-gray-600">
                {lot.storageRequirement?.temperatureRange === 1 ? '🌡️ Ambient' :
                 lot.storageRequirement?.temperatureRange === 2 ? '❄️ Cold' :
                 lot.storageRequirement?.temperatureRange === 3 ? '🧊 Deep Freeze' : '—'}
                {lot.storageRequirement?.hazardClass === 2 ? ' + IBC' :
                 lot.storageRequirement?.hazardClass === 3 ? ' + IPPC' : ''}
              </td>
              <td class="px-4 py-3">
                <button
                  onclick={() => openAssign(lot.id, lot.lotNumber)}
                  class="px-3 py-1 bg-purple-100 text-purple-700 rounded text-xs font-medium hover:bg-purple-200"
                >
                  Assign Slot →
                </button>
              </td>
            </tr>
          {:else}
            <tr><td colspan="4" class="px-4 py-8 text-center text-gray-400">No lots awaiting assignment</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<!-- Assignment Modal -->
{#if showModal}
  <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
    <div class="bg-white rounded-lg p-6 w-full max-w-lg space-y-4">
      <h2 class="text-lg font-bold">Assign Slot: {selectedLotNumber}</h2>

      {#if loadingRecs}
        <p class="text-gray-500 text-sm">Loading recommendations...</p>
      {:else if recommendations.length === 0}
        <p class="text-red-600 text-sm">No compatible slots found for this lot's storage requirements.</p>
      {:else}
        <div class="space-y-2">
          <p class="text-sm text-gray-500">Recommended slots (sorted by capacity):</p>
          {#each recommendations as rec}
            <div class="border rounded-md p-3 flex items-center justify-between hover:bg-gray-50">
              <div>
                <span class="font-mono font-bold text-sm">{rec.location?.code}</span>
                <span class="text-xs text-gray-500 ml-2">{rec.location?.zone}</span>
                <p class="text-xs text-gray-400 mt-0.5">{rec.reason}</p>
              </div>
              <button
                onclick={() => doAssign(rec.location?.id)}
                disabled={assigning}
                class="px-3 py-1 bg-purple-600 text-white rounded text-xs hover:bg-purple-700 disabled:opacity-50"
              >
                {assigning ? '...' : 'Assign'}
              </button>
            </div>
          {/each}
        </div>
      {/if}

      {#if assignError}
        <p class="text-sm text-red-600">{assignError}</p>
      {/if}

      <div class="flex justify-end">
        <button onclick={() => showModal = false} class="px-4 py-2 border rounded-md text-sm">Cancel</button>
      </div>
    </div>
  </div>
{/if}
