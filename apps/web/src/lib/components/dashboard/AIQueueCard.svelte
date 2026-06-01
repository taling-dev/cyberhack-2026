<script lang="ts">
  import { t } from 'svelte-i18n';
  import DashboardIcon from './DashboardIcon.svelte';

  type LotLike = {
    id: string;
    lotNumber: string;
    materialName: string;
    supplierName: string;
  };

  let {
    lots = [],
    loading = false,
  } = $props<{
    lots?: LotLike[];
    loading?: boolean;
  }>();
</script>

<section class="flex h-full min-h-0 flex-col rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
  <div class="flex items-center justify-between">
    <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">{$t('widgets.ai_queue')}</h2>
    <span class="rounded-full bg-orange-100 px-2 py-0.5 text-xs font-semibold text-orange-700">{loading ? '-' : lots.length}</span>
  </div>

  <div class="mt-4 min-h-0 flex-1 overflow-hidden">
    <p class="mb-3 text-sm font-semibold text-slate-950">{$t('widgets.awaiting_review')}</p>

    {#if loading}
      <div class="space-y-3">
        {#each [1, 2, 3] as _}
          <div class="h-10 animate-pulse rounded bg-slate-100"></div>
        {/each}
      </div>
    {:else if lots.length === 0}
      <div class="flex h-32 flex-col items-center justify-center rounded-md bg-slate-50 text-center">
        <DashboardIcon name="check-circle" class="size-6 text-emerald-600" />
        <p class="mt-2 text-sm font-medium text-slate-700">{$t('widgets.queue_clear')}</p>
        <p class="text-xs text-slate-500">No lots currently require QC review.</p>
      </div>
    {:else}
      <div class="space-y-3">
        {#each lots.slice(0, 4) as lot}
          <a href="/qc/{lot.id}" class="group flex items-center gap-3 rounded-md border border-transparent py-1.5 transition-colors hover:border-orange-100 hover:bg-orange-50/60">
            <span class="flex size-8 shrink-0 items-center justify-center rounded-md bg-orange-100 text-orange-700">
              <DashboardIcon name="bot" class="size-4" />
            </span>
            <span class="min-w-0 flex-1">
              <span class="block truncate text-xs font-semibold text-slate-950">{lot.lotNumber}</span>
              <span class="block truncate text-[11px] text-slate-500">{lot.materialName} - {lot.supplierName}</span>
            </span>
            <DashboardIcon name="arrow-right" class="mr-1 size-4 text-slate-400 transition-colors group-hover:text-orange-600" />
          </a>
        {/each}
      </div>
    {/if}
  </div>

  <a href="/qc" class="mt-3 flex h-9 items-center justify-center gap-2 rounded-md border border-slate-200 text-sm font-medium text-blue-600 transition-colors hover:bg-blue-50">
    {$t('common.see_all')} QC Reviews
    <DashboardIcon name="arrow-right" class="size-4" />
  </a>
</section>
