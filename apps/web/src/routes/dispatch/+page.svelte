<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery, getQueryClientContext } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { DashboardService } from '$lib/gen/simaops/dashboard/v1/dashboard_pb';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { DispatchService, DispatchStatus } from '$lib/gen/simaops/dispatch/v1/dispatch_pb';
  import DashboardIcon from '$lib/components/dashboard/DashboardIcon.svelte';
  import KpiCard from '$lib/components/dashboard/KpiCard.svelte';
  import { highlightOnChange } from '$lib/actions/highlightOnChange.svelte';
  import { focusTrap } from '$lib/actions/focusTrap.svelte';

  const dashboardClient = createClient(DashboardService, transport);
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

  const opsQuery = createQuery(() => ({
    queryKey: ['dashboard-ops'],
    queryFn: () => dashboardClient.getOpsDashboard({}),
    refetchInterval: 30_000,
  }));

  const statusMeta: Record<number, { tone: string; dot: string }> = {
    1: { tone: 'bg-slate-100 text-slate-700 ring-slate-200', dot: 'bg-slate-400' },
    2: { tone: 'bg-blue-100 text-blue-700 ring-blue-200', dot: 'bg-blue-500' },
    3: { tone: 'bg-orange-100 text-orange-700 ring-orange-200', dot: 'bg-orange-500' },
    4: { tone: 'bg-emerald-100 text-emerald-700 ring-emerald-200', dot: 'bg-emerald-500' },
    5: { tone: 'bg-red-100 text-red-700 ring-red-200', dot: 'bg-red-500' },
  };

  function nextStatus(s: number): number | null {
    if (s === DispatchStatus.PENDING) return DispatchStatus.SCHEDULED;
    if (s === DispatchStatus.SCHEDULED) return DispatchStatus.IN_TRANSIT;
    if (s === DispatchStatus.IN_TRANSIT) return DispatchStatus.DELIVERED;
    return null;
  }
  const isTerminal = (s: number) =>
    s === DispatchStatus.DELIVERED || s === DispatchStatus.CANCELLED;

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

  function statusClass(status: number) {
    return statusMeta[status]?.tone ?? statusMeta[1].tone;
  }

  function statusDot(status: number) {
    return statusMeta[status]?.dot ?? statusMeta[1].dot;
  }

  function statusCount(status: string) {
    return opsQuery.data?.lotsByStatus?.find((item) => item.status === status)?.count ?? 0;
  }

  function formatNumber(value?: number | null) {
    return value == null ? '-' : value.toLocaleString('en-US');
  }

  function formatQuantity(value: number) {
    return Number.isInteger(value)
      ? value.toLocaleString('en-US')
      : value.toLocaleString('en-US', { maximumFractionDigits: 2 });
  }

  function formatDateTime(seconds?: bigint | number | string) {
    if (!seconds) return '-';
    return new Date(Number(seconds) * 1000).toLocaleString('en-US', {
      month: '2-digit',
      day: '2-digit',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  }

  function activeDispatchCount() {
    return dispatches.filter((dispatch) => !isTerminal(dispatch.status)).length;
  }

  const dispatches = $derived(dispatchesQuery.data?.dispatches ?? []);
  const readyLots = $derived(readyLotsQuery.data?.lots ?? []);
  const warehouseAssigned = $derived(statusCount('WAREHOUSE_ASSIGNED'));
  const blockedLots = $derived(statusCount('BLOCKED'));
</script>

<svelte:window onkeydown={(e) => { if (showModal && e.key === 'Escape') showModal = false; }} />

<div class="space-y-5">
  <header class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
    <div>
      <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">Production</p>
      <h1 class="mt-1 text-[28px] font-bold tracking-normal text-slate-950">Production</h1>
    </div>

    <div class="flex flex-wrap items-center gap-3">
      <span class="rounded-md border border-slate-200 bg-white px-3 py-2 text-xs font-medium text-slate-500 shadow-sm">
        {readyLots.length} ready lots
      </span>
      <button
        type="button"
        onclick={() => {
          dispatchesQuery.refetch();
          readyLotsQuery.refetch();
          opsQuery.refetch();
        }}
        class="inline-flex h-10 items-center gap-2 rounded-md border border-slate-200 bg-white px-3 text-sm font-medium text-slate-600 shadow-sm transition-colors hover:bg-slate-50 hover:text-slate-950"
      >
        <svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M21 12a9 9 0 0 1-15.5 6.2" />
          <path d="M3 12A9 9 0 0 1 18.5 5.8" />
          <path d="M18 2v4h4" />
          <path d="M6 22v-4H2" />
        </svg>
        {$t('common.refresh')}
      </button>
      <button onclick={openCreate} class="inline-flex h-10 items-center gap-2 rounded-md bg-blue-600 px-4 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-blue-700">
        <svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
          <path d="M12 5v14" />
          <path d="M5 12h14" />
        </svg>
        New Dispatch
      </button>
    </div>
  </header>

  {#if banner}
    <div class="rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm font-medium text-emerald-700" role="status">
      {banner}
    </div>
  {/if}

  {#if opsQuery.isError}
    <div class="rounded-lg border border-orange-200 bg-orange-50 px-4 py-3 text-sm text-orange-700">
      Production summary unavailable: {opsQuery.error?.message || $t('common.error')}
    </div>
  {/if}

  <section class="grid grid-cols-2 gap-3 lg:grid-cols-4">
    <KpiCard title="Ready for Production" value={formatNumber(opsQuery.data?.lotsReadyForProduction)} icon="check-circle" tone="green" loading={opsQuery.isLoading} />
    <KpiCard title="Waiting QC" value={formatNumber(opsQuery.data?.lotsAwaitingQc)} icon="clock" tone="orange" loading={opsQuery.isLoading} />
    <KpiCard title="Warehouse Assigned" value={formatNumber(warehouseAssigned)} icon="warehouse" tone="purple" loading={opsQuery.isLoading} />
    <KpiCard title="Blocked Lots" value={formatNumber(blockedLots)} icon="shield" tone="red" loading={opsQuery.isLoading} />
  </section>

  <div class="grid gap-4 xl:grid-cols-[1.05fr_.95fr]">
    <section class="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
      <div class="flex items-center justify-between border-b border-slate-200 px-4 py-3">
        <div>
          <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">Production Ready Queue</h2>
        </div>
        <span class="rounded-md bg-emerald-100 px-2.5 py-1 text-xs font-semibold text-emerald-700">{readyLots.length} ready</span>
      </div>

      {#if readyLotsQuery.isLoading}
        <div class="divide-y divide-slate-100">
          {#each [1, 2, 3, 4] as _}
            <div class="grid grid-cols-[1fr_1fr_.65fr_.65fr] gap-4 px-4 py-4">
              {#each [1, 2, 3, 4] as __}
                <div class="h-4 animate-pulse rounded bg-slate-100"></div>
              {/each}
            </div>
          {/each}
        </div>
      {:else if readyLotsQuery.isError}
        <div class="m-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {readyLotsQuery.error?.message || $t('common.error')}
        </div>
      {:else if readyLots.length === 0}
        <div class="flex min-h-56 flex-col items-center justify-center px-4 text-center">
          <div class="flex size-12 items-center justify-center rounded-lg bg-slate-100 text-slate-500">
            <DashboardIcon name="cube" class="size-6" />
          </div>
          <h3 class="mt-3 text-sm font-semibold text-slate-950">{$t('dispatch.no_ready_lots')}</h3>
          <p class="mt-1 max-w-sm text-sm text-slate-500">Approved and warehouse-assigned lots will appear here once they are ready for production handoff.</p>
        </div>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full min-w-[680px] text-left text-sm">
            <thead class="border-b border-slate-200 bg-slate-50 text-xs font-semibold text-slate-500">
              <tr>
                <th class="px-4 py-3">Lot No.</th>
                <th class="px-4 py-3">Material</th>
                <th class="px-4 py-3">Quantity</th>
                <th class="px-4 py-3">Readiness</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-100">
              {#each readyLots as lot}
                <tr class="transition-colors hover:bg-slate-50" use:highlightOnChange={lot.id}>
                  <td class="px-4 py-3 align-middle">
                    <a href="/lots/{lot.id}" class="font-mono text-xs font-semibold text-blue-600 hover:underline">{lot.lotNumber}</a>
                  </td>
                  <td class="px-4 py-3 align-middle">
                    <div class="font-medium text-slate-950">{lot.materialName}</div>
                    <div class="mt-0.5 text-xs text-slate-500">{$t(`material_type.${lot.materialType}`)}</div>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 align-middle text-slate-700">{formatQuantity(lot.quantity)} {lot.unit}</td>
                  <td class="px-4 py-3 align-middle">
                    <span class="inline-flex items-center gap-1.5 rounded-md bg-emerald-100 px-2.5 py-1 text-xs font-semibold text-emerald-700 ring-1 ring-inset ring-emerald-200">
                      <span class="size-1.5 rounded-full bg-emerald-500"></span>
                      {$t('lot_status.8')}
                    </span>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>

    <section class="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
      <div class="flex items-start justify-between gap-3">
        <div>
          <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">Production Bottlenecks</h2>
        </div>
        <DashboardIcon name="activity" class="size-5 text-slate-400" />
      </div>

      {#if opsQuery.isLoading}
        <div class="mt-4 space-y-3">
          {#each [1, 2, 3] as _}
            <div class="h-16 animate-pulse rounded-lg bg-slate-100"></div>
          {/each}
        </div>
      {:else}
        <div class="mt-4 space-y-3">
          <div class="rounded-lg border border-orange-200 bg-orange-50 p-3">
            <div class="flex items-center justify-between gap-3">
              <span class="text-sm font-semibold text-orange-800">Waiting QC</span>
              <span class="text-lg font-bold text-slate-950">{formatNumber(opsQuery.data?.lotsAwaitingQc)}</span>
            </div>
            <p class="mt-1 text-xs text-orange-700">Lots still need QC completion before production readiness.</p>
          </div>
          <div class="rounded-lg border border-purple-200 bg-purple-50 p-3">
            <div class="flex items-center justify-between gap-3">
              <span class="text-sm font-semibold text-purple-800">Warehouse Assigned</span>
              <span class="text-lg font-bold text-slate-950">{formatNumber(warehouseAssigned)}</span>
            </div>
            <p class="mt-1 text-xs text-purple-700">Lots are slotted and waiting for final readiness transition.</p>
          </div>
          <div class="rounded-lg border border-red-200 bg-red-50 p-3">
            <div class="flex items-center justify-between gap-3">
              <span class="text-sm font-semibold text-red-800">Blocked Lots</span>
              <span class="text-lg font-bold text-slate-950">{formatNumber(blockedLots)}</span>
            </div>
            <p class="mt-1 text-xs text-red-700">Blocked material needs review before production handoff.</p>
          </div>
        </div>
      {/if}
    </section>
  </div>

  <section class="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
    <div class="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3">
      <div>
        <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">Production Dispatches</h2>
      </div>
      <span class="rounded-md bg-blue-100 px-2.5 py-1 text-xs font-semibold text-blue-700">{activeDispatchCount()} active</span>
    </div>

    {#if dispatchesQuery.isLoading}
      <div class="divide-y divide-slate-100">
        {#each [1, 2, 3, 4, 5] as _}
          <div class="grid grid-cols-[1fr_.9fr_1fr_.8fr_.65fr_.9fr_.8fr_1.15fr] gap-4 px-4 py-4">
            {#each [1, 2, 3, 4, 5, 6, 7, 8] as __}
              <div class="h-4 animate-pulse rounded bg-slate-100"></div>
            {/each}
          </div>
        {/each}
      </div>
    {:else if dispatchesQuery.isError}
      <div class="m-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
        {dispatchesQuery.error?.message || $t('common.error')}
      </div>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full min-w-[1120px] text-left text-sm">
          <thead class="border-b border-slate-200 bg-slate-50 text-xs font-semibold text-slate-500">
            <tr>
              <th class="px-4 py-3">Dispatch No.</th>
              <th class="px-4 py-3">Lot No.</th>
              <th class="px-4 py-3">Destination</th>
              <th class="px-4 py-3">Carrier</th>
              <th class="px-4 py-3">Quantity</th>
              <th class="px-4 py-3">Schedule</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            {#each dispatches as dispatch}
              <tr class="transition-colors hover:bg-slate-50" use:highlightOnChange={dispatch.id}>
                <td class="px-4 py-3 align-middle font-mono text-xs font-semibold text-slate-800">{dispatch.dispatchNumber}</td>
                <td class="px-4 py-3 align-middle font-mono text-xs">
                  {#if dispatch.lotId}
                    <a href="/lots/{dispatch.lotId}" class="font-semibold text-blue-600 hover:underline">{dispatch.lotNumber || dispatch.lotId}</a>
                  {:else}
                    <span class="text-slate-400">-</span>
                  {/if}
                </td>
                <td class="px-4 py-3 align-middle text-slate-700">{dispatch.destination}</td>
                <td class="px-4 py-3 align-middle text-slate-600">{dispatch.carrier || '-'}</td>
                <td class="whitespace-nowrap px-4 py-3 align-middle text-slate-700">{formatQuantity(dispatch.quantity)} {dispatch.unit}</td>
                <td class="whitespace-nowrap px-4 py-3 align-middle text-xs text-slate-500">{formatDateTime(dispatch.scheduledAt?.seconds)}</td>
                <td class="px-4 py-3 align-middle">
                  <span class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-semibold ring-1 ring-inset {statusClass(dispatch.status)}">
                    <span class="size-1.5 rounded-full {statusDot(dispatch.status)}"></span>
                    {$t(`dispatch_status.${dispatch.status}`)}
                  </span>
                </td>
                <td class="px-4 py-3 text-right align-middle">
                  <div class="flex flex-wrap justify-end gap-2">
                    {#if nextStatus(dispatch.status) !== null}
                      {@const to = nextStatus(dispatch.status)}
                      <button
                        onclick={() => advance(dispatch.id, to!)}
                        disabled={rowBusy === dispatch.id}
                        class="inline-flex h-8 items-center rounded-md bg-blue-600 px-3 text-xs font-semibold text-white shadow-sm transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
                      >
                        {$t('dispatch.advance_to').replace(' ->', '')} {$t(`dispatch_status.${to}`)}
                      </button>
                    {/if}
                    {#if !isTerminal(dispatch.status)}
                      <button
                        onclick={() => cancel(dispatch.id)}
                        disabled={rowBusy === dispatch.id}
                        class="inline-flex h-8 items-center rounded-md border border-red-200 bg-white px-3 text-xs font-semibold text-red-600 shadow-sm transition-colors hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50"
                      >
                        {$t('dispatch.cancel')}
                      </button>
                    {/if}
                  </div>
                </td>
              </tr>
            {:else}
              <tr>
                <td colspan="8" class="px-4 py-16">
                  <div class="mx-auto flex max-w-sm flex-col items-center text-center">
                    <div class="flex size-12 items-center justify-center rounded-lg bg-blue-100 text-blue-700">
                      <DashboardIcon name="check-circle" class="size-6" />
                    </div>
                    <h2 class="mt-3 text-sm font-semibold text-slate-950">{$t('dispatch.no_dispatches')}</h2>
                    <p class="mt-1 text-sm text-slate-500">Create a dispatch when a production-ready lot is ready to leave the facility.</p>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>
</div>

{#if showModal}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/55 p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="dispatch-title"
    tabindex="-1"
    use:focusTrap
    onclick={(e) => { if (e.target === e.currentTarget) showModal = false; }}
    onkeydown={(e) => { if (e.key === 'Escape') showModal = false; }}
  >
    <div class="max-h-[90vh] w-full max-w-2xl overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl">
      <div class="border-b border-slate-200 px-5 py-4">
        <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">Production Handoff</p>
        <h2 id="dispatch-title" class="mt-1 text-lg font-bold text-slate-950">{$t('dispatch.create_title')}</h2>
      </div>

      <div class="max-h-[66vh] space-y-4 overflow-y-auto px-5 py-4">
        <div>
          <label for="d-lot" class="mb-1 block text-sm font-semibold text-slate-700">{$t('dispatch.ready_lot')} <span class="text-red-500">*</span></label>
          <select
            id="d-lot"
            bind:value={form.lotId}
            onchange={onLotChange}
            class="h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-700 shadow-sm outline-none transition-colors hover:bg-slate-50 focus:border-blue-400 focus:ring-2 focus:ring-blue-100"
          >
            <option value="">{$t('dispatch.select_lot')}</option>
            {#each readyLots as lot}
              <option value={lot.id}>{lot.lotNumber} - {lot.materialName}</option>
            {/each}
          </select>
          {#if readyLots.length === 0}
            <p class="mt-1 text-xs text-slate-500">{$t('dispatch.no_ready_lots')}</p>
          {/if}
        </div>

        <div>
          <label for="d-dest" class="mb-1 block text-sm font-semibold text-slate-700">{$t('dispatch.destination')} <span class="text-red-500">*</span></label>
          <input id="d-dest" bind:value={form.destination} class="h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-700 shadow-sm outline-none transition-colors focus:border-blue-400 focus:ring-2 focus:ring-blue-100" />
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label for="d-carrier" class="mb-1 block text-sm font-semibold text-slate-700">{$t('dispatch.carrier')}</label>
            <input id="d-carrier" bind:value={form.carrier} class="h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-700 shadow-sm outline-none transition-colors focus:border-blue-400 focus:ring-2 focus:ring-blue-100" />
          </div>
          <div>
            <label for="d-sched" class="mb-1 block text-sm font-semibold text-slate-700">{$t('dispatch.scheduled_at')}</label>
            <input id="d-sched" type="datetime-local" bind:value={form.scheduledAt} class="h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-700 shadow-sm outline-none transition-colors focus:border-blue-400 focus:ring-2 focus:ring-blue-100" />
          </div>
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label for="d-qty" class="mb-1 block text-sm font-semibold text-slate-700">{$t('lot.quantity')} <span class="text-red-500">*</span></label>
            <input id="d-qty" type="number" min="0" step="0.001" bind:value={form.quantity} class="h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-700 shadow-sm outline-none transition-colors focus:border-blue-400 focus:ring-2 focus:ring-blue-100" />
          </div>
          <div>
            <label for="d-unit" class="mb-1 block text-sm font-semibold text-slate-700">{$t('lot.unit')}</label>
            <input id="d-unit" bind:value={form.unit} class="h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-700 shadow-sm outline-none transition-colors focus:border-blue-400 focus:ring-2 focus:ring-blue-100" />
          </div>
        </div>

        <div>
          <label for="d-notes" class="mb-1 block text-sm font-semibold text-slate-700">{$t('dispatch.notes')}</label>
          <textarea id="d-notes" bind:value={form.notes} rows="3" class="w-full rounded-md border border-slate-200 bg-white px-3 py-2 text-sm text-slate-700 shadow-sm outline-none transition-colors focus:border-blue-400 focus:ring-2 focus:ring-blue-100"></textarea>
        </div>

        {#if createError}
          <p class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{createError}</p>
        {/if}
      </div>

      <div class="flex justify-end gap-2 border-t border-slate-200 bg-slate-50 px-5 py-3">
        <button onclick={() => showModal = false} class="h-9 rounded-md border border-slate-200 bg-white px-4 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50">
          {$t('common.cancel')}
        </button>
        <button onclick={submitCreate} disabled={creating} class="h-9 rounded-md bg-blue-600 px-4 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50">
          {creating ? $t('dispatch.creating') : $t('dispatch.create_submit')}
        </button>
      </div>
    </div>
  </div>
{/if}
