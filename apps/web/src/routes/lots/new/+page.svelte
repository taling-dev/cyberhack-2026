<script lang="ts">
  import { goto } from '$app/navigation';
  import { t } from 'svelte-i18n';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';

  const client = createClient(LotService, transport);

  let supplierName = $state('');
  let materialName = $state('');
  let materialType = $state(1); // RAW_BOTANICAL
  let quantity = $state<number | undefined>(undefined);
  let unit = $state('kg');
  let arrivalDate = $state(new Date().toISOString().slice(0, 10));
  let temperatureRange = $state(1); // AMBIENT
  let hazardClass = $state(1); // NONE
  let submitting = $state(false);
  let error = $state('');

  // Auto-adjust defaults when material type changes
  $effect(() => {
    if (materialType === 1) {
      temperatureRange = 1;
      hazardClass = 1;
    } else if (materialType === 2 || materialType === 3) {
      temperatureRange = 2;
    }
  });

  function validate(): string | null {
    if (!supplierName.trim()) return 'Supplier name is required';
    if (!materialName.trim()) return 'Material name is required';
    if (!quantity || quantity <= 0) return 'Quantity must be greater than 0';
    if (!arrivalDate) return 'Arrival date is required';
    return null;
  }

  async function handleSubmit(e: Event) {
    e.preventDefault();
    const v = validate();
    if (v) { error = v; return; }
    submitting = true;
    error = '';
    try {
      const res = await client.createLot({
        supplierName: supplierName.trim(),
        materialName: materialName.trim(),
        materialType,
        quantity: quantity!,
        unit,
        arrivalDate,
        storageRequirement: { temperatureRange, hazardClass },
        idempotencyKey: crypto.randomUUID()
      });
      goto(`/lots/${res.lot?.id}`);
    } catch (e: any) {
      error = e.message || 'Failed to create lot';
    } finally {
      submitting = false;
    }
  }
</script>

<div class="max-w-2xl space-y-6">
  <div>
    <a href="/lots" class="text-sm text-gray-500 hover:text-blue-600">{$t('lot.back_to_lots')}</a>
    <h1 class="text-2xl font-bold mt-1">{$t('lot.create_title')}</h1>
  </div>

  {#if error}
    <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm" role="alert">{error}</div>
  {/if}

  <form onsubmit={handleSubmit} class="space-y-4 bg-white border rounded-lg p-6" autocomplete="on">
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div>
        <label for="supplier" class="block text-sm font-medium mb-1">{$t('lot.supplier_name')} <span class="text-red-500">*</span></label>
        <input id="supplier" name="supplier" bind:value={supplierName} required autocomplete="organization" class="w-full border rounded-md px-3 py-2 text-sm" />
      </div>
      <div>
        <label for="material" class="block text-sm font-medium mb-1">{$t('lot.material_name')} <span class="text-red-500">*</span></label>
        <input id="material" name="material" bind:value={materialName} required class="w-full border rounded-md px-3 py-2 text-sm" />
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
      <div>
        <label for="mtype" class="block text-sm font-medium mb-1">{$t('lot.material_type')}</label>
        <select id="mtype" bind:value={materialType} class="w-full border rounded-md px-3 py-2 text-sm">
          {#each [1,2,3,4] as m}
            <option value={m}>{$t(`material_type.${m}`)}</option>
          {/each}
        </select>
      </div>
      <div>
        <label for="qty" class="block text-sm font-medium mb-1">{$t('lot.quantity')} <span class="text-red-500">*</span></label>
        <input id="qty" type="number" step="0.001" min="0.001" bind:value={quantity} placeholder="0.000" required class="w-full border rounded-md px-3 py-2 text-sm" />
      </div>
      <div>
        <label for="unit" class="block text-sm font-medium mb-1">{$t('lot.unit')}</label>
        <select id="unit" bind:value={unit} class="w-full border rounded-md px-3 py-2 text-sm">
          <option value="kg">kg</option>
          <option value="L">L</option>
          <option value="pcs">pcs</option>
        </select>
      </div>
    </div>

    <div>
      <label for="arrival" class="block text-sm font-medium mb-1">{$t('lot.arrival_date')} <span class="text-red-500">*</span></label>
      <input id="arrival" bind:value={arrivalDate} type="date" required class="w-full border rounded-md px-3 py-2 text-sm" />
    </div>

    <fieldset class="border rounded-md p-4 space-y-3">
      <legend class="text-sm font-medium px-2">{$t('lot.storage_requirement')}</legend>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label for="temp" class="block text-sm mb-1">{$t('lot.temperature_range')}</label>
          <select id="temp" bind:value={temperatureRange} class="w-full border rounded-md px-3 py-2 text-sm">
            {#each [1,2,3] as r}
              <option value={r}>{$t(`temp_range.${r}`)}</option>
            {/each}
          </select>
        </div>
        <div>
          <label for="hz" class="block text-sm mb-1">{$t('lot.hazard_class')}</label>
          <select id="hz" bind:value={hazardClass} class="w-full border rounded-md px-3 py-2 text-sm">
            {#each [1,2,3] as h}
              <option value={h}>{$t(`hazard.${h}`)}</option>
            {/each}
          </select>
        </div>
      </div>
    </fieldset>

    <div class="flex gap-3">
      <button type="submit" disabled={submitting} class="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 text-sm disabled:opacity-50">
        {submitting ? $t('lot.submit_creating') : $t('lot.submit_create')}
      </button>
      <a href="/lots" class="px-4 py-2 border rounded-md text-sm hover:bg-gray-50">{$t('common.cancel')}</a>
    </div>
  </form>
</div>
