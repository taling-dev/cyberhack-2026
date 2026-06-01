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
    href = undefined,
    emphasis = false,
  } = $props<{
    title: string;
    value: string;
    icon: DashboardIconName;
    tone?: Tone;
    loading?: boolean;
    href?: string;
    emphasis?: boolean;
  }>();

  function classesFor(value: Tone | undefined) {
    return toneClasses[value ?? 'slate'] ?? toneClasses.slate;
  }

  const toneClass = $derived(classesFor(tone));
</script>

<svelte:element
  this={href ? 'a' : 'section'}
  {href}
  class="block rounded-lg border border-slate-200 bg-white p-3 shadow-sm transition-colors {href ? 'hover:border-slate-300 hover:bg-slate-50' : ''}"
>
  <p class="truncate text-center text-[12px] font-medium text-slate-700">{title}</p>
  <div class="mt-3 flex items-center justify-center gap-3">
    <span class="flex shrink-0 items-center justify-center rounded-md {emphasis ? 'size-11' : 'size-10'} {toneClass.badge}">
      <DashboardIcon name={icon} class={emphasis ? 'size-7' : 'size-6'} />
    </span>
    {#if loading}
      <span class="h-8 w-20 animate-pulse rounded bg-slate-100"></span>
    {:else}
      <span class="truncate font-bold tracking-normal {emphasis ? 'text-3xl' : 'text-2xl'} {toneClass.value}">{value}</span>
    {/if}
  </div>
</svelte:element>
