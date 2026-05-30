<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery, getQueryClientContext } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { WarehouseService } from '$lib/gen/simaops/warehouse/v1/warehouse_pb';
  import { highlightOnChange } from '$lib/actions/highlightOnChange.svelte';
  import { focusTrap } from '$lib/actions/focusTrap.svelte';

  const lotClient = createClient(LotService, transport);
  const whClient = createClient(WarehouseService, transport);
  const queryClient = getQueryClientContext();

  // Status 5 = QC_APPROVED
  const lotsQuery = createQuery(() => ({
    queryKey: ['warehouse-queue'],
    queryFn: () => lotClient.listLots({ pageSize: 50, statusFilter: 5 }),
    refetchInterval: 15_000,
  }));

  let showModal = $state(false);
  let selectedLotId = $state('');
  let selectedLotNumber = $state('');
  let recommendations = $state<any[]>([]);
  let loadingRecs = $state(false);
  let assigning = $state(false);
  let assignError = $state('');
  let assignSuccess = $state('');

  function handleKeydown(e: KeyboardEvent) {
    if (showModal && e.key === 'Escape') showModal = false;
  }

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

  const lots = $derived(lotsQuery.data?.lots ?? []);
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="space-y-4">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-bold">{$t('nav.warehouse')}</h1>
    <span class="text-sm text-gray-500">{$t('warehouse.subtitle')} · {lots.length}</span>
  </div>

  {#if assignSuccess}
    <div class="p-3 bg-green-50 border border-green-200 rounded text-green-700 text-sm" role="status">
      ✓ {$t('warehouse.assigned_msg')}: {assignSuccess}
    </div>
  {/if}

  {#if lotsQuery.isLoading}
    <p class="text-gray-500">{$t('common.loading')}</p>
  {:else if lotsQuery.isError}
    <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm">{lotsQuery.error?.message || $t('common.error')}</div>
  {:else}
    <div class="overflow-x-auto border rounded-lg bg-white">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 border-b">
          <tr>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.lot_number')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.material')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('warehouse.storage_req')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('common.actions')}</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each lots as lot}
            <tr class="hover:bg-gray-50 transition-colors" use:highlightOnChange={lot.id}>
              <td class="px-4 py-3 font-mono text-xs">{lot.lotNumber}</td>
              <td class="px-4 py-3">{lot.materialName}</td>
              <td class="px-4 py-3 text-xs text-gray-600">
                {lot.storageRequirement?.temperatureRange === 1 ? '🌡️ ' :
                 lot.storageRequirement?.temperatureRange === 2 ? '❄️ ' :
                 lot.storageRequirement?.temperatureRange === 3 ? '🧊 ' : ''}{lot.storageRequirement?.temperatureRange ? $t(`temp_range.${lot.storageRequirement.temperatureRange}`).split(' (')[0] : '—'}
                {lot.storageRequirement?.hazardClass === 2 ? ' + IBC' :
                 lot.storageRequirement?.hazardClass === 3 ? ' + IPPC' : ''}
              </td>
              <td class="px-4 py-3">
                <button
                  onclick={() => openAssign(lot.id, lot.lotNumber)}
                  class="px-3 py-1 bg-purple-100 text-purple-700 rounded text-xs font-medium hover:bg-purple-200"
                >
                  {$t('warehouse.assign_button')}
                </button>
              </td>
            </tr>
          {:else}
            <tr><td colspan="4" class="px-4 py-12 text-center text-gray-400">{$t('warehouse.no_queue')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<!-- Assignment Modal -->
{#if showModal}
  <div
    class="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="assign-title"
    tabindex="-1"
    use:focusTrap
    onclick={(e) => { if (e.target === e.currentTarget) showModal = false; }}
    onkeydown={(e) => { if (e.key === 'Escape') showModal = false; }}
  >
    <div class="bg-white rounded-lg p-6 w-full max-w-lg space-y-4 shadow-xl">
      <h2 id="assign-title" class="text-lg font-bold">{$t('warehouse.assign_modal_title')}: {selectedLotNumber}</h2>

      {#if loadingRecs}
        <p class="text-gray-500 text-sm">{$t('warehouse.loading_recs')}</p>
      {:else if recommendations.length === 0}
        <p class="text-red-600 text-sm">{$t('warehouse.no_recs')}</p>
      {:else}
        <div class="space-y-2">
          <p class="text-sm text-gray-500">{$t('warehouse.recs_label')}</p>
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
                {assigning ? $t('warehouse.assigning') : 'Assign'}
              </button>
            </div>
          {/each}
        </div>
      {/if}

      {#if assignError}
        <p class="text-sm text-red-600" role="alert">{assignError}</p>
      {/if}

      <div class="flex justify-end">
        <button onclick={() => showModal = false} class="px-4 py-2 border rounded-md text-sm">{$t('common.cancel')}</button>
      </div>
    </div>
  </div>
{/if}
