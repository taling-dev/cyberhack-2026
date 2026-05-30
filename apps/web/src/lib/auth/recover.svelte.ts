// Shared auth-recovery ladder, used by BOTH the SSE realtime store and the
// Connect RPC transport interceptor so token expiry is handled uniformly
// regardless of which layer first observes the 401.
//
// Tiers:
//   0. force-refresh the access cookie via /auth/heartbeat?force=true
//      (only worth trying when the failure looks like plain expiry).
//   1. silent OIDC renew via hidden iframe (SSO session still alive).
//   2. give up → flag session-expired so the UI shows the re-login modal.
//
// Single-flight: concurrent 401s (e.g. three dashboard queries firing at
// once) collapse into ONE recovery attempt; all callers await the same
// promise. This prevents a refresh storm and duplicate silent-renew iframes.

import { browser } from '$app/environment';
import { silentRenew } from '$lib/auth/silentRenew';

// Reactive flag the layout binds the SessionExpiredModal to. Set true when
// recovery is exhausted; reset to false on a successful recovery or sign-in.
export const authState = $state<{ sessionExpired: boolean }>({ sessionExpired: false });

let inflight: Promise<boolean> | null = null;

/**
 * recoverAuth runs the recovery ladder. Returns true if the session was
 * restored (caller should retry), false if the user must re-authenticate
 * (authState.sessionExpired is set true).
 *
 * @param canForceRefresh when true, attempt Tier-0 force-refresh first.
 *   Pass false when the caller already knows a server-side refresh just
 *   failed (e.g. a heartbeat 401), so we skip straight to silent renew.
 */
export function recoverAuth(canForceRefresh = true): Promise<boolean> {
  if (!browser) return Promise.resolve(false);
  if (inflight) return inflight;

  inflight = (async () => {
    try {
      if (canForceRefresh) {
        try {
          const res = await fetch('/auth/heartbeat?force=true', { credentials: 'same-origin' });
          if (res.ok) {
            authState.sessionExpired = false;
            return true;
          }
        } catch {
          // fall through to silent renew
        }
      }
      const ok = await silentRenew();
      if (ok) {
        authState.sessionExpired = false;
        return true;
      }
      authState.sessionExpired = true;
      return false;
    } finally {
      inflight = null;
    }
  })();

  return inflight;
}

/** Clear the session-expired flag after a successful interactive re-login. */
export function markRecovered(): void {
  authState.sessionExpired = false;
}
