<script lang="ts">
  import DashboardIcon, { type DashboardIconName } from './DashboardIcon.svelte';

  type Tone = 'blue' | 'emerald' | 'green' | 'orange' | 'purple' | 'red' | 'slate';

  const toneClasses: Record<Tone, { badge: string; value: string }> = {
    blue: { badge: 'bg-blue-100 text-blue-700', value: 'text-slate-950' },
    emerald: { badge: 'bg-emerald-100 text-emerald-700', value: 'text-slate-950' },
    green: { badge: 'bg-green-100 text-green-700', value: 'text-slate-950' },
    orange: { badge: 'bg-orange-100 text-orange-700', value: 'text-slate-950' },
    purple: { badge: 'bg-purple-100 text-purple-700', value: 'text-slate-950' },
    red: { badge: 'bg-red-100 text-red-700', value: 'text-slate-950' },
    slate: { badge: 'bg-slate-100 text-slate-700', value: 'text-slate-950' },
  };

  let {
    title,
    value,
    icon,
    tone = 'slate',
    loading = false,
  } = $props<{
    title: string;
    value: string;
    icon: DashboardIconName;
    tone?: Tone;
    loading?: boolean;
  }>();

  function classesFor(value: Tone | undefined) {
    return toneClasses[value ?? 'slate'] ?? toneClasses.slate;
  }

  const toneClass = $derived(classesFor(tone));
</script>

<section class="rounded-lg border border-slate-200 bg-white p-3 shadow-sm">
  <p class="truncate text-center text-[12px] font-medium text-slate-700">{title}</p>
  <div class="mt-3 flex items-center justify-center gap-3">
    <span class="flex size-10 shrink-0 items-center justify-center rounded-md {toneClass.badge}">
      <DashboardIcon name={icon} class="size-6" />
    </span>
    {#if loading}
      <span class="h-8 w-20 animate-pulse rounded bg-slate-100"></span>
    {:else}
      <span class="truncate text-2xl font-bold tracking-normal {toneClass.value}">{value}</span>
    {/if}
  </div>
</section>
