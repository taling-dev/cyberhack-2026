<script lang="ts">
  import { goto } from '$app/navigation';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';

  const client = createClient(LotService, transport);

  let supplierName = $state('');
  let materialName = $state('');
  let materialType = $state(1); // RAW_BOTANICAL
  let quantity = $state(0);
  let unit = $state('kg');
  let arrivalDate = $state(new Date().toISOString().slice(0, 10));
  let temperatureRange = $state(1); // AMBIENT
  let hazardClass = $state(1); // NONE
  let submitting = $state(false);
  let error = $state('');

  // Auto-adjust defaults when material type changes
  $effect(() => {
    if (materialType === 1) { // RAW_BOTANICAL
      temperatureRange = 1; // AMBIENT
      hazardClass = 1; // NONE
    } else if (materialType === 2 || materialType === 3) { // EXTRACT or POWDER
      temperatureRange = 2; // COLD
    }
  });

  async function handleSubmit(e: Event) {
    e.preventDefault();
    submitting = true;
    error = '';
    try {
      const res = await client.createLot({
        supplierName,
        materialName,
        materialType,
        quantity,
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
  <h1 class="text-2xl font-bold">Create New Lot</h1>

  {#if error}
    <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm">{error}</div>
  {/if}

  <form onsubmit={handleSubmit} class="space-y-4">
    <div class="grid grid-cols-2 gap-4">
      <div>
        <label class="block text-sm font-medium mb-1">Supplier Name</label>
        <input bind:value={supplierName} required class="w-full border rounded-md px-3 py-2 text-sm" />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1">Material Name</label>
        <input bind:value={materialName} required class="w-full border rounded-md px-3 py-2 text-sm" />
      </div>
    </div>

    <div class="grid grid-cols-3 gap-4">
      <div>
        <label class="block text-sm font-medium mb-1">Material Type</label>
        <select bind:value={materialType} class="w-full border rounded-md px-3 py-2 text-sm">
          <option value={1}>Raw Botanical</option>
          <option value={2}>Extract</option>
          <option value={3}>Powder</option>
          <option value={4}>Other</option>
        </select>
      </div>
      <div>
        <label class="block text-sm font-medium mb-1">Quantity</label>
        <input bind:value={quantity} type="number" step="0.001" min="0" required class="w-full border rounded-md px-3 py-2 text-sm" />
      </div>
      <div>
        <label class="block text-sm font-medium mb-1">Unit</label>
        <select bind:value={unit} class="w-full border rounded-md px-3 py-2 text-sm">
          <option value="kg">kg</option>
          <option value="L">L</option>
          <option value="pcs">pcs</option>
        </select>
      </div>
    </div>

    <div>
      <label class="block text-sm font-medium mb-1">Arrival Date</label>
      <input bind:value={arrivalDate} type="date" required class="w-full border rounded-md px-3 py-2 text-sm" />
    </div>

    <fieldset class="border rounded-md p-4 space-y-3">
      <legend class="text-sm font-medium px-2">Storage Requirement</legend>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="block text-sm mb-1">Temperature Range</label>
          <select bind:value={temperatureRange} class="w-full border rounded-md px-3 py-2 text-sm">
            <option value={1}>Ambient (15–25 °C)</option>
            <option value={2}>Cold (2–8 °C)</option>
            <option value={3}>Deep Freeze (−20 to −4 °C)</option>
          </select>
        </div>
        <div>
          <label class="block text-sm mb-1">Drum / Hazard Class</label>
          <select bind:value={hazardClass} class="w-full border rounded-md px-3 py-2 text-sm">
            <option value={1}>None</option>
            <option value={2}>IBC</option>
            <option value={3}>IPPC</option>
          </select>
        </div>
      </div>
    </fieldset>

    <div class="flex gap-3">
      <button type="submit" disabled={submitting} class="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 text-sm disabled:opacity-50">
        {submitting ? 'Creating...' : 'Create Lot'}
      </button>
      <a href="/lots" class="px-4 py-2 border rounded-md text-sm hover:bg-gray-50">Cancel</a>
    </div>
  </form>
</div>
