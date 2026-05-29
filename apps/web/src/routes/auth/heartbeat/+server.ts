import type { RequestHandler } from './$types';
import { COOKIE_REFRESH } from '$lib/server/auth';

/**
 * GET /auth/heartbeat — used by the realtime store to keep the access cookie
 * fresh while an SSE stream is open (Set-Cookie can't flush mid-stream).
 *
 * Behavior:
 *   - Runs after hooks.server.ts. If `?force=true` was set, hooks already
 *     attempted a refresh regardless of expiry status.
 *   - If we still have locals.user, return 204. Cookies (potentially newly
 *     rotated) are already set on the response.
 *   - If we don't have locals.user but DO have a refresh-token cookie, the
 *     refresh failed permanently (invalid_grant). Return 401 + revoked.
 *   - If we have nothing at all, return 401 + missing.
 */
export const GET: RequestHandler = async ({ locals, cookies }) => {
  if (locals.user) {
    return new Response(null, {
      status: 204,
      headers: { 'cache-control': 'no-store' },
    });
  }
  // No user. Distinguish "had cookies but refresh died" from "never had any".
  const reason = cookies.get(COOKIE_REFRESH) ? 'revoked' : 'missing';
  return new Response(null, {
    status: 401,
    headers: {
      'X-Auth-Failure-Reason': reason,
      'WWW-Authenticate': `Bearer error="invalid_token", error_description="${reason}"`,
      'cache-control': 'no-store',
    },
  });
};
