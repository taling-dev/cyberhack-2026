<script lang="ts">
  import DashboardIcon from './DashboardIcon.svelte';

  type LotLike = {
    id: string;
    lotNumber: string;
    materialName: string;
  };

  type QCResultLike = {
    recommendation?: number;
    confidence?: number;
    findings?: Array<{
      className?: string;
      mappedFinding?: string;
      confidence?: number;
      isAnomaly?: boolean;
    }>;
    createdAt?: { seconds?: bigint | number | string };
  };

  let {
    lot = null,
    result = null,
    imageUrl = '',
    loading = false,
    unavailable = false,
  } = $props<{
    lot?: LotLike | null;
    result?: QCResultLike | null;
    imageUrl?: string;
    loading?: boolean;
    unavailable?: boolean;
  }>();

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

  function timestampLabel(value?: { seconds?: bigint | number | string }) {
    if (!value?.seconds) return '-';
    return new Date(Number(value.seconds) * 1000).toLocaleString('en-US', {
      month: '2-digit',
      day: '2-digit',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  }
</script>

<section class="flex h-full min-h-0 flex-col rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
  <div class="flex items-center justify-between">
    <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">Latest AI Inspection</h2>
    {#if lot}
      <a href="/qc/{lot.id}" class="text-blue-600" aria-label="Open latest inspection">
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
    {:else if unavailable || !lot}
      <div class="flex h-full flex-col items-center justify-center rounded-md bg-slate-50 text-center">
        <DashboardIcon name="bot" class="size-7 text-slate-400" />
        <p class="mt-2 text-sm font-medium text-slate-700">No active inspection detail</p>
        <p class="max-w-56 text-xs leading-5 text-slate-500">Latest global inspection data is not exposed by the current API.</p>
      </div>
    {:else}
      <div class="grid grid-cols-[88px_1fr] gap-3">
        <div class="h-[88px] w-[88px] overflow-hidden rounded-lg bg-slate-100">
          {#if imageUrl}
            <img src={imageUrl} alt="QC inspection preview for {lot.lotNumber}" class="size-full object-cover" />
          {:else}
            <div class="flex size-full items-center justify-center text-slate-400">
              <DashboardIcon name="bot" class="size-8" />
            </div>
          {/if}
        </div>
        <div class="min-w-0 text-xs">
          <p class="text-slate-500">Lot ID</p>
          <p class="truncate font-semibold text-slate-950">{lot.lotNumber}</p>
          <p class="mt-2 text-slate-500">Material</p>
          <p class="truncate font-semibold text-slate-950">{lot.materialName}</p>
          <div class="mt-2 flex items-center gap-2">
            <span class="rounded-md bg-emerald-100 px-2 py-0.5 font-semibold text-emerald-700">{pct(result?.confidence)}</span>
            <span class="rounded-md px-2 py-0.5 font-semibold {recommendationClass(result?.recommendation)}">
              {recommendationLabel(result?.recommendation)}
            </span>
          </div>
        </div>
      </div>

      <div class="mt-4 min-h-0 space-y-2 overflow-hidden text-xs">
        {#if result?.findings?.length}
          {#each result.findings.slice(0, 4) as finding}
            <div class="flex items-center justify-between gap-3">
              <span class="flex min-w-0 items-center gap-2">
                <span class="size-2 shrink-0 rounded-full {finding.isAnomaly ? 'bg-red-500' : 'bg-emerald-500'}"></span>
                <span class="truncate text-slate-700">{finding.mappedFinding || finding.className || 'Finding'}</span>
              </span>
              <span class="shrink-0 rounded-md bg-slate-100 px-2 py-0.5 text-slate-600">{finding.isAnomaly ? 'Anomaly' : 'Normal'}</span>
            </div>
          {/each}
        {:else}
          <div class="rounded-md bg-slate-50 px-3 py-2 text-slate-500">No finding checklist returned yet.</div>
        {/if}
        <p class="pt-1 text-[11px] text-slate-400">Inspected at {timestampLabel(result?.createdAt)}</p>
      </div>
    {/if}
  </div>
</section>
