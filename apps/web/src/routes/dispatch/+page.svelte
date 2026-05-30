<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery, getQueryClientContext } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { DispatchService, DispatchStatus } from '$lib/gen/simaops/dispatch/v1/dispatch_pb';
  import { highlightOnChange } from '$lib/actions/highlightOnChange.svelte';
  import { focusTrap } from '$lib/actions/focusTrap.svelte';

  const dispatchClient = createClient(DispatchService, transport);
  const lotClient = createClient(LotService, transport);
  const queryClient = getQueryClientContext();

  const dispatchesQuery = createQuery(() => ({
    queryKey: ['dispatches'],
    queryFn: () => dispatchClient.listDispatches({ pageSize: 50 }),
    refetchInterval: 15_000,
  }));

  // READY_FOR_PRODUCTION lots (status 8) eligible for a new dispatch.
  const readyLotsQuery = createQuery(() => ({
    queryKey: ['dispatch-ready-lots'],
    queryFn: () => lotClient.listLots({ pageSize: 50, statusFilter: 8 }),
  }));

  // Status enum (numeric) → tailwind chip classes.
  const statusColors: Record<number, string> = {
    1: 'bg-gray-100 text-gray-700',       // PENDING
    2: 'bg-blue-100 text-blue-700',       // SCHEDULED
    3: 'bg-amber-100 text-amber-700',     // IN_TRANSIT
    4: 'bg-emerald-100 text-emerald-700', // DELIVERED
    5: 'bg-red-100 text-red-700',         // CANCELLED
  };

  // FSM next-step mapping (mirrors the Go dispatchTransitions).
  function nextStatus(s: number): number | null {
    if (s === DispatchStatus.PENDING) return DispatchStatus.SCHEDULED;
    if (s === DispatchStatus.SCHEDULED) return DispatchStatus.IN_TRANSIT;
    if (s === DispatchStatus.IN_TRANSIT) return DispatchStatus.DELIVERED;
    return null;
  }
  const isTerminal = (s: number) =>
    s === DispatchStatus.DELIVERED || s === DispatchStatus.CANCELLED;

  // ─── Create modal ────────────────────────────────────────────────
  let showModal = $state(false);
  let form = $state({ lotId: '', destination: '', carrier: '', quantity: 0, unit: 'kg', scheduledAt: '', notes: '' });
  let creating = $state(false);
  let createError = $state('');
  let banner = $state('');
  let rowBusy = $state<string | null>(null);

  function openCreate() {
    form = { lotId: '', destination: '', carrier: '', quantity: 0, unit: 'kg', scheduledAt: '', notes: '' };
    createError = '';
    showModal = true;
  }

  // When a lot is picked, default the dispatch quantity/unit to the lot's.
  function onLotChange() {
    const lot = (readyLotsQuery.data?.lots ?? []).find((l) => l.id === form.lotId);
    if (lot) {
      form.quantity = lot.quantity;
      form.unit = lot.unit;
    }
  }

  async function submitCreate() {
    if (!form.lotId) { createError = $t('dispatch.validation.lot_required'); return; }
    if (!form.destination.trim()) { createError = $t('dispatch.validation.destination_required'); return; }
    if (form.quantity <= 0) { createError = $t('dispatch.validation.quantity_positive'); return; }
    creating = true;
    createError = '';
    try {
      await dispatchClient.createDispatch({
        lotId: form.lotId,
        destination: form.destination.trim(),
        carrier: form.carrier.trim(),
        quantity: form.quantity,
        unit: form.unit,
        scheduledAt: form.scheduledAt ? new Date(form.scheduledAt).toISOString() : '',
        notes: form.notes.trim(),
        idempotencyKey: crypto.randomUUID(),
      });
      showModal = false;
      banner = $t('dispatch.created_msg');
      queryClient.invalidateQueries({ queryKey: ['dispatches'] });
      queryClient.invalidateQueries({ queryKey: ['dispatch-ready-lots'] });
    } catch (e: any) {
      createError = e.message || $t('common.error');
    } finally {
      creating = false;
    }
  }

  async function advance(id: string, to: number) {
    rowBusy = id;
    try {
      await dispatchClient.updateDispatchStatus({ dispatchId: id, newStatus: to, idempotencyKey: crypto.randomUUID() });
      queryClient.invalidateQueries({ queryKey: ['dispatches'] });
    } catch (e: any) {
      banner = e.message || $t('common.error');
    } finally {
      rowBusy = null;
    }
  }

  async function cancel(id: string) {
    rowBusy = id;
    try {
      await dispatchClient.updateDispatchStatus({ dispatchId: id, newStatus: DispatchStatus.CANCELLED, idempotencyKey: crypto.randomUUID() });
      queryClient.invalidateQueries({ queryKey: ['dispatches'] });
    } catch (e: any) {
      banner = e.message || $t('common.error');
    } finally {
      rowBusy = null;
    }
  }

  const dispatches = $derived(dispatchesQuery.data?.dispatches ?? []);
  const readyLots = $derived(readyLotsQuery.data?.lots ?? []);
</script>

<svelte:window onkeydown={(e) => { if (showModal && e.key === 'Escape') showModal = false; }} />

<div class="space-y-4">
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-bold">{$t('nav.dispatch')}</h1>
      <p class="text-sm text-gray-500">{$t('dispatch.subtitle')}</p>
    </div>
    <button onclick={openCreate} class="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 text-sm">
      {$t('dispatch.create_button')}
    </button>
  </div>

  {#if banner}
    <div class="p-3 bg-green-50 border border-green-200 rounded text-green-700 text-sm" role="status">✓ {banner}</div>
  {/if}

  {#if dispatchesQuery.isLoading}
    <p class="text-gray-500">{$t('common.loading')}</p>
  {:else if dispatchesQuery.isError}
    <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm">{dispatchesQuery.error?.message || $t('common.error')}</div>
  {:else}
    <div class="overflow-x-auto border rounded-lg bg-white">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 border-b">
          <tr>
            <th class="px-4 py-3 text-left font-medium">{$t('dispatch.dispatch_number')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.lot_number')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('dispatch.destination')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('dispatch.carrier')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('lot.quantity')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('common.status')}</th>
            <th class="px-4 py-3 text-left font-medium">{$t('common.actions')}</th>
          </tr>
        </thead>
        <tbody class="divide-y">
          {#each dispatches as d}
            <tr class="hover:bg-gray-50 transition-colors" use:highlightOnChange={d.id}>
              <td class="px-4 py-3 font-mono text-xs">{d.dispatchNumber}</td>
              <td class="px-4 py-3 font-mono text-xs">
                {#if d.lotId}<a href="/lots/{d.lotId}" class="text-blue-600 hover:underline">{d.lotNumber || d.lotId}</a>{/if}
              </td>
              <td class="px-4 py-3">{d.destination}</td>
              <td class="px-4 py-3 text-gray-600">{d.carrier || '—'}</td>
              <td class="px-4 py-3 whitespace-nowrap">{d.quantity} {d.unit}</td>
              <td class="px-4 py-3">
                <span class="px-2 py-0.5 rounded text-xs whitespace-nowrap {statusColors[d.status] ?? ''}">{$t(`dispatch_status.${d.status}`)}</span>
              </td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  {#if nextStatus(d.status) !== null}
                    {@const to = nextStatus(d.status)}
                    <button
                      onclick={() => advance(d.id, to!)}
                      disabled={rowBusy === d.id}
                      class="px-3 py-1 bg-blue-100 text-blue-700 rounded text-xs font-medium hover:bg-blue-200 disabled:opacity-50"
                    >{$t('dispatch.advance_to')} {$t(`dispatch_status.${to}`)}</button>
                  {/if}
                  {#if !isTerminal(d.status)}
                    <button
                      onclick={() => cancel(d.id)}
                      disabled={rowBusy === d.id}
                      class="px-3 py-1 bg-red-50 text-red-600 rounded text-xs font-medium hover:bg-red-100 disabled:opacity-50"
                    >{$t('dispatch.cancel')}</button>
                  {/if}
                </div>
              </td>
            </tr>
          {:else}
            <tr><td colspan="7" class="px-4 py-12 text-center text-gray-400">{$t('dispatch.no_dispatches')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<!-- Create modal -->
{#if showModal}
  <div
    class="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4"
    role="dialog" aria-modal="true" aria-labelledby="dispatch-title" tabindex="-1"
    use:focusTrap
    onclick={(e) => { if (e.target === e.currentTarget) showModal = false; }}
    onkeydown={(e) => { if (e.key === 'Escape') showModal = false; }}
  >
    <div class="bg-white rounded-lg p-6 w-full max-w-lg space-y-4 shadow-xl">
      <h2 id="dispatch-title" class="text-lg font-bold">{$t('dispatch.create_title')}</h2>

      <div>
        <label for="d-lot" class="block text-sm font-medium mb-1">{$t('dispatch.ready_lot')} <span class="text-red-500">*</span></label>
        <select id="d-lot" bind:value={form.lotId} onchange={onLotChange} class="w-full border rounded-md px-3 py-2 text-sm">
          <option value="">{$t('dispatch.select_lot')}</option>
          {#each readyLots as lot}
            <option value={lot.id}>{lot.lotNumber} — {lot.materialName}</option>
          {/each}
        </select>
        {#if readyLots.length === 0}
          <p class="text-xs text-gray-400 mt-1">{$t('dispatch.no_ready_lots')}</p>
        {/if}
      </div>

      <div>
        <label for="d-dest" class="block text-sm font-medium mb-1">{$t('dispatch.destination')} <span class="text-red-500">*</span></label>
        <input id="d-dest" bind:value={form.destination} class="w-full border rounded-md px-3 py-2 text-sm" />
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="d-carrier" class="block text-sm font-medium mb-1">{$t('dispatch.carrier')}</label>
          <input id="d-carrier" bind:value={form.carrier} class="w-full border rounded-md px-3 py-2 text-sm" />
        </div>
        <div>
          <label for="d-sched" class="block text-sm font-medium mb-1">{$t('dispatch.scheduled_at')}</label>
          <input id="d-sched" type="datetime-local" bind:value={form.scheduledAt} class="w-full border rounded-md px-3 py-2 text-sm" />
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="d-qty" class="block text-sm font-medium mb-1">{$t('lot.quantity')} <span class="text-red-500">*</span></label>
          <input id="d-qty" type="number" min="0" step="0.001" bind:value={form.quantity} class="w-full border rounded-md px-3 py-2 text-sm" />
        </div>
        <div>
          <label for="d-unit" class="block text-sm font-medium mb-1">{$t('lot.unit')}</label>
          <input id="d-unit" bind:value={form.unit} class="w-full border rounded-md px-3 py-2 text-sm" />
        </div>
      </div>

      <div>
        <label for="d-notes" class="block text-sm font-medium mb-1">{$t('dispatch.notes')}</label>
        <textarea id="d-notes" bind:value={form.notes} rows="2" class="w-full border rounded-md px-3 py-2 text-sm"></textarea>
      </div>

      {#if createError}
        <p class="text-sm text-red-600" role="alert">{createError}</p>
      {/if}

      <div class="flex justify-end gap-2">
        <button onclick={() => showModal = false} class="px-4 py-2 border rounded-md text-sm">{$t('common.cancel')}</button>
        <button onclick={submitCreate} disabled={creating} class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm hover:bg-blue-700 disabled:opacity-50">
          {creating ? $t('dispatch.creating') : $t('dispatch.create_submit')}
        </button>
      </div>
    </div>
  </div>
{/if}
