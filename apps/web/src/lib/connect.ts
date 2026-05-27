import { createConnectTransport } from '@connectrpc/connect-web';
import { browser } from '$app/environment';

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
 * Server-side (SSR) calls go directly to the cluster-internal API URL.
 */
function getApiUrl(): string {
  if (!browser) {
    // SSR: hit the API directly inside the cluster (no proxy needed).
    return 'http://simaops-api.simaops:8080';
  }
  // Browser: route through the BFF proxy (same origin).
  return `${window.location.origin}/api/v1`;
}

export const transport = createConnectTransport({
  baseUrl: getApiUrl(),
  useBinaryFormat: false,
  // credentials: 'include' is implicit because we're same-origin.
  // No interceptor needed — cookies travel with the request automatically.
});
