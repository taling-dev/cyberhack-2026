<script lang="ts">
  import { t } from 'svelte-i18n';
  import { createQuery, getQueryClientContext } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { DashboardService } from '$lib/gen/simaops/dashboard/v1/dashboard_pb';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { WarehouseService } from '$lib/gen/simaops/warehouse/v1/warehouse_pb';
  import DashboardIcon from '$lib/components/dashboard/DashboardIcon.svelte';
  import KpiCard from '$lib/components/dashboard/KpiCard.svelte';
  import WarehouseCapacityCard from '$lib/components/dashboard/WarehouseCapacityCard.svelte';
  import { highlightOnChange } from '$lib/actions/highlightOnChange.svelte';
  import { focusTrap } from '$lib/actions/focusTrap.svelte';

  const dashboardClient = createClient(DashboardService, transport);
  const lotClient = createClient(LotService, transport);
  const whClient = createClient(WarehouseService, transport);
  const queryClient = getQueryClientContext();

  // Tabs: 'pending' = lots awaiting assignment, 'assigned' = lots with slots
  let activeTab = $state<'pending' | 'assigned'>('pending');

  // Status 5 = QC_APPROVED (pending), 7 = WAREHOUSE_ASSIGNED, 8 = READY_FOR_PRODUCTION (assigned)
  const pendingLotsQuery = createQuery(() => ({
    queryKey: ['warehouse-queue'],
    queryFn: () => lotClient.listLots({ pageSize: 50, statusFilter: 5 }),
    refetchInterval: 15_000,
    enabled: activeTab === 'pending',
  }));

  const assignedLotsQuery = createQuery(() => ({
    queryKey: ['warehouse-assigned'],
    queryFn: async () => {
      // Get lots that are WAREHOUSE_ASSIGNED (7) or READY_FOR_PRODUCTION (8)
      const assigned = await lotClient.listLots({ pageSize: 50, statusFilter: 7 });
      const ready = await lotClient.listLots({ pageSize: 50, statusFilter: 8 });
      return [...(assigned.lots ?? []), ...(ready.lots ?? [])];
    },
    refetchInterval: 15_000,
    enabled: activeTab === 'assigned',
  }));

  // Query for slot decisions for a lot
  async function getSlotDecisions(lotId: string) {
    try {
      const res = await whClient.listSlotDecisions({ lotId });
      return res.decisions ?? [];
    } catch {
      return [];
    }
  }

  const warehouseMetricsQuery = createQuery(() => ({
    queryKey: ['dashboard-warehouse'],
    queryFn: () => dashboardClient.getWarehouseMetrics({}),
    refetchInterval: 30_000,
  }));

  const coldChainQuery = createQuery(() => ({
    queryKey: ['coldchain-status'],
    queryFn: async () => {
      const r = await fetch('/api/coldchain');
      if (!r.ok) throw new Error('coldchain');
      return r.json();
    },
    refetchInterval: 10_000,
  }));

  let showModal = $state(false);
  let selectedLotId = $state('');
  let selectedLotNumber = $state('');
  let recommendations = $state<any[]>([]);
  let loadingRecs = $state(false);
  let assigning = $state(false);
  let assignError = $state('');
  let assignSuccess = $state('');

  // Unassign modal state
  let showUnassignModal = $state(false);
  let unassignLotId = $state('');
  let unassignLotNumber = $state('');
  let unassignReason = $state('');
  let unassigning = $state(false);
  let unassignError = $state('');

  function handleKeydown(event: KeyboardEvent) {
    if (showModal && event.key === 'Escape') showModal = false;
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
    } catch (error: any) {
      assignError = error.message || 'Failed to load recommendations';
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
        idempotencyKey: crypto.randomUUID(),
      });
      showModal = false;
      assignSuccess = `${selectedLotNumber} assigned to ${res.assignment?.locationCode ?? 'slot'}`;
      queryClient.invalidateQueries({ queryKey: ['warehouse-queue'] });
      queryClient.invalidateQueries({ queryKey: ['warehouse-assigned'] });
    } catch (error: any) {
      assignError = error.message || 'Assignment failed';
    } finally {
      assigning = false;
    }
  }

  async function openUnassign(lotId: string, lotNumber: string) {
    unassignLotId = lotId;
    unassignLotNumber = lotNumber;
    unassignReason = '';
    unassignError = '';
    showUnassignModal = true;
  }

  async function doUnassign() {
    if (!unassignReason.trim()) {
      unassignError = 'Please provide a reason for unassigning';
      return;
    }
    unassigning = true;
    unassignError = '';
    try {
      await whClient.unassignSlot({
        lotId: unassignLotId,
        reason: unassignReason,
        idempotencyKey: crypto.randomUUID(),
      });
      showUnassignModal = false;
      assignSuccess = `${unassignLotNumber} slot released`;
      queryClient.invalidateQueries({ queryKey: ['warehouse-queue'] });
      queryClient.invalidateQueries({ queryKey: ['warehouse-assigned'] });
    } catch (error: any) {
      unassignError = error.message || 'Unassign failed';
    } finally {
      unassigning = false;
    }
  }

  function utilization(occupied?: number, total?: number) {
    return total && total > 0 ? occupied ?? 0 / total : 0;
  }

  function utilizationPercent(occupied?: number, total?: number) {
    if (!total || total <= 0) return '0%';
    return `${Math.round(((occupied ?? 0) / total) * 100)}%`;
  }

  function formatNumber(value?: number | null) {
    return value == null ? '-' : value.toLocaleString('en-US');
  }

  function formatQuantity(value: number) {
    return Number.isInteger(value)
      ? value.toLocaleString('en-US')
      : value.toLocaleString('en-US', { maximumFractionDigits: 2 });
  }

  function storageLabel(lot: any) {
    const temp = lot.storageRequirement?.temperatureRange;
    const hazard = lot.storageRequirement?.hazardClass;
    const tempLabel = temp ? $t(`temp_range.${temp}`).split(' (')[0] : '-';
    const hazardLabel = hazard === 2 ? 'IBC' : hazard === 3 ? 'IPPC' : '';
    return hazardLabel ? `${tempLabel} + ${hazardLabel}` : tempLabel;
  }

  function conditionFor(pct: number) {
    if (pct >= 90) return { label: 'Critical', tone: 'bg-red-100 text-red-700' };
    if (pct >= 75) return { label: 'High', tone: 'bg-orange-100 text-orange-700' };
    return { label: 'Stable', tone: 'bg-emerald-100 text-emerald-700' };
  }

  function scoreLabel(score?: number) {
    if (score == null) return 'N/A';
    return score.toLocaleString('en-US', { maximumFractionDigits: 1 });
  }

  const lots = $derived(pendingLotsQuery.data?.lots ?? []);
  const assignedLots = $derived(assignedLotsQuery.data ?? []);
  const totalCapacity = $derived(warehouseMetricsQuery.data?.totalCapacity ?? 0);
  const totalOccupied = $derived(warehouseMetricsQuery.data?.totalOccupied ?? 0);
  const totalAvailable = $derived(warehouseMetricsQuery.data?.totalAvailable ?? 0);
  const warehousePct = $derived(utilizationPercent(totalOccupied, totalCapacity));
  const coldChain = $derived(coldChainQuery.data?.equipment ?? []);

  // Cold-chain WARNING/CRITICAL toasts are emitted app-wide from the root
  // layout (gated to warehouse-view roles); the panel below just displays them.

</script>

<svelte:window onkeydown={handleKeydown} />

<div class="space-y-5">
  <header class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
    <div>
      <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">Storage Operations</p>
      <h1 class="mt-1 text-[28px] font-bold tracking-normal text-slate-950">{$t('nav.warehouse')}</h1>
    </div>

    <div class="flex items-center gap-3">
      <span class="rounded-md border border-slate-200 bg-white px-3 py-2 text-xs font-medium text-slate-500 shadow-sm">
        {lots.length} lots awaiting slot assignment
      </span>
      <button
        type="button"
        onclick={() => {
          lotsQuery.refetch();
          warehouseMetricsQuery.refetch();
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
    </div>
  </header>

  {#if assignSuccess}
    <div class="rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm font-medium text-emerald-700" role="status">
      {$t('warehouse.assigned_msg')}: {assignSuccess}
    </div>
  {/if}

  <section class="grid grid-cols-2 gap-3 lg:grid-cols-4">
    <KpiCard title="Total Capacity" value={formatNumber(totalCapacity)} icon="warehouse" tone="blue" loading={warehouseMetricsQuery.isLoading} />
    <KpiCard title="Used Slots" value={formatNumber(totalOccupied)} icon="database" tone="purple" loading={warehouseMetricsQuery.isLoading} />
    <KpiCard title="Available Slots" value={formatNumber(totalAvailable)} icon="check-circle" tone="green" loading={warehouseMetricsQuery.isLoading} />
    <KpiCard title="Utilization Rate" value={warehousePct} icon="percent" tone="orange" loading={warehouseMetricsQuery.isLoading} />
  </section>

  <div class="grid gap-4 xl:grid-cols-[1fr_1.05fr]">
    <section class="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
      <div class="mb-4 flex items-center justify-between">
        <div>
          <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">Storage Zones</h2>
          <p class="mt-1 text-xs text-slate-500">Aggregate capacity by warehouse zone.</p>
        </div>
        <DashboardIcon name="warehouse" class="size-5 text-slate-400" />
      </div>

      {#if warehouseMetricsQuery.isLoading}
        <div class="grid gap-3 md:grid-cols-3 xl:grid-cols-1 2xl:grid-cols-3">
          {#each [1, 2, 3] as _}
            <div class="h-28 animate-pulse rounded-lg bg-slate-100"></div>
          {/each}
        </div>
      {:else if warehouseMetricsQuery.isError}
        <div class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {warehouseMetricsQuery.error?.message || $t('common.error')}
        </div>
      {:else if (warehouseMetricsQuery.data?.zones?.length ?? 0) === 0}
        <div class="flex h-40 items-center justify-center rounded-lg bg-slate-50 text-sm text-slate-500">
          No warehouse zone data available.
        </div>
      {:else}
        <div class="grid gap-3 md:grid-cols-3 xl:grid-cols-1 2xl:grid-cols-3">
          {#each warehouseMetricsQuery.data?.zones ?? [] as zone}
            {@const usedPct = zone.totalCapacity > 0 ? Math.round((zone.occupied / zone.totalCapacity) * 100) : 0}
            {@const condition = conditionFor(usedPct)}
            <article class="rounded-lg border border-slate-200 bg-slate-50/60 p-4">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <h3 class="truncate text-sm font-bold text-slate-950">{zone.zone}</h3>
                  <p class="mt-1 text-xs text-slate-500">{zone.available.toLocaleString('en-US')} slots available</p>
                </div>
                <span class="rounded-md px-2 py-1 text-xs font-semibold {condition.tone}">{condition.label}</span>
              </div>

              <div class="mt-4">
                <div class="mb-1 flex items-center justify-between text-xs">
                  <span class="text-slate-500">Utilization</span>
                  <span class="font-semibold text-slate-950">{zone.occupied.toLocaleString('en-US')} / {zone.totalCapacity.toLocaleString('en-US')}</span>
                </div>
                <div
                  class="h-2 overflow-hidden rounded-full bg-white"
                  role="progressbar"
                  aria-valuenow={usedPct}
                  aria-valuemin="0"
                  aria-valuemax="100"
                >
                  <div class="h-full rounded-full bg-blue-600 transition-all" style="width: {usedPct}%"></div>
                </div>
                <p class="mt-2 text-right text-xs font-bold text-slate-950">{usedPct}%</p>
              </div>
            </article>
          {/each}
        </div>
      {/if}
    </section>

    <WarehouseCapacityCard
      zones={warehouseMetricsQuery.data?.zones ?? []}
      totalCapacity={totalCapacity}
      totalOccupied={totalOccupied}
      loading={warehouseMetricsQuery.isLoading}
    />
  </div>

  {#if coldChain.length > 0}
    <section class="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
      <div class="mb-4 flex items-center justify-between">
        <div>
          <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">🧊 {$t('warehouse.coldchain_title')}</h2>
          <p class="mt-1 text-xs text-slate-500">Live per-zone temperature and equipment health.</p>
        </div>
        <DashboardIcon name="warehouse" class="size-5 text-slate-400" />
      </div>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
        {#each coldChain as eq}
          {@const h = eq.health?.status ?? 'NO_DATA'}
          <article class="rounded-lg border p-3 {h === 'CRITICAL' ? 'border-red-200 bg-red-50' : h === 'WARNING' ? 'border-amber-200 bg-amber-50' : 'border-emerald-200 bg-emerald-50'}">
            <div class="flex items-center justify-between">
              <span class="text-sm font-bold text-slate-950">{$t('warehouse.zone')} {eq.equipment_id}</span>
              <span class="text-xs font-semibold {h === 'CRITICAL' ? 'text-red-700' : h === 'WARNING' ? 'text-amber-700' : 'text-emerald-700'}">{eq.health?.health_score ?? '—'}</span>
            </div>
            <div class="mt-1 font-mono text-lg text-slate-950">{eq.latest_temperature ?? '—'}°C</div>
            {#if eq.latest_alert}
              <p class="mt-1 text-xs text-red-600">⚠ {eq.latest_alert.message}</p>
            {/if}
          </article>
        {/each}
      </div>
    </section>
  {/if}

  <section class="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
    <!-- Tabs -->
    <div class="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3">
      <div class="flex items-center gap-4">
        <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">Assignment Queue</h2>
        <div class="flex items-center gap-1 rounded-lg bg-slate-100 p-1">
          <button
            onclick={() => activeTab = 'pending'}
            class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors {activeTab === 'pending' ? 'bg-white text-slate-950 shadow-sm' : 'text-slate-600 hover:text-slate-950'}"
          >
            Pending
            <span class="ml-1 rounded-full bg-emerald-100 px-1.5 py-0.5 text-xs">{lots.length}</span>
          </button>
          <button
            onclick={() => activeTab = 'assigned'}
            class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors {activeTab === 'assigned' ? 'bg-white text-slate-950 shadow-sm' : 'text-slate-600 hover:text-slate-950'}"
          >
            Assigned
            <span class="ml-1 rounded-full bg-blue-100 px-1.5 py-0.5 text-xs">{assignedLots.length}</span>
          </button>
        </div>
      </div>
      <span class="text-xs text-slate-500">{activeTab === 'pending' ? 'QC-approved lots ready for slotting' : 'Lots with warehouse assignments'}</span>
    </div>

    {#if activeTab === 'pending'}
      <!-- Pending lots table -->
      {#if pendingLotsQuery.isLoading}
        <div class="divide-y divide-slate-100">
          {#each [1, 2, 3, 4, 5] as _}
            <div class="grid grid-cols-[1fr_1fr_1fr_.75fr_1fr_.7fr_.8fr] gap-4 px-4 py-4">
              {#each [1, 2, 3, 4, 5, 6, 7] as __}
                <div class="h-4 animate-pulse rounded bg-slate-100"></div>
              {/each}
            </div>
          {/each}
        </div>
      {:else if pendingLotsQuery.isError}
        <div class="m-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {pendingLotsQuery.error?.message || $t('common.error')}
        </div>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full min-w-[980px] text-left text-sm">
            <thead class="border-b border-slate-200 bg-slate-50 text-xs font-semibold text-slate-500">
              <tr>
                <th class="px-4 py-3">Lot No.</th>
                <th class="px-4 py-3">Material</th>
                <th class="px-4 py-3">Supplier</th>
                <th class="px-4 py-3">Quantity</th>
                <th class="px-4 py-3">Storage Requirement</th>
                <th class="px-4 py-3">Status</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-100">
              {#each lots as lot}
                <tr class="transition-colors hover:bg-slate-50" use:highlightOnChange={lot.id}>
                  <td class="px-4 py-3 align-middle">
                    <a href="/lots/{lot.id}" class="font-mono text-xs font-semibold text-blue-600 hover:underline">{lot.lotNumber}</a>
                  </td>
                  <td class="px-4 py-3 align-middle">
                    <div class="font-medium text-slate-950">{lot.materialName}</div>
                    <div class="mt-0.5 text-xs text-slate-500">{$t(`material_type.${lot.materialType}`)}</div>
                  </td>
                  <td class="px-4 py-3 align-middle text-slate-700">{lot.supplierName}</td>
                  <td class="whitespace-nowrap px-4 py-3 align-middle text-slate-700">{formatQuantity(lot.quantity)} {lot.unit}</td>
                  <td class="px-4 py-3 align-middle">
                    <span class="inline-flex rounded-md bg-slate-100 px-2 py-1 text-xs font-medium text-slate-700">
                      {storageLabel(lot)}
                    </span>
                  </td>
                  <td class="px-4 py-3 align-middle">
                    <span class="inline-flex items-center gap-1.5 rounded-md bg-emerald-100 px-2.5 py-1 text-xs font-semibold text-emerald-700 ring-1 ring-inset ring-emerald-200">
                      <span class="size-1.5 rounded-full bg-emerald-500"></span>
                      {$t('lot_status.5')}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-right align-middle">
                    <button
                      onclick={() => openAssign(lot.id, lot.lotNumber)}
                      class="inline-flex h-8 items-center rounded-md bg-purple-600 px-3 text-xs font-semibold text-white shadow-sm transition-colors hover:bg-purple-700"
                    >
                      {$t('warehouse.assign_button').replace(' ->', '')}
                    </button>
                  </td>
                </tr>
              {:else}
                <tr>
                  <td colspan="7" class="px-4 py-16">
                    <div class="mx-auto flex max-w-sm flex-col items-center text-center">
                      <div class="flex size-12 items-center justify-center rounded-lg bg-emerald-100 text-emerald-700">
                        <DashboardIcon name="check-circle" class="size-6" />
                      </div>
                      <h2 class="mt-3 text-sm font-semibold text-slate-950">{$t('warehouse.no_queue')}</h2>
                      <p class="mt-1 text-sm text-slate-500">New QC-approved lots will appear here when they are ready for warehouse assignment.</p>
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    {:else}
      <!-- Assigned lots table with slot info and unassign -->
      {#if assignedLotsQuery.isLoading}
        <div class="divide-y divide-slate-100">
          {#each [1, 2, 3, 4, 5] as _}
            <div class="h-16 animate-pulse bg-slate-50"></div>
          {/each}
        </div>
      {:else if assignedLots.length === 0}
        <div class="px-4 py-16">
          <div class="mx-auto flex max-w-sm flex-col items-center text-center">
            <div class="flex size-12 items-center justify-center rounded-lg bg-slate-100 text-slate-400">
              <DashboardIcon name="warehouse" class="size-6" />
            </div>
            <h2 class="mt-3 text-sm font-semibold text-slate-950">No assigned lots</h2>
            <p class="mt-1 text-sm text-slate-500">Lots with warehouse assignments will appear here.</p>
          </div>
        </div>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full min-w-[1100px] text-left text-sm">
            <thead class="border-b border-slate-200 bg-slate-50 text-xs font-semibold text-slate-500">
              <tr>
                <th class="px-4 py-3">Lot No.</th>
                <th class="px-4 py-3">Material</th>
                <th class="px-4 py-3">Supplier</th>
                <th class="px-4 py-3">Quantity</th>
                <th class="px-4 py-3">Storage Requirement</th>
                <th class="px-4 py-3">Status</th>
                <th class="px-4 py-3">Decision</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-100">
              {#each assignedLots as lot}
                {@const decisionType = lot.assignment?.decisionType}
                {@const decisionLabel = decisionType === 1 ? 'Auto' : decisionType === 2 ? 'Manual' : decisionType === 3 ? 'Override' : 'Manual'}
                {@const decisionTone = decisionType === 1 ? 'bg-blue-100 text-blue-700' : decisionType === 3 ? 'bg-amber-100 text-amber-700' : 'bg-slate-100 text-slate-700'}
                <tr class="transition-colors hover:bg-slate-50">
                  <td class="px-4 py-3 align-middle">
                    <a href="/lots/{lot.id}" class="font-mono text-xs font-semibold text-blue-600 hover:underline">{lot.lotNumber}</a>
                  </td>
                  <td class="px-4 py-3 align-middle">
                    <div class="font-medium text-slate-950">{lot.materialName}</div>
                    <div class="mt-0.5 text-xs text-slate-500">{$t(`material_type.${lot.materialType}`)}</div>
                  </td>
                  <td class="px-4 py-3 align-middle text-slate-700">{lot.supplierName}</td>
                  <td class="whitespace-nowrap px-4 py-3 align-middle text-slate-700">{formatQuantity(lot.quantity)} {lot.unit}</td>
                  <td class="px-4 py-3 align-middle">
                    <span class="inline-flex rounded-md bg-slate-100 px-2 py-1 text-xs font-medium text-slate-700">
                      {storageLabel(lot)}
                    </span>
                  </td>
                  <td class="px-4 py-3 align-middle">
                    {#if lot.status === 7}
                      <span class="inline-flex items-center gap-1.5 rounded-md bg-purple-100 px-2.5 py-1 text-xs font-semibold text-purple-700 ring-1 ring-inset ring-purple-200">
                        {$t('lot_status.7')}
                      </span>
                    {:else}
                      <span class="inline-flex items-center gap-1.5 rounded-md bg-green-100 px-2.5 py-1 text-xs font-semibold text-green-700 ring-1 ring-inset ring-green-200">
                        {$t('lot_status.8')}
                      </span>
                    {/if}
                  </td>
                  <td class="px-4 py-3 align-middle">
                    <span class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-semibold {decisionTone}">
                      {#if decisionType === 1}
                        <svg viewBox="0 0 24 24" class="size-3" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm0 18a8 8 0 1 1 8-8 8 8 0 0 1-8 8z"/><path d="M12 6v6l4 2"/></svg>
                        Auto-assigned
                      {:else if decisionType === 3}
                        <svg viewBox="0 0 24 24" class="size-3" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 4v6h6M23 20v-6h-6"/><path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 4m22 6-4.64 4.36A9 9 0 0 1 3.51 15"/></svg>
                        Override
                      {:else}
                        Manual
                      {/if}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-right align-middle">
                    <button
                      onclick={() => openUnassign(lot.id, lot.lotNumber)}
                      class="inline-flex h-8 items-center gap-1.5 rounded-md border border-red-200 bg-white px-3 text-xs font-semibold text-red-600 shadow-sm transition-colors hover:bg-red-50"
                    >
                      <svg viewBox="0 0 24 24" class="size-3.5" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 6h18M8 6V4h8v2M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6"/></svg>
                      Unassign
                    </button>
                  </td>
                </tr>
              {:else}
                <tr>
                  <td colspan="8" class="px-4 py-16">
                    <div class="mx-auto flex max-w-sm flex-col items-center text-center">
                      <div class="flex size-12 items-center justify-center rounded-lg bg-slate-100 text-slate-400">
                        <DashboardIcon name="warehouse" class="size-6" />
                      </div>
                      <h2 class="mt-3 text-sm font-semibold text-slate-950">No assigned lots</h2>
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    {/if}
  </section>
</div>

{#if showModal}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/55 p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="assign-title"
    tabindex="-1"
    use:focusTrap
    onclick={(event) => {
      if (event.target === event.currentTarget) showModal = false;
    }}
    onkeydown={(event) => {
      if (event.key === 'Escape') showModal = false;
    }}
  >
    <div class="max-h-[90vh] w-full max-w-2xl overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl">
      <div class="border-b border-slate-200 px-5 py-4">
        <p class="text-xs font-semibold uppercase tracking-normal text-purple-600">Recommended Slots</p>
        <h2 id="assign-title" class="mt-1 text-lg font-bold text-slate-950">{$t('warehouse.assign_modal_title')}: {selectedLotNumber}</h2>
      </div>

      <div class="max-h-[62vh] overflow-y-auto px-5 py-4">
        {#if loadingRecs}
          <div class="space-y-3">
            {#each [1, 2, 3] as _}
              <div class="h-20 animate-pulse rounded-lg bg-slate-100"></div>
            {/each}
          </div>
        {:else if recommendations.length === 0}
          <div class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
            {$t('warehouse.no_recs')}
          </div>
        {:else}
          <div class="space-y-3">
            <p class="text-sm text-slate-500">{$t('warehouse.recs_label')}</p>
            {#each recommendations as rec}
              <article class="rounded-lg border border-slate-200 bg-white p-4 shadow-sm transition-colors hover:bg-slate-50">
                <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div class="min-w-0">
                    <div class="flex flex-wrap items-center gap-2">
                      <span class="font-mono text-sm font-bold text-slate-950">{rec.location?.code}</span>
                      <span class="rounded-md bg-slate-100 px-2 py-1 text-xs font-medium text-slate-600">{rec.location?.zone}</span>
                      <span class="rounded-md bg-blue-100 px-2 py-1 text-xs font-semibold text-blue-700">Score {scoreLabel(rec.score)}</span>
                    </div>
                    <p class="mt-2 text-xs leading-5 text-slate-500">{rec.reason}</p>
                    <p class="mt-1 text-xs text-slate-500">Remaining capacity: {rec.location?.capacity ?? 0}</p>
                  </div>
                  <button
                    onclick={() => doAssign(rec.location?.id)}
                    disabled={assigning || !rec.location?.id}
                    class="inline-flex h-9 shrink-0 items-center justify-center rounded-md bg-purple-600 px-3 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-purple-700 disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {assigning ? $t('warehouse.assigning') : 'Assign'}
                  </button>
                </div>
              </article>
            {/each}
          </div>
        {/if}

        {#if assignError}
          <p class="mt-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{assignError}</p>
        {/if}
      </div>

      <div class="flex justify-end border-t border-slate-200 bg-slate-50 px-5 py-3">
        <button onclick={() => showModal = false} class="h-9 rounded-md border border-slate-200 bg-white px-4 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50">
          {$t('common.cancel')}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Unassign Modal -->
{#if showUnassignModal}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/55 p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="unassign-title"
    tabindex="-1"
    use:focusTrap
    onclick={(event) => {
      if (event.target === event.currentTarget) showUnassignModal = false;
    }}
    onkeydown={(event) => {
      if (event.key === 'Escape') showUnassignModal = false;
    }}
  >
    <div class="w-full max-w-md overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl">
      <div class="border-b border-slate-200 px-5 py-4">
        <p class="text-xs font-semibold uppercase tracking-normal text-red-600">Release Slot Assignment</p>
        <h2 id="unassign-title" class="mt-1 text-lg font-bold text-slate-950">{$t('warehouse.unassign_title')}: {unassignLotNumber}</h2>
      </div>

      <div class="px-5 py-4">
        <p class="mb-4 text-sm text-slate-600">
          This will release the slot and move the lot back to QC_APPROVED status.
          A new slot can be assigned after this action.
        </p>

        <label for="unassign-reason" class="block text-sm font-medium text-slate-700">
          Reason for unassigning <span class="text-red-500">*</span>
        </label>
        <textarea
          id="unassign-reason"
          bind:value={unassignReason}
          rows="3"
          placeholder="Why are you releasing this slot?"
          class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-purple-500 focus:outline-none focus:ring-1 focus:ring-purple-500"
        ></textarea>

        {#if unassignError}
          <p class="mt-2 text-sm text-red-600" role="alert">{unassignError}</p>
        {/if}
      </div>

      <div class="flex justify-end gap-3 border-t border-slate-200 bg-slate-50 px-5 py-3">
        <button
          onclick={() => showUnassignModal = false}
          class="h-9 rounded-md border border-slate-200 bg-white px-4 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50"
        >
          {$t('common.cancel')}
        </button>
        <button
          onclick={doUnassign}
          disabled={unassigning || !unassignReason.trim()}
          class="inline-flex h-9 items-center gap-2 rounded-md bg-red-600 px-4 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {unassigning ? 'Releasing...' : 'Release Slot'}
        </button>
      </div>
    </div>
  </div>
{/if}
