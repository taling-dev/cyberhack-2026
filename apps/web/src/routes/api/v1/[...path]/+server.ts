import { error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';
import { COOKIE_REFRESH, parseJwtPayload } from '$lib/server/auth';

// Backend API URL — internal cluster URL by default, configurable for dev.
const BACKEND_URL = env.PRIVATE_API_URL || 'http://simaops-api.simaops:8080';

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
const handler: RequestHandler = async ({ request, params, locals, url, cookies }) => {
  if (!locals.accessToken) {
    throw error(401, 'unauthenticated');
  }

  const path = params.path;
  if (!path) throw error(400, 'missing rpc path');

  const targetUrl = `${BACKEND_URL}/${path}${url.search}`;

  // Build forwarded headers.
  const headers = new Headers();
  for (const [k, v] of request.headers.entries()) {
    if (FORWARDED_HEADERS.has(k.toLowerCase())) {
      headers.set(k, v);
    }
  }
  headers.set('Authorization', `Bearer ${locals.accessToken}`);

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
    // Skip hop-by-hop headers.
    if (lk === 'transfer-encoding' || lk === 'connection' || lk === 'content-encoding') {
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
