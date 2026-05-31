<script lang="ts">
  import DashboardIcon from './DashboardIcon.svelte';

  type Day = { date: string; passCount: number; reviewCount: number; failCount: number };

  let { days = [], loading = false } = $props<{ days?: Day[]; loading?: boolean }>();

  const totals = $derived(days.reduce(
    (a: { pass: number; review: number; fail: number }, d: Day) => ({
      pass: a.pass + d.passCount,
      review: a.review + d.reviewCount,
      fail: a.fail + d.failCount,
    }),
    { pass: 0, review: 0, fail: 0 }
  ));
  const grandTotal = $derived(totals.pass + totals.review + totals.fail);
  const maxDay = $derived(Math.max(1, ...days.map((d: Day) => d.passCount + d.reviewCount + d.failCount)));

  function dow(date: string) {
    return new Date(date + 'T00:00:00').toLocaleDateString('en-US', { weekday: 'short' });
  }
</script>

<section class="flex h-full min-h-0 flex-col rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
  <div class="flex items-center justify-between">
    <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">QC Trend (7 Days)</h2>
    <DashboardIcon name="chart" class="size-4 text-slate-400" />
  </div>

  <div class="mt-3 min-h-0 flex-1">
    {#if loading}
      <div class="h-full w-full animate-pulse rounded-md bg-slate-100"></div>
    {:else if grandTotal === 0}
      <div class="flex h-full flex-col items-center justify-center rounded-md border border-slate-100 bg-slate-50 px-6 text-center">
        <p class="text-sm font-semibold text-slate-700">No QC activity in the last 7 days</p>
      </div>
    {:else}
      <div class="flex h-full items-end justify-between gap-2">
        {#each days as d}
          {@const dayTotal = d.passCount + d.reviewCount + d.failCount}
          <div class="flex h-full flex-1 flex-col items-center justify-end gap-1" title="{d.date}: {d.passCount} pass · {d.reviewCount} review · {d.failCount} fail">
            <div class="flex w-full max-w-7 flex-col-reverse overflow-hidden rounded" style="height: {(dayTotal / maxDay) * 100}%">
              {#if d.passCount}<div class="bg-emerald-500" style="flex: {d.passCount}"></div>{/if}
              {#if d.reviewCount}<div class="bg-orange-500" style="flex: {d.reviewCount}"></div>{/if}
              {#if d.failCount}<div class="bg-red-500" style="flex: {d.failCount}"></div>{/if}
            </div>
            <span class="text-[10px] text-slate-400">{dow(d.date)}</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <div class="mt-3 grid grid-cols-4 gap-2 border-t border-slate-100 pt-2 text-xs">
    <div><p class="text-slate-500">Total</p><p class="mt-0.5 font-bold text-slate-950">{loading ? '-' : grandTotal}</p></div>
    <div><p class="text-emerald-600">Pass</p><p class="mt-0.5 font-bold text-slate-950">{loading ? '-' : totals.pass}</p></div>
    <div><p class="text-orange-600">Review</p><p class="mt-0.5 font-bold text-slate-950">{loading ? '-' : totals.review}</p></div>
    <div><p class="text-red-600">Fail</p><p class="mt-0.5 font-bold text-slate-950">{loading ? '-' : totals.fail}</p></div>
  </div>
</section>
