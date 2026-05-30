import { createConnectTransport } from '@connectrpc/connect-web';
import { Code, ConnectError, type Interceptor } from '@connectrpc/connect';
import { browser } from '$app/environment';
import { recoverAuth } from '$lib/auth/recover';

/**
 * Connect-Web transport routed through the SvelteKit BFF proxy.
 *
 * Browser calls go to `/api/v1/<service>/<method>` which is handled by
 * `routes/api/v1/[...path]/+server.ts`. That server-side route reads the
 * HttpOnly access-token cookie, attaches it as a Bearer token, and forwards
 * the call to the backend API.
 *
 * The browser never reads or sees the access token. Same-origin cookies are
 * sent automatically — no CSRF risk because state-changing RPCs require the
 * Bearer token (server-side) AND the SvelteKit handler is same-origin.
 *
 * This transport is browser-only. It deliberately has no way to attach an
 * Authorization header itself, so using it during SSR would hit the backend
 * UNAUTHENTICATED. The guard interceptor below makes that mistake fail loudly
 * instead of silently issuing an anonymous request. Server-side code that
 * needs the API must build its own authenticated transport with the request's
 * access token.
 */
function getApiUrl(): string {
  if (!browser) {
    // Never actually used — see the SSR guard interceptor. Kept as a clearly
    // non-functional sentinel so an accidental SSR call can't reach a real host.
    return 'http://ssr-disallowed.invalid';
  }
  // Browser: route through the BFF proxy (same origin).
  return `${window.location.origin}/api/v1`;
}

const ssrGuard: Interceptor = (next) => (req) => {
  if (!browser) {
    throw new Error(
      `Connect transport used during SSR for ${req.method.name} — this transport is browser-only ` +
        `and would call the API unauthenticated. Build an authenticated server-side transport instead.`,
    );
  }
  return next(req);
};

// On an Unauthenticated (401) response, run the shared auth-recovery ladder
// (force-refresh → silent renew → session-expired modal) and retry the
// request ONCE. Without this, a token expiry on a page with no active SSE
// interaction surfaced as a raw error instead of triggering re-auth.
// Recovery is single-flight, so concurrent 401s collapse into one attempt.
const authRecovery: Interceptor = (next) => async (req) => {
  try {
    return await next(req);
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
      const recovered = await recoverAuth(true);
      if (recovered) {
        return await next(req);
      }
    }
    throw err;
  }
};

export const transport = createConnectTransport({
  baseUrl: getApiUrl(),
  useBinaryFormat: false,
  interceptors: [ssrGuard, authRecovery],
  // credentials: 'include' is implicit because we're same-origin.
  // No interceptor needed — cookies travel with the request automatically.
});
