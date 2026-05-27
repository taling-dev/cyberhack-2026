// Tier 2 of the auth recovery ladder: interactive login in a popup window.
//
// Resolves to true if the user successfully signed in (the popup posted
// a `login-complete` message back to the opener and closed itself).
// Resolves to false if:
//   - the popup was blocked by the browser
//   - the user closed the popup without completing sign-in
//   - the timeout was hit (default 5 minutes)
//
// On success the parent window's HttpOnly cookies have already been rotated
// by the callback route — the caller just needs to reconnect SSE and refetch
// queries.

interface PopupLoginMessage {
  type: 'login-complete';
}

export function popupLogin(timeoutMs = 5 * 60 * 1000): Promise<boolean> {
  return new Promise((resolve) => {
    if (typeof window === 'undefined') {
      resolve(false);
      return;
    }
    const popup = window.open(
      '/auth/login?popup=1',
      'simaops-login',
      'width=480,height=620,resizable=yes,scrollbars=yes'
    );
    if (!popup) {
      // Browser blocked the popup. Caller falls back to Tier 3 (full redirect).
      resolve(false);
      return;
    }

    let settled = false;
    const finish = (ok: boolean) => {
      if (settled) return;
      settled = true;
      window.removeEventListener('message', onMessage);
      clearInterval(closeWatcher);
      clearTimeout(timer);
      try {
        if (!popup.closed) popup.close();
      } catch {
        // cross-origin close may throw — ignore
      }
      resolve(ok);
    };

    function onMessage(e: MessageEvent) {
      if (e.origin !== window.location.origin) return;
      const data = e.data as PopupLoginMessage | undefined;
      if (data && data.type === 'login-complete') {
        finish(true);
      }
    }
    window.addEventListener('message', onMessage);

    // Detect manual popup close. We can read .closed even cross-origin.
    const closeWatcher = setInterval(() => {
      if (popup.closed) {
        finish(false);
      }
    }, 500);

    const timer = setTimeout(() => finish(false), timeoutMs);
  });
}
