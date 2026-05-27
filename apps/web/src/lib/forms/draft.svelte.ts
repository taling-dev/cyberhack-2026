// Form draft persistence — survives session expiry → re-auth → reload.
//
// useDraft(formKey, initial) returns { state, clear } where:
//   - state is a Svelte 5 $state object pre-populated from localStorage if a
//     draft exists, otherwise from `initial`.
//   - state changes are auto-saved to localStorage with a 500ms debounce.
//   - clear() removes the draft (call after successful submit).
//
// Storage key is namespaced per user-sub so multi-user laptops don't clash.
//
// Drafts older than 7 days are swept on app load (sweepOldDrafts() in
// +layout.svelte's onMount). The TTL is generous because we'd rather keep a
// draft and let the user delete it than surprise them by losing their work.

import { browser } from '$app/environment';

const PREFIX = 'simaops:draft:';
const SAVE_DEBOUNCE_MS = 500;
const SWEEP_TTL_MS = 7 * 24 * 60 * 60 * 1000; // 7 days

interface StoredDraft<T> {
  v: 1;
  data: T;
  ts: number;
}

function key(userSub: string | undefined, formKey: string): string {
  return `${PREFIX}${userSub ?? 'anon'}:${formKey}`;
}

/**
 * useDraft returns a $state-rune object plus a clear() callback.
 *
 * The caller is expected to bind the returned object's properties to form
 * inputs (e.g. `<input bind:value={draft.state.supplierName}>`).
 *
 * Restores from localStorage if a draft exists for this user+formKey;
 * otherwise initializes from the `initial` argument.
 */
export function useDraft<T extends Record<string, any>>(
  userSub: string | undefined,
  formKey: string,
  initial: T,
): { state: T; clear: () => void } {
  const storageKey = key(userSub, formKey);

  let initialValue: T = { ...initial };
  if (browser) {
    try {
      const raw = localStorage.getItem(storageKey);
      if (raw) {
        const parsed = JSON.parse(raw) as StoredDraft<T>;
        if (parsed?.v === 1 && parsed.data) {
          initialValue = { ...initial, ...parsed.data };
        }
      }
    } catch {
      // ignore corrupt drafts
    }
  }

  const state = $state<T>(initialValue);

  let saveTimer: ReturnType<typeof setTimeout> | null = null;
  if (browser) {
    $effect(() => {
      // Touch every property of state so the effect is dependency-tracked.
      const snapshot = $state.snapshot(state);
      if (saveTimer) clearTimeout(saveTimer);
      saveTimer = setTimeout(() => {
        try {
          const stored: StoredDraft<T> = { v: 1, data: snapshot as T, ts: Date.now() };
          localStorage.setItem(storageKey, JSON.stringify(stored));
        } catch {
          // localStorage full / disabled — accept silent failure
        }
      }, SAVE_DEBOUNCE_MS);

      return () => {
        if (saveTimer) clearTimeout(saveTimer);
      };
    });
  }

  function clear() {
    if (!browser) return;
    try {
      localStorage.removeItem(storageKey);
    } catch {
      // ignore
    }
  }

  return { state, clear };
}

/**
 * sweepOldDrafts removes any simaops:draft:* entries older than 7 days.
 * Call once on app load (typically from +layout.svelte's onMount).
 */
export function sweepOldDrafts() {
  if (!browser) return;
  try {
    const now = Date.now();
    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i);
      if (!k || !k.startsWith(PREFIX)) continue;
      try {
        const raw = localStorage.getItem(k);
        if (!raw) continue;
        const parsed = JSON.parse(raw) as StoredDraft<unknown>;
        if (now - (parsed?.ts ?? 0) > SWEEP_TTL_MS) {
          localStorage.removeItem(k);
          i--; // index shift after removal
        }
      } catch {
        // corrupt entry — remove it
        localStorage.removeItem(k);
        i--;
      }
    }
  } catch {
    // localStorage unavailable
  }
}
