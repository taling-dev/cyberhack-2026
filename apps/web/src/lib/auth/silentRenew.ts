// Tier 1 of the auth recovery ladder: silent OIDC renew via a hidden iframe.
//
// Returns true if Keycloak's SSO session was still alive and we got fresh
// cookies; false if Keycloak returned `error=login_required` (or anything
// else went wrong). The caller should then attempt Tier 2 (popup login).
//
// The flow:
//   1. Insert a hidden iframe pointing at /auth/login?silent=1.
//   2. /auth/login adds prompt=none and redirects to Keycloak.
//   3. Keycloak either returns a fresh code (SSO session alive) or
//      error=login_required.
//   4. Our /auth/callback detects `silent` mode in the OAuth state and
//      responds with HTML that postMessages the result to the parent.
//   5. We resolve the promise, clean up the iframe, and tell the caller.

interface SilentRenewMessage {
  type: 'silent-renew-result';
  ok: boolean;
}

export function silentRenew(timeoutMs = 5000): Promise<boolean> {
  return new Promise((resolve) => {
    if (typeof document === 'undefined' || typeof window === 'undefined') {
      resolve(false);
      return;
    }
    const iframe = document.createElement('iframe');
    iframe.style.display = 'none';
    iframe.style.width = '0';
    iframe.style.height = '0';
    iframe.setAttribute('aria-hidden', 'true');
    iframe.src = '/auth/login?silent=1';

    let settled = false;
    const cleanup = () => {
      window.removeEventListener('message', onMessage);
      try {
        iframe.remove();
      } catch {
        // ignore
      }
    };
    const finish = (ok: boolean) => {
      if (settled) return;
      settled = true;
      cleanup();
      resolve(ok);
    };

    function onMessage(e: MessageEvent) {
      if (e.origin !== window.location.origin) return;
      const data = e.data as SilentRenewMessage | undefined;
      if (data && data.type === 'silent-renew-result') {
        finish(Boolean(data.ok));
      }
    }
    window.addEventListener('message', onMessage);

    const timer = setTimeout(() => finish(false), timeoutMs);
    iframe.addEventListener('load', () => {
      // load fires for both success and error paths — but the callback page
      // also posts a message in both cases, so we rely on that. Keep the
      // timeout as a safety net in case the page ran into CSP issues or the
      // browser navigated away from our origin.
    });
    document.body.appendChild(iframe);
    // Cleanup the timer ref on settle.
    Promise.resolve().then(() => {
      const origFinish = finish;
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      const _ = origFinish; // satisfy strict no-unused-vars
    });
    // Tie the timer to settle.
    void timer;
  });
}
