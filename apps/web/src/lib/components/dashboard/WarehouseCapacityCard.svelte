<script lang="ts">
  import { t } from 'svelte-i18n';
  import DashboardIcon from './DashboardIcon.svelte';

  type ZoneMetricLike = {
    zone: string;
    totalCapacity: number;
    occupied: number;
    available: number;
  };

  const zoneColors = ['#16a34a', '#6366f1', '#c026d3', '#0ea5e9'];

  let {
    zones = [],
    totalCapacity = 0,
    totalOccupied = 0,
    loading = false,
  } = $props<{
    zones?: ZoneMetricLike[];
    totalCapacity?: number;
    totalOccupied?: number;
    loading?: boolean;
  }>();

  function pct(occupied: number, capacity: number) {
    return capacity > 0 ? Math.round((occupied / capacity) * 100) : 0;
  }
</script>

<section class="flex h-full min-h-0 flex-col rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
  <div class="flex items-center justify-between">
    <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">{$t('widgets.warehouse_capacity')}</h2>
    <span class="text-xs font-semibold text-slate-500">{pct(totalOccupied, totalCapacity)}%</span>
  </div>

  <div class="mt-4 min-h-0 flex-1 space-y-3 overflow-hidden">
    {#if loading}
      {#each [1, 2, 3] as _}
        <div class="h-12 animate-pulse rounded bg-slate-100"></div>
      {/each}
    {:else if zones.length === 0}
      <div class="flex h-full items-center justify-center rounded-md bg-slate-50 text-sm text-slate-500">
        No warehouse zone data available.
      </div>
    {:else}
      {#each zones.slice(0, 4) as zone, index}
        {@const usedPct = pct(zone.occupied, zone.totalCapacity)}
        <div class="grid grid-cols-[30px_1fr_42px] items-center gap-3">
          <div class="flex size-8 items-center justify-center rounded-md bg-slate-50 text-slate-700">
            <DashboardIcon name="warehouse" class="size-5" />
          </div>
          <div class="min-w-0">
            <div class="mb-1 flex items-center justify-between gap-3 text-xs">
              <span class="truncate font-medium text-slate-950">{zone.zone}</span>
              <span class="shrink-0 text-slate-500">{zone.occupied.toLocaleString('en-US')} / {zone.totalCapacity.toLocaleString('en-US')}</span>
            </div>
            <div
              class="h-1.5 overflow-hidden rounded-full bg-slate-100"
              role="progressbar"
              aria-valuenow={usedPct}
              aria-valuemin="0"
              aria-valuemax="100"
            >
              <div
                class="h-full rounded-full transition-all"
                style="width: {usedPct}%; background-color: {zoneColors[index % zoneColors.length]};"
              ></div>
            </div>
          </div>
          <span class="text-right text-xs font-semibold text-slate-950">{usedPct}%</span>
        </div>
      {/each}
    {/if}
  </div>
</section>
