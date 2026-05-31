<script lang="ts">
  import DashboardIcon, { type DashboardIconName } from './DashboardIcon.svelte';

  const features: Array<{ icon: DashboardIconName; title: string; body: string }> = [
    { icon: 'sparkles', title: 'AI Powered', body: 'Computer vision for automated QC' },
    { icon: 'clock', title: 'Real-time Updates', body: 'Updated through event streaming' },
    { icon: 'warehouse', title: 'Enterprise Ready', body: 'Traceable operations workflow' },
    { icon: 'shield', title: 'Audit Trail', body: 'All activities recorded' },
    { icon: 'database', title: 'Data Driven', body: 'Accurate operational decisions' },
  ];
</script>

<section class="grid grid-cols-5 divide-x divide-emerald-100 bg-emerald-50/70 px-5 py-2.5">
  {#each features as feature}
    <div class="flex items-center gap-3 px-4 first:pl-0 last:pr-0">
      <DashboardIcon name={feature.icon} class="size-7 shrink-0 text-emerald-600" />
      <div class="min-w-0">
        <p class="truncate text-sm font-bold text-slate-950">{feature.title}</p>
        <p class="truncate text-[11px] text-slate-600">{feature.body}</p>
      </div>
    </div>
  {/each}
</section>
