// apps/web/src/lib/actions/focusTrap.svelte.ts
//
// `use:focusTrap` — a single Svelte action that implements WCAG 2.1 AA-compliant
// modal focus management:
//
//   * Marks the rest of the document `inert` while the modal is open so screen
//     readers, mouse, and keyboard cannot reach background content.
//   * Moves focus to the first focusable element inside the modal on open.
//   * Restores focus to the element that triggered the modal when it closes.
//   * Cycles Tab / Shift-Tab strictly within the modal — Tab from the last
//     focusable wraps to the first; Shift-Tab from the first wraps to the last.
//
// Usage:
//
//   <div role="dialog" aria-modal="true" use:focusTrap>
//     ...
//   </div>
//
// Mount the action only when the modal is actually rendered (i.e., wrap the
// dialog in {#if open}); the action's lifecycle handles install / restore.

type FocusTrapTarget = HTMLElement | null;

const FOCUSABLE_SELECTOR = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',');

function focusable(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
    (el) => !el.hasAttribute('aria-hidden') && el.offsetParent !== null,
  );
}

export function focusTrap(node: HTMLElement) {
  // Remember the trigger so we can restore focus when the modal closes.
  const previouslyFocused: FocusTrapTarget = (document.activeElement as HTMLElement) ?? null;

  // Mark every direct sibling of the modal's effective parent `inert` so
  // background controls can't be tabbed/clicked. We restore on cleanup.
  const inertSiblings: HTMLElement[] = [];
  const root = document.body;
  for (const child of Array.from(root.children)) {
    if (child instanceof HTMLElement && child !== node && !child.contains(node)) {
      if (!child.hasAttribute('inert')) {
        child.setAttribute('inert', '');
        inertSiblings.push(child);
      }
    }
  }

  // Move initial focus into the modal.
  const initial = focusable(node);
  if (initial.length > 0) {
    initial[0].focus();
  } else {
    // Fall back to the dialog container itself so screen readers announce it.
    node.tabIndex = -1;
    node.focus();
  }

  function onKeyDown(e: KeyboardEvent) {
    if (e.key !== 'Tab') return;
    const tabbables = focusable(node);
    if (tabbables.length === 0) {
      e.preventDefault();
      return;
    }
    const first = tabbables[0];
    const last = tabbables[tabbables.length - 1];
    const active = document.activeElement as HTMLElement;
    if (e.shiftKey && active === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && active === last) {
      e.preventDefault();
      first.focus();
    }
  }

  node.addEventListener('keydown', onKeyDown);

  return {
    destroy() {
      node.removeEventListener('keydown', onKeyDown);
      for (const sib of inertSiblings) sib.removeAttribute('inert');
      // Best-effort focus restoration; the trigger may have been removed
      // (e.g., navigation away). Failures are harmless.
      try {
        previouslyFocused?.focus?.();
      } catch {
        // ignore
      }
    },
  };
}
