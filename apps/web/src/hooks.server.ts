import type { Handle } from '@sveltejs/kit';
import { COOKIE_ACCESS, COOKIE_REFRESH, parseJwtPayload, refreshToken, COOKIE_OPTS, COOKIE_OPTS_READABLE } from '$lib/server/auth';

function clearAuthCookies(cookies: any) {
  cookies.delete(COOKIE_ACCESS, { path: '/' });
  cookies.delete(COOKIE_REFRESH, { path: '/' });
  cookies.delete('sa_id', { path: '/' });
}

export const handle: Handle = async ({ event, resolve }) => {
  const accessToken = event.cookies.get(COOKIE_ACCESS);
  const refreshTokenValue = event.cookies.get(COOKIE_REFRESH);

  if (accessToken) {
    try {
      // Parse JWT to get user info and check expiry
      const payload = parseJwtPayload(accessToken);
      const now = Math.floor(Date.now() / 1000);

      if (!payload.sub) {
        // Malformed token — clear and continue unauth
        clearAuthCookies(event.cookies);
      } else if (payload.exp && payload.exp > now) {
        // Token still valid — set user in locals
        event.locals.user = {
          sub: payload.sub,
          username: payload.preferred_username ?? '',
          email: payload.email ?? '',
          name: payload.name ?? payload.preferred_username ?? '',
          roles: payload.realm_access?.roles ?? []
        };
        event.locals.accessToken = accessToken;
      } else if (refreshTokenValue) {
        // Token expired — try refresh
        try {
          const tokens = await refreshToken(refreshTokenValue);
          event.cookies.set(COOKIE_ACCESS, tokens.access_token, { ...COOKIE_OPTS_READABLE, maxAge: tokens.expires_in });
          event.cookies.set(COOKIE_REFRESH, tokens.refresh_token, { ...COOKIE_OPTS, maxAge: 86400 * 30 });

          const newPayload = parseJwtPayload(tokens.access_token);
          if (newPayload.sub) {
            event.locals.user = {
              sub: newPayload.sub,
              username: newPayload.preferred_username ?? '',
              email: newPayload.email ?? '',
              name: newPayload.name ?? newPayload.preferred_username ?? '',
              roles: newPayload.realm_access?.roles ?? []
            };
            event.locals.accessToken = tokens.access_token;
          }
        } catch {
          // Refresh failed (Keycloak restart, expired refresh, etc.) — clear cookies
          clearAuthCookies(event.cookies);
        }
      } else {
        // Token expired, no refresh — clear
        clearAuthCookies(event.cookies);
      }
    } catch {
      // parseJwtPayload threw — token is corrupt, clear it
      clearAuthCookies(event.cookies);
    }
  }

  return resolve(event);
};
