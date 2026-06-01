<script lang="ts">
  import { t } from 'svelte-i18n';
  import DashboardIcon from './DashboardIcon.svelte';

  type Inspection = {
    present?: boolean;
    lotId?: string;
    lotNumber?: string;
    materialName?: string;
    recommendation?: number;
    confidence?: number;
    imageObjectKey?: string;
    createdAtUnix?: bigint | number | string;
    findings?: Array<{ mappedFinding?: string; confidence?: number; isAnomaly?: boolean }>;
  };

  let {
    inspection = null,
    imageUrl = '',
    loading = false,
    unavailable = false,
  } = $props<{
    inspection?: Inspection | null;
    imageUrl?: string;
    loading?: boolean;
    unavailable?: boolean;
  }>();

  const present = $derived(!!inspection?.present);

  function recommendationLabel(value?: number) {
    if (value === 1) return 'PASS';
    if (value === 2) return 'REVIEW';
    if (value === 3) return 'FAIL';
    return 'Pending';
  }
  function recommendationClass(value?: number) {
    if (value === 1) return 'bg-emerald-100 text-emerald-700';
    if (value === 2) return 'bg-orange-100 text-orange-700';
    if (value === 3) return 'bg-red-100 text-red-700';
    return 'bg-slate-100 text-slate-600';
  }
  function pct(value?: number) {
    return value != null ? `${Math.round(value * 1000) / 10}%` : '-';
  }
  function timestampLabel(unix?: bigint | number | string) {
    if (!unix) return '-';
    return new Date(Number(unix) * 1000).toLocaleString('en-US', {
      month: '2-digit', day: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit',
    });
  }
</script>

<section class="flex h-full min-h-0 flex-col rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
  <div class="flex items-center justify-between">
    <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">{$t('widgets.latest_inspection')}</h2>
    {#if present && inspection?.lotId}
      <a href="/qc/{inspection.lotId}" class="text-blue-600" aria-label="Open latest inspection">
        <DashboardIcon name="arrow-right" class="size-5" />
      </a>
    {/if}
  </div>

  <div class="mt-4 min-h-0 flex-1 overflow-hidden">
    {#if loading}
      <div class="grid grid-cols-[88px_1fr] gap-3">
        <div class="h-[88px] w-[88px] animate-pulse rounded-lg bg-slate-100"></div>
        <div class="space-y-2">
          <div class="h-4 w-28 animate-pulse rounded bg-slate-100"></div>
          <div class="h-4 w-36 animate-pulse rounded bg-slate-100"></div>
          <div class="h-4 w-24 animate-pulse rounded bg-slate-100"></div>
        </div>
      </div>
    {:else if unavailable}
      <div class="flex h-full flex-col items-center justify-center rounded-md bg-slate-50 text-center">
        <DashboardIcon name="bot" class="size-7 text-slate-400" />
        <p class="mt-2 text-sm font-medium text-slate-700">{$t('widgets.inspection_unavailable')}</p>
      </div>
    {:else if !present}
      <div class="flex h-full flex-col items-center justify-center rounded-md bg-slate-50 text-center">
        <DashboardIcon name="bot" class="size-7 text-slate-400" />
        <p class="mt-2 text-sm font-medium text-slate-700">{$t('widgets.no_inspections')}</p>
        <p class="max-w-56 text-xs leading-5 text-slate-500">AI inspection results will appear here once a QC job completes.</p>
      </div>
    {:else}
      <div class="grid grid-cols-[88px_1fr] gap-3">
        <div class="h-[88px] w-[88px] overflow-hidden rounded-lg bg-slate-100">
          {#if imageUrl}
            <img src={imageUrl} alt="QC inspection preview for {inspection?.lotNumber}" class="size-full object-cover" />
          {:else}
            <div class="flex size-full items-center justify-center text-slate-400">
              <DashboardIcon name="bot" class="size-8" />
            </div>
          {/if}
        </div>
        <div class="min-w-0 text-xs">
          <p class="text-slate-500">{$t('widgets.lot')}</p>
          <p class="truncate font-semibold text-slate-950">{inspection?.lotNumber}</p>
          <p class="mt-2 text-slate-500">{$t('widgets.material')}</p>
          <p class="truncate font-semibold text-slate-950">{inspection?.materialName}</p>
          <div class="mt-2 flex items-center gap-2">
            <span class="rounded-md bg-slate-100 px-2 py-0.5 font-semibold text-slate-700">{pct(inspection?.confidence)}</span>
            <span class="rounded-md px-2 py-0.5 font-semibold {recommendationClass(inspection?.recommendation)}">
              {recommendationLabel(inspection?.recommendation)}
            </span>
          </div>
        </div>
      </div>

      <div class="mt-4 min-h-0 space-y-2 overflow-hidden text-xs">
        {#if inspection?.findings?.length}
          {#each inspection.findings.slice(0, 4) as finding}
            <div class="flex items-center justify-between gap-3">
              <span class="flex min-w-0 items-center gap-2">
                <span class="size-2 shrink-0 rounded-full {finding.isAnomaly ? 'bg-red-500' : 'bg-emerald-500'}"></span>
                <span class="truncate text-slate-700">{finding.mappedFinding || 'Finding'}</span>
              </span>
              <span class="shrink-0 rounded-md bg-slate-100 px-2 py-0.5 text-slate-600">{finding.isAnomaly ? 'Anomaly' : 'Normal'}</span>
            </div>
          {/each}
        {/if}
        <p class="pt-1 text-[11px] text-slate-400">Inspected at {timestampLabel(inspection?.createdAtUnix)}</p>
      </div>
    {/if}
  </div>
</section>
