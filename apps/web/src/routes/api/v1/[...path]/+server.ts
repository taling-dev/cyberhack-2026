import { error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';

// Backend API URL — internal cluster URL by default, configurable for dev.
const BACKEND_URL = env.PRIVATE_API_URL || 'http://simaops-api.simaops:8080';

// Forwarded headers — only what Connect RPC needs. Strip anything else for safety.
const FORWARDED_HEADERS = new Set([
  'content-type',
  'connect-protocol-version',
  'connect-timeout-ms',
  'idempotency-key',
  'x-request-id',
]);

/**
 * BFF proxy: forwards Connect RPC calls from the browser to the API.
 * Adds the Authorization Bearer header from the HttpOnly cookie server-side.
 * The browser never sees the access token.
 *
 * Path mapping: /api/v1/<service>/<method> → <BACKEND>/<service>/<method>
 */
const handler: RequestHandler = async ({ request, params, locals, url }) => {
  if (!locals.accessToken) {
    throw error(401, 'unauthenticated');
  }

  const path = params.path;
  if (!path) throw error(400, 'missing rpc path');

  const targetUrl = `${BACKEND_URL}/${path}${url.search}`;

  // Build forwarded headers
  const headers = new Headers();
  for (const [k, v] of request.headers.entries()) {
    if (FORWARDED_HEADERS.has(k.toLowerCase())) {
      headers.set(k, v);
    }
  }
  headers.set('Authorization', `Bearer ${locals.accessToken}`);

  // Forward the request
  let body: BodyInit | undefined;
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    body = await request.arrayBuffer();
  }

  const res = await fetch(targetUrl, {
    method: request.method,
    headers,
    body,
  });

  // Stream response back, preserving Connect headers
  const respHeaders = new Headers();
  for (const [k, v] of res.headers.entries()) {
    // Skip hop-by-hop headers
    if (['transfer-encoding', 'connection', 'content-encoding'].includes(k.toLowerCase())) continue;
    respHeaders.set(k, v);
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
