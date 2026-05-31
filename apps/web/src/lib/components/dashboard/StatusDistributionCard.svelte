<script lang="ts">
  type StatusCountLike = {
    status: string;
    count: number;
  };

  const circumference = 2 * Math.PI * 42;

  const statusMeta: Record<string, { label: string; color: string }> = {
    DRAFT: { label: 'Draft', color: '#94a3b8' },
    PENDING_QC: { label: 'Pending QC', color: '#facc15' },
    AI_PROCESSING: { label: 'AI Processing', color: '#3b82f6' },
    QC_REVIEW: { label: 'QC Review', color: '#f97316' },
    QC_APPROVED: { label: 'QC Approved', color: '#10b981' },
    QC_REJECTED: { label: 'Rejected', color: '#ef4444' },
    WAREHOUSE_ASSIGNED: { label: 'Warehouse Assigned', color: '#8b5cf6' },
    READY_FOR_PRODUCTION: { label: 'Ready Production', color: '#16a34a' },
    BLOCKED: { label: 'Blocked', color: '#b91c1c' },
  };

  let {
    statuses = [],
    total = 0,
    loading = false,
  } = $props<{
    statuses?: StatusCountLike[];
    total?: number;
    loading?: boolean;
  }>();

  function totalFromStatuses() {
    return total || (statuses as StatusCountLike[]).reduce((sum: number, status: StatusCountLike) => sum + status.count, 0);
  }

  function labelFor(status: string) {
    return statusMeta[status]?.label ?? status.replace(/_/g, ' ').toLowerCase();
  }

  function colorFor(status: string) {
    return statusMeta[status]?.color ?? '#64748b';
  }

  function segments() {
    const safeTotal = totalFromStatuses();
    let offset = 0;
    return (statuses as StatusCountLike[])
      .filter((status: StatusCountLike) => status.count > 0 && safeTotal > 0)
      .map((status: StatusCountLike) => {
        const length = (status.count / safeTotal) * circumference;
        const segment = {
          ...status,
          label: labelFor(status.status),
          color: colorFor(status.status),
          length,
          offset,
          pct: Math.round((status.count / safeTotal) * 1000) / 10,
        };
        offset += length;
        return segment;
      });
  }
</script>

<section class="flex h-full min-h-0 flex-col rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
  <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">Lot Status Distribution</h2>

  <div class="mt-3 grid min-h-0 flex-1 grid-cols-[150px_1fr] items-center gap-4">
    <div class="relative mx-auto size-32">
      {#if loading}
        <div class="size-32 animate-pulse rounded-full bg-slate-100"></div>
      {:else}
        <svg class="size-32" viewBox="0 0 128 128" aria-label="Lot status distribution">
          <circle cx="64" cy="64" r="42" fill="none" stroke="#e5e7eb" stroke-width="18" />
          {#each segments() as segment}
            <circle
              cx="64"
              cy="64"
              r="42"
              fill="none"
              stroke={segment.color}
              stroke-width="18"
              stroke-linecap="butt"
              stroke-dasharray="{segment.length} {circumference - segment.length}"
              stroke-dashoffset={-segment.offset}
              transform="rotate(-90 64 64)"
            />
          {/each}
        </svg>
        <div class="absolute inset-0 flex flex-col items-center justify-center">
          <span class="text-xl font-bold text-slate-950">{totalFromStatuses().toLocaleString('en-US')}</span>
          <span class="text-[11px] text-slate-500">Total</span>
        </div>
      {/if}
    </div>

    <div class="min-h-0 space-y-2 overflow-hidden">
      {#if loading}
        {#each [1, 2, 3, 4] as _}
          <div class="h-4 animate-pulse rounded bg-slate-100"></div>
        {/each}
      {:else if totalFromStatuses() === 0}
        <p class="text-sm text-slate-500">No lot status data available.</p>
      {:else}
        {#each segments().slice(0, 6) as segment}
          <div class="flex items-start gap-2 text-xs">
            <span class="mt-1 size-2 shrink-0 rounded-full" style="background-color: {segment.color};"></span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center justify-between gap-2">
                <span class="truncate text-slate-700">{segment.label}</span>
                <span class="shrink-0 font-medium text-slate-950">{segment.count.toLocaleString('en-US')}</span>
              </div>
              <p class="text-[11px] text-slate-500">{segment.pct}%</p>
            </div>
          </div>
        {/each}
      {/if}
    </div>
  </div>
</section>
