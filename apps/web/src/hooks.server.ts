import type { Handle } from '@sveltejs/kit';
import {
  COOKIE_ACCESS,
  COOKIE_REFRESH,
  COOKIE_ID,
  parseJwtPayload,
  refreshTokenWithRetry,
  RefreshError,
  COOKIE_OPTS,
  REFRESH_COOKIE_MAX_AGE,
} from '$lib/server/auth';

// Locale cookie name. Read by the i18n module on the client; we use it
// server-side to set the document <html lang="…"> attribute so screen
// readers pick the correct pronunciation rules.
const LOCALE_COOKIE = 'simaops_locale';

// Security response headers. Applied to every response (incl. SSE and the
// API proxy) by the `addSecurityHeaders` helper below.
//
// NOTE: Content-Security-Policy is NOT set here. It is owned by SvelteKit's
// `kit.csp` config (svelte.config.js) so the framework can attach a hash to
// its own injected inline hydration script. A hand-rolled `script-src 'self'`
// header here previously blocked that script (the `dashboard:13` CSP error).

// Refresh threshold: when the access token has fewer than this many seconds
// of remaining life, hooks.server.ts proactively refreshes it. 90s gives the
// SSE client a window to reconnect with a fresh cookie before the API's
// natural expiry-driven 401 fires (60s buffer + JWT leeway).
const REFRESH_BEFORE_EXPIRY_SECONDS = 90;

// Locales we ship translations for. Anything else falls back to "en".
const SUPPORTED_LOCALES = new Set(['en', 'id']);

function addSecurityHeaders(response: Response): void {
  // Always-on hardening headers. CSP is handled by SvelteKit (kit.csp),
  // not here — see the note above the removed CSP constant.
  response.headers.set('X-Content-Type-Options', 'nosniff');
  response.headers.set('X-Frame-Options', 'DENY');
  response.headers.set('Referrer-Policy', 'strict-origin-when-cross-origin');
  response.headers.set('Permissions-Policy', 'camera=(), microphone=(), geolocation=(), payment=()');
  response.headers.set('Strict-Transport-Security', 'max-age=31536000; includeSubDomains');
}

function clearAuthCookies(cookies: any) {
  cookies.delete(COOKIE_ACCESS, { path: '/' });
  cookies.delete(COOKIE_REFRESH, { path: '/' });
  cookies.delete('sa_id', { path: '/' });
}

function applyTokens(cookies: any, tokens: { access_token: string; refresh_token: string; id_token?: string; expires_in: number }) {
  cookies.set(COOKIE_ACCESS, tokens.access_token, { ...COOKIE_OPTS, maxAge: tokens.expires_in });
  cookies.set(COOKIE_REFRESH, tokens.refresh_token, { ...COOKIE_OPTS, maxAge: REFRESH_COOKIE_MAX_AGE });
  // Persist the rotated id_token so logout can still send id_token_hint past
  // the initial 5-minute access-token lifetime (without this the sa_id cookie
  // expires and IdP single-logout degrades to a confirmation prompt).
  if (tokens.id_token) {
    cookies.set(COOKIE_ID, tokens.id_token, { ...COOKIE_OPTS, maxAge: tokens.expires_in });
  }
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
  // Resolve the user's preferred locale BEFORE running auth so the page
  // transform below can substitute the correct lang attribute. Falls back
  // to "en" for unsupported values (defense against hand-edited cookies).
  const localeCookie = event.cookies.get(LOCALE_COOKIE);
  const lang = localeCookie && SUPPORTED_LOCALES.has(localeCookie) ? localeCookie : 'en';
  event.locals.lang = lang;

  // Run the auth ladder. `inner` only sets event.locals.{user,accessToken}
  // and may rotate cookies; the actual response is built once at the end
  // so security headers are guaranteed to be applied to every response.
  await runAuth(event);

  // Impersonation (display layer): if the real signed-in user is ADMIN and an
  // impersonation cookie is set, present the impersonated user across the UI so
  // the sidebar name + role-gated nav match what the API enforces. The real
  // admin identity is preserved in locals.realUser for the banner / safety.
  const imp = event.cookies.get('impersonate');
  if (imp && event.locals.user?.roles?.includes('ADMIN')) {
    try {
      const t = JSON.parse(imp);
      if (t?.username && t.username !== event.locals.user.username) {
        event.locals.realUser = event.locals.user;
        event.locals.user = {
          sub: t.sub ?? '',
          username: t.username,
          email: t.email ?? '',
          name: t.name ?? t.username,
          roles: Array.isArray(t.roles) ? t.roles : [],
        };
      }
    } catch {
      // malformed cookie: ignore, keep the real user
    }
  }

  const response = await resolve(event, {
    transformPageChunk: ({ html }) => html.replace('<html lang="en">', `<html lang="${lang}">`),
  });
  addSecurityHeaders(response);
  return response;
};

// runAuth implements the proactive refresh ladder. Mutates event.locals and
// event.cookies as needed; never returns a Response (the caller does).
async function runAuth(event: Parameters<Handle>[0]['event']) {
  const accessToken = event.cookies.get(COOKIE_ACCESS);
  const refreshTokenValue = event.cookies.get(COOKIE_REFRESH);

  // Force-refresh path used by /auth/heartbeat?force=true. When set, we
  // attempt refresh regardless of current access-token expiry — useful for
  // the "Stay signed in" toast button in the SSE realtime store.
  const forceRefresh =
    event.url.pathname === '/auth/heartbeat' &&
    event.url.searchParams.get('force') === 'true';

  if (!accessToken && !refreshTokenValue) {
    return; // anonymous
  }

  if (accessToken) {
    let payload: Record<string, any> = {};
    try {
      payload = parseJwtPayload(accessToken);
    } catch {
      clearAuthCookies(event.cookies);
      return;
    }

    if (!payload.sub) {
      clearAuthCookies(event.cookies);
      return;
    }

    const now = Math.floor(Date.now() / 1000);
    const remaining = (payload.exp ?? 0) - now;

    if (!forceRefresh && remaining > REFRESH_BEFORE_EXPIRY_SECONDS) {
      event.locals.user = userFromPayload(payload);
      event.locals.accessToken = accessToken;
      return;
    }

    if (refreshTokenValue) {
      try {
        const tokens = await refreshTokenWithRetry(refreshTokenValue);
        applyTokens(event.cookies, tokens);
        const newPayload = parseJwtPayload(tokens.access_token);
        if (newPayload.sub) {
          event.locals.user = userFromPayload(newPayload);
          event.locals.accessToken = tokens.access_token;
        }
        return;
      } catch (err) {
        if (err instanceof RefreshError && err.kind === 'permanent') {
          // H1: A 'permanent' invalid_grant can mean the refresh token was
          // genuinely revoked, OR that we lost a single-use-rotation race
          // (another concurrent request — possibly on another replica —
          // already rotated it). If our access token is still valid, the
          // session is fine: serve it rather than force a spurious logout.
          // Only clear cookies when the access token is also dead.
          if (remaining > 0) {
            event.locals.user = userFromPayload(payload);
            event.locals.accessToken = accessToken;
          } else {
            clearAuthCookies(event.cookies);
          }
        } else if (remaining > -60) {
          // Transient error: serve the request with the still-just-valid token.
          event.locals.user = userFromPayload(payload);
          event.locals.accessToken = accessToken;
        }
        return;
      }
    }

    if (remaining <= 0) {
      clearAuthCookies(event.cookies);
    } else {
      event.locals.user = userFromPayload(payload);
      event.locals.accessToken = accessToken;
    }
    return;
  }

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
    }
  }
}
