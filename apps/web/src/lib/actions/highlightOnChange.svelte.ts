// Svelte action: `use:highlightOnChange={resourceId}`.
//
// Watches the realtime store's recentEvents and applies a CSS class for
// 1.5s whenever an event with a matching resource_id arrives. Honors
// prefers-reduced-motion (skips the fade transition; just briefly
// highlights and removes).
//
// Usage:
//   <tr use:highlightOnChange={lot.id}>...</tr>
//
// The action listens to a custom DOM event 'simaops:highlight' that the
// realtime store dispatches per event. This avoids each row having to
// import the realtime store directly.

const HIGHLIGHT_CLASS = 'simaops-highlight';
const HIGHLIGHT_DURATION_MS = 1500;

export function highlightOnChange(node: HTMLElement, resourceId: string) {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let currentId = resourceId;

  function trigger() {
    node.classList.add(HIGHLIGHT_CLASS);
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      node.classList.remove(HIGHLIGHT_CLASS);
    }, HIGHLIGHT_DURATION_MS);
  }

  function onHighlight(ev: Event) {
    const e = ev as CustomEvent<{ resourceId: string; lotId?: string }>;
    if (!currentId) return;
    if (e.detail.resourceId === currentId || e.detail.lotId === currentId) {
      trigger();
    }
  }

  window.addEventListener('simaops:highlight', onHighlight as EventListener);

  return {
    update(next: string) {
      currentId = next;
    },
    destroy() {
      window.removeEventListener('simaops:highlight', onHighlight as EventListener);
      if (timer) clearTimeout(timer);
    },
  };
}
