import { error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';
import { env as pubEnv } from '$env/dynamic/public';
import { COOKIE_REFRESH, parseJwtPayload } from '$lib/server/auth';

// Backend API URL — internal cluster URL by default, configurable for dev.
const BACKEND_URL = env.PRIVATE_API_URL || 'http://simaops-api.simaops:8080';

// Allowed Origin for cross-site protection on mutating requests. SameSite=Lax
// blocks GET cross-origin but a top-level form POST from an attacker site
// would still ride along with the cookies. We require the Origin header to
// match this app's own origin for any non-safe method.
//
// Multiple values are accepted (comma-separated) so a single deployment can
// serve from both the app domain and (e.g.) a www. alias.
const APP_ORIGINS = (pubEnv.PUBLIC_APP_URL ?? 'http://localhost:5173')
  .split(',')
  .map((s) => s.trim())
  .filter(Boolean);
const SAFE_METHODS = new Set(['GET', 'HEAD', 'OPTIONS']);

// Forwarded request headers — only what the API needs. SSE handshakes require
// `accept` and `last-event-id`; everything else is the existing Connect set.
const FORWARDED_HEADERS = new Set([
  'content-type',
  'accept',
  'last-event-id',
  'connect-protocol-version',
  'connect-timeout-ms',
  'idempotency-key',
  'x-request-id',
]);

// Response headers we explicitly want to surface to the browser (in addition
// to whatever the upstream sends). Hop-by-hop headers are stripped below.
const PASSTHROUGH_RESPONSE_HEADERS = new Set([
  'x-auth-failure-reason',
  'www-authenticate',
  'x-token-expires-at',
]);

/**
 * BFF proxy: forwards Connect RPC and SSE traffic from the browser to the API.
 *
 * Adds the Authorization Bearer header from the HttpOnly cookie server-side.
 * For the SSE path (/api/v1/events), additionally:
 *   - parses the refresh-token cookie's `exp` and forwards it as
 *     `X-Refresh-Expires-At` so the API can include it in connection-info
 *     without calling Keycloak.
 *   - sets `Cache-Control: no-cache, no-transform` on the response.
 *
 * Path mapping: /api/v1/<service>/<method> → <BACKEND>/<service>/<method>
 */
// Connect RPC paths look like `simaops.lot.v1.LotService/CreateLot` and the
// SSE bridge uses the literal `events`. Anything outside this character set
// (including `..`, `//`, scheme prefixes, query injection in the path) is a
// SSRF attempt against the cluster's other services and must be rejected
// before we hand the value to fetch().
const RPC_PATH_RE = /^[a-zA-Z0-9._/-]+$/;

const handler: RequestHandler = async ({ request, params, locals, url, cookies }) => {
  if (!locals.accessToken) {
    throw error(401, 'unauthenticated');
  }

  // CSRF defense — for any non-safe method, require an Origin header that
  // matches our app. Connect-RPC always sets `Content-Type: application/json`
  // which prevents simple form-encoded POSTs, and the BFF cookie has
  // SameSite=Lax which blocks cross-origin sub-resource fetches. The Origin
  // check covers the residual case (top-level form POST with appropriate
  // content type, or a navigator that does send Origin on cross-origin POST).
  if (!SAFE_METHODS.has(request.method)) {
    const origin = request.headers.get('origin');
    if (!origin || !APP_ORIGINS.includes(origin)) {
      throw error(403, 'cross-origin request blocked');
    }
  }

  const path = params.path;
  if (!path) throw error(400, 'missing rpc path');

  // SSRF defense — refuse anything that doesn't look like a Connect RPC
  // path or the literal SSE bridge path. This blocks `..`, `//`, encoded
  // characters, and absolute-URL injection.
  if (!RPC_PATH_RE.test(path) || path.includes('..') || path.startsWith('/')) {
    throw error(400, 'invalid rpc path');
  }

  const targetUrl = `${BACKEND_URL}/${path}${url.search}`;

  // Build forwarded headers.
  const headers = new Headers();
  for (const [k, v] of request.headers.entries()) {
    if (FORWARDED_HEADERS.has(k.toLowerCase())) {
      headers.set(k, v);
    }
  }
  headers.set('Authorization', `Bearer ${locals.accessToken}`);

  // Impersonation: forward the chosen target username. The API only honors
  // this when the real verified token is ADMIN, so a forged cookie is inert.
  const impersonate = cookies.get('impersonate');
  if (impersonate) {
    try {
      const u = JSON.parse(impersonate)?.username;
      if (u) headers.set('X-Impersonate', u);
    } catch {
      // legacy plain-string cookie
      headers.set('X-Impersonate', impersonate);
    }
  }

  // SSE-specific: include refresh-token expiry hint so the API's
  // connection-info frame can drive the client's session-ending warning.
  if (path === 'events') {
    const rt = cookies.get(COOKIE_REFRESH);
    if (rt) {
      try {
        const payload = parseJwtPayload(rt);
        if (typeof payload.exp === 'number') {
          headers.set('X-Refresh-Expires-At', String(payload.exp));
        }
      } catch {
        // Malformed refresh cookie — skip the hint, the API just won't
        // know when refresh expires (no session-ending warning that round).
      }
    }
  }

  // Forward request body for non-GET/HEAD methods.
  let body: BodyInit | undefined;
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    body = await request.arrayBuffer();
  }

  // Propagate AbortSignal so the upstream connection closes when the browser
  // disconnects mid-SSE.
  const res = await fetch(targetUrl, {
    method: request.method,
    headers,
    body,
    signal: request.signal,
  });

  // Stream response back, preserving Connect headers + SSE/auth metadata.
  const respHeaders = new Headers();
  for (const [k, v] of res.headers.entries()) {
    const lk = k.toLowerCase();
    // Skip hop-by-hop / length-framing headers. We re-stream res.body, so the
    // platform must recompute the framing — forwarding the upstream
    // content-length (or content-encoding) can leave a declared length that
    // doesn't match the bytes we actually emit, which browsers reject as
    // ERR_HTTP2_PROTOCOL_ERROR even though the status is 200.
    if (
      lk === 'transfer-encoding' ||
      lk === 'connection' ||
      lk === 'content-encoding' ||
      lk === 'content-length'
    ) {
      continue;
    }
    respHeaders.set(k, v);
  }
  // Defensive: always make sure auth-related response headers are present
  // even if the upstream Headers iteration above missed them (some proxies
  // strip unfamiliar headers; this is belt-and-braces).
  for (const want of PASSTHROUGH_RESPONSE_HEADERS) {
    const existing = res.headers.get(want);
    if (existing && !respHeaders.has(want)) {
      respHeaders.set(want, existing);
    }
  }
  if (path === 'events') {
    respHeaders.set('Cache-Control', 'no-cache, no-transform');
  }

  return new Response(res.body, {
    status: res.status,
    headers: respHeaders,
  });
};

export const GET: RequestHandler = handler;
export const POST: RequestHandler = handler;
export const PUT: RequestHandler = handler;
export const DELETE: RequestHandler = handler;
export const OPTIONS: RequestHandler = handler;
