<script lang="ts">
  export type DashboardIconName =
    | 'activity'
    | 'arrow-right'
    | 'bell'
    | 'bot'
    | 'calendar'
    | 'chart'
    | 'check-circle'
    | 'clock'
    | 'cube'
    | 'database'
    | 'globe'
    | 'leaf'
    | 'percent'
    | 'shield'
    | 'sparkles'
    | 'warehouse';

  let {
    name,
    class: className = 'size-5',
  } = $props<{
    name: DashboardIconName;
    class?: string;
  }>();
</script>

<svg
  class={className}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="2"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {#if name === 'activity'}
    <path d="M3 12h4l2.5-7 5 14L17 12h4" />
  {:else if name === 'arrow-right'}
    <path d="M5 12h14" />
    <path d="m13 6 6 6-6 6" />
  {:else if name === 'bell'}
    <path d="M18 8a6 6 0 1 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9" />
    <path d="M10 21h4" />
  {:else if name === 'bot'}
    <rect x="5" y="8" width="14" height="10" rx="2" />
    <path d="M12 4v4" />
    <path d="M9 13h.01" />
    <path d="M15 13h.01" />
    <path d="M9 18v2" />
    <path d="M15 18v2" />
  {:else if name === 'calendar'}
    <rect x="3" y="5" width="18" height="16" rx="2" />
    <path d="M16 3v4" />
    <path d="M8 3v4" />
    <path d="M3 11h18" />
  {:else if name === 'chart'}
    <path d="M4 19V5" />
    <path d="M4 19h16" />
    <path d="M8 15v-4" />
    <path d="M12 15V8" />
    <path d="M16 15v-6" />
  {:else if name === 'check-circle'}
    <circle cx="12" cy="12" r="9" />
    <path d="m8 12 2.5 2.5L16 9" />
  {:else if name === 'clock'}
    <circle cx="12" cy="12" r="9" />
    <path d="M12 7v5l3 2" />
  {:else if name === 'cube'}
    <path d="m12 3 8 4.5v9L12 21l-8-4.5v-9L12 3Z" />
    <path d="m4 7.5 8 4.5 8-4.5" />
    <path d="M12 12v9" />
  {:else if name === 'database'}
    <ellipse cx="12" cy="5" rx="7" ry="3" />
    <path d="M5 5v6c0 1.7 3.1 3 7 3s7-1.3 7-3V5" />
    <path d="M5 11v6c0 1.7 3.1 3 7 3s7-1.3 7-3v-6" />
  {:else if name === 'globe'}
    <circle cx="12" cy="12" r="9" />
    <path d="M3 12h18" />
    <path d="M12 3c2.3 2.5 3.5 5.5 3.5 9S14.3 18.5 12 21" />
    <path d="M12 3c-2.3 2.5-3.5 5.5-3.5 9s1.2 6.5 3.5 9" />
  {:else if name === 'leaf'}
    <path d="M5 19c6.5.2 13-4.5 14-14-9.5 1-14.2 7.5-14 14Z" />
    <path d="M5 19c2.5-5 6-8.5 11-11" />
  {:else if name === 'percent'}
    <path d="m19 5-14 14" />
    <circle cx="7.5" cy="7.5" r="2" />
    <circle cx="16.5" cy="16.5" r="2" />
  {:else if name === 'shield'}
    <path d="M12 3 20 6v6c0 5-3.3 8-8 9-4.7-1-8-4-8-9V6l8-3Z" />
    <path d="m9 12 2 2 4-5" />
  {:else if name === 'sparkles'}
    <path d="m12 3 1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5L12 3Z" />
    <path d="m5 14 .8 2.2L8 17l-2.2.8L5 20l-.8-2.2L2 17l2.2-.8L5 14Z" />
    <path d="m19 14 .8 2.2L22 17l-2.2.8L19 20l-.8-2.2L16 17l2.2-.8L19 14Z" />
  {:else if name === 'warehouse'}
    <path d="M3 10.5 12 5l9 5.5" />
    <path d="M5 10v9h14v-9" />
    <path d="M9 19v-6h6v6" />
    <path d="M9 13h6" />
  {/if}
</svg>
