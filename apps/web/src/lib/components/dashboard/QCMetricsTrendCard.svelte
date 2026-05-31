<script lang="ts">
  import DashboardIcon from './DashboardIcon.svelte';

  let {
    passCount = 0,
    reviewCount = 0,
    failCount = 0,
    loading = false,
  } = $props<{
    passCount?: number;
    reviewCount?: number;
    failCount?: number;
    loading?: boolean;
  }>();

  const total = $derived(passCount + reviewCount + failCount);
</script>

<section class="flex h-full min-h-0 flex-col rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
  <div class="flex items-center justify-between">
    <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">QC Trends (Last 7 Days)</h2>
    <DashboardIcon name="chart" class="size-4 text-slate-400" />
  </div>

  <div class="relative mt-3 min-h-0 flex-1 overflow-hidden rounded-md border border-slate-100 bg-slate-50">
    <svg class="absolute inset-0 size-full" viewBox="0 0 520 170" preserveAspectRatio="none" aria-hidden="true">
      {#each [30, 65, 100, 135] as y}
        <path d="M32 {y} H500" stroke="#dfe5ed" stroke-width="1" />
      {/each}
      {#each [92, 164, 236, 308, 380, 452] as x}
        <path d="M{x} 24 V148" stroke="#eef2f6" stroke-width="1" />
      {/each}
    </svg>

    <div class="absolute inset-0 flex flex-col items-center justify-center px-6 text-center">
      {#if loading}
        <div class="h-3 w-32 animate-pulse rounded bg-slate-200"></div>
        <div class="mt-3 h-3 w-44 animate-pulse rounded bg-slate-200"></div>
      {:else}
        <p class="text-sm font-semibold text-slate-700">Trend history unavailable</p>
        <p class="mt-1 max-w-56 text-xs leading-5 text-slate-500">Showing the current 24h QC snapshot only.</p>
      {/if}
    </div>
  </div>

  <div class="mt-3 grid grid-cols-4 gap-2 text-xs">
    <div>
      <p class="text-slate-500">Total</p>
      <p class="mt-0.5 font-bold text-slate-950">{loading ? '-' : total}</p>
    </div>
    <div>
      <p class="text-emerald-600">Pass</p>
      <p class="mt-0.5 font-bold text-slate-950">{loading ? '-' : passCount}</p>
    </div>
    <div>
      <p class="text-orange-600">Review</p>
      <p class="mt-0.5 font-bold text-slate-950">{loading ? '-' : reviewCount}</p>
    </div>
    <div>
      <p class="text-red-600">Fail</p>
      <p class="mt-0.5 font-bold text-slate-950">{loading ? '-' : failCount}</p>
    </div>
  </div>
</section>
