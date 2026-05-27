<script lang="ts" module>
  // Toast stack lives in module scope so any component (or the realtime
  // store) can call `pushToast(...)` without prop-drilling. Svelte 5 $state
  // works at module scope too.

  export interface ToastSpec {
    id: string;
    title: string;
    body?: string;
    href?: string;
    variant?: 'info' | 'success' | 'warning' | 'error';
    /** Auto-dismiss after this many ms (0 = sticky) */
    timeoutMs?: number;
  }

  const MAX_STACK = 5;
  let toasts = $state<ToastSpec[]>([]);

  function genId() {
    if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
    return Math.random().toString(36).slice(2);
  }

  export function pushToast(t: Omit<ToastSpec, 'id'> & { id?: string }) {
    const id = t.id ?? genId();
    const spec: ToastSpec = { id, timeoutMs: 5000, variant: 'info', ...t };
    // Drop oldest if stack is full.
    const next = [...toasts, spec].slice(-MAX_STACK);
    toasts = next;
    if (spec.timeoutMs && spec.timeoutMs > 0) {
      setTimeout(() => dismissToast(id), spec.timeoutMs);
    }
  }

  export function dismissToast(id: string) {
    toasts = toasts.filter((t) => t.id !== id);
  }

  export function getToasts() {
    return toasts;
  }
</script>

<script lang="ts">
  import { goto } from '$app/navigation';

  function variantClass(v: ToastSpec['variant']) {
    switch (v) {
      case 'success':
        return 'border-green-300 bg-green-50';
      case 'warning':
        return 'border-amber-300 bg-amber-50';
      case 'error':
        return 'border-red-300 bg-red-50';
      default:
        return 'border-blue-300 bg-blue-50';
    }
  }

  function variantIcon(v: ToastSpec['variant']) {
    switch (v) {
      case 'success':
        return '✓';
      case 'warning':
        return '⚠';
      case 'error':
        return '✕';
      default:
        return 'ℹ';
    }
  }

  async function handleClick(t: ToastSpec) {
    if (t.href) {
      await goto(t.href);
      dismissToast(t.id);
    }
  }
</script>

<div
  class="fixed bottom-4 right-4 z-[55] flex flex-col gap-2 max-w-sm w-[calc(100vw-2rem)] sm:w-96"
  role="region"
  aria-live="polite"
  aria-label="Notifications"
>
  {#each toasts as toast (toast.id)}
    <div
      class="relative flex items-start gap-3 border rounded-md px-4 py-3 shadow-lg {variantClass(toast.variant)}"
    >
      <span aria-hidden="true" class="text-lg">{variantIcon(toast.variant)}</span>
      <div class="flex-1 min-w-0">
        {#if toast.href}
          <button
            type="button"
            class="text-left w-full"
            onclick={() => handleClick(toast)}
          >
            <div class="text-sm font-semibold">{toast.title}</div>
            {#if toast.body}
              <div class="text-sm text-gray-700 mt-0.5">{toast.body}</div>
            {/if}
          </button>
        {:else}
          <div class="text-sm font-semibold">{toast.title}</div>
          {#if toast.body}
            <div class="text-sm text-gray-700 mt-0.5">{toast.body}</div>
          {/if}
        {/if}
      </div>
      <button
        type="button"
        class="text-gray-500 hover:text-gray-800 leading-none"
        aria-label="Dismiss"
        onclick={() => dismissToast(toast.id)}
      >
        ×
      </button>
    </div>
  {/each}
</div>
