import type { Handle } from '@sveltejs/kit';
import {
  COOKIE_ACCESS,
  COOKIE_REFRESH,
  parseJwtPayload,
  refreshTokenWithRetry,
  RefreshError,
  COOKIE_OPTS,
  COOKIE_OPTS_READABLE,
} from '$lib/server/auth';

// Refresh threshold: when the access token has fewer than this many seconds
// of remaining life, hooks.server.ts proactively refreshes it. 90s gives the
// SSE client a window to reconnect with a fresh cookie before the API's
// natural expiry-driven 401 fires (60s buffer + JWT leeway).
const REFRESH_BEFORE_EXPIRY_SECONDS = 90;

function clearAuthCookies(cookies: any) {
  cookies.delete(COOKIE_ACCESS, { path: '/' });
  cookies.delete(COOKIE_REFRESH, { path: '/' });
  cookies.delete('sa_id', { path: '/' });
}

function applyTokens(cookies: any, tokens: { access_token: string; refresh_token: string; expires_in: number }) {
  cookies.set(COOKIE_ACCESS, tokens.access_token, { ...COOKIE_OPTS_READABLE, maxAge: tokens.expires_in });
  cookies.set(COOKIE_REFRESH, tokens.refresh_token, { ...COOKIE_OPTS, maxAge: 86400 * 30 });
}

function userFromPayload(payload: Record<string, any>) {
  return {
    sub: payload.sub,
    username: payload.preferred_username ?? '',
    email: payload.email ?? '',
    name: payload.name ?? payload.preferred_username ?? '',
    roles: payload.realm_access?.roles ?? [],
  };
}

export const handle: Handle = async ({ event, resolve }) => {
  const accessToken = event.cookies.get(COOKIE_ACCESS);
  const refreshTokenValue = event.cookies.get(COOKIE_REFRESH);

  // Force-refresh path used by /auth/heartbeat?force=true. When set, we
  // attempt refresh regardless of current access-token expiry — useful for
  // the "Stay signed in" toast button in the SSE realtime store.
  const forceRefresh =
    event.url.pathname === '/auth/heartbeat' &&
    event.url.searchParams.get('force') === 'true';

  if (!accessToken && !refreshTokenValue) {
    // No auth context — anonymous request.
    return resolve(event);
  }

  if (accessToken) {
    let payload: Record<string, any> = {};
    try {
      payload = parseJwtPayload(accessToken);
    } catch {
      // Corrupt access cookie — clear and continue unauth.
      clearAuthCookies(event.cookies);
      return resolve(event);
    }

    if (!payload.sub) {
      clearAuthCookies(event.cookies);
      return resolve(event);
    }

    const now = Math.floor(Date.now() / 1000);
    const remaining = (payload.exp ?? 0) - now;

    // Token still has plenty of life and we're not force-refreshing — use as-is.
    if (!forceRefresh && remaining > REFRESH_BEFORE_EXPIRY_SECONDS) {
      event.locals.user = userFromPayload(payload);
      event.locals.accessToken = accessToken;
      return resolve(event);
    }

    // Need to refresh. Either close-to-expiry, expired, or force-refresh.
    if (refreshTokenValue) {
      try {
        const tokens = await refreshTokenWithRetry(refreshTokenValue);
        applyTokens(event.cookies, tokens);
        const newPayload = parseJwtPayload(tokens.access_token);
        if (newPayload.sub) {
          event.locals.user = userFromPayload(newPayload);
          event.locals.accessToken = tokens.access_token;
        }
        return resolve(event);
      } catch (err) {
        if (err instanceof RefreshError && err.kind === 'permanent') {
          // Refresh token dead → clear cookies so the user is logged out.
          clearAuthCookies(event.cookies);
        } else {
          // Transient error: leave cookies untouched. If the access token still
          // has *some* life (within leeway), use it; otherwise the request will
          // hit the API with an expired token and the API will return
          // X-Auth-Failure-Reason: expired so the client can recover.
          if (remaining > -60) {
            event.locals.user = userFromPayload(payload);
            event.locals.accessToken = accessToken;
          }
        }
        return resolve(event);
      }
    }

    // No refresh token but access token expired — clear and continue anon.
    if (remaining <= 0) {
      clearAuthCookies(event.cookies);
    } else {
      event.locals.user = userFromPayload(payload);
      event.locals.accessToken = accessToken;
    }
    return resolve(event);
  }

  // No access token but we have a refresh token — try refresh.
  if (refreshTokenValue) {
    try {
      const tokens = await refreshTokenWithRetry(refreshTokenValue);
      applyTokens(event.cookies, tokens);
      const newPayload = parseJwtPayload(tokens.access_token);
      if (newPayload.sub) {
        event.locals.user = userFromPayload(newPayload);
        event.locals.accessToken = tokens.access_token;
      }
    } catch (err) {
      if (err instanceof RefreshError && err.kind === 'permanent') {
        clearAuthCookies(event.cookies);
      }
      // Transient: leave cookies; next request retries.
    }
  }

  return resolve(event);
};
