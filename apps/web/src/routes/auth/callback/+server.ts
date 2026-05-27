import { redirect, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import {
  exchangeCode,
  decodeAuthState,
  COOKIE_ACCESS,
  COOKIE_REFRESH,
  COOKIE_ID,
  COOKIE_PKCE,
  COOKIE_STATE,
  COOKIE_OPTS,
  COOKIE_OPTS_READABLE,
} from '$lib/server/auth';

/**
 * /auth/callback handles the OIDC redirect for three modes:
 *   - default: standard 302 to /dashboard or ?return_to.
 *   - silent:  iframe → posts {type:'silent-renew-result', ok:true|false} to
 *              its parent and self-closes. Keycloak prompt=none returns
 *              error=login_required if SSO session has expired; we still post
 *              a result so the parent can move to Tier 2.
 *   - popup:   window.open child → posts {type:'login-complete'} to opener
 *              and self-closes. Parent is unchanged.
 */
export const GET: RequestHandler = async ({ url, cookies }) => {
  const code = url.searchParams.get('code');
  const state = url.searchParams.get('state');
  const errorParam = url.searchParams.get('error');

  // Decode state early so silent flows can still respond when Keycloak returned
  // an error rather than a code.
  const decoded = state ? decodeAuthState(state) : null;
  const mode = decoded?.mode ?? 'default';

  // Validate state against the cookie we set in /auth/login. Silent failures
  // (login_required) skip this check because Keycloak returns the same state
  // we set, so the cookie comparison still works.
  const savedState = cookies.get(COOKIE_STATE);
  if (state && savedState && state !== savedState) {
    if (mode === 'silent') return silentResultHtml(false);
    throw error(400, 'State mismatch');
  }

  if (errorParam) {
    if (mode === 'silent') return silentResultHtml(false);
    throw error(400, `Auth error: ${errorParam}`);
  }
  if (!code) {
    if (mode === 'silent') return silentResultHtml(false);
    throw error(400, 'Missing code');
  }

  const codeVerifier = cookies.get(COOKIE_PKCE);
  if (!codeVerifier) {
    if (mode === 'silent') return silentResultHtml(false);
    throw error(400, 'Missing PKCE verifier');
  }

  let tokens;
  try {
    tokens = await exchangeCode(code, codeVerifier);
  } catch {
    if (mode === 'silent') return silentResultHtml(false);
    throw error(400, 'Token exchange failed');
  }

  // Set session cookies.
  cookies.set(COOKIE_ACCESS, tokens.access_token, { ...COOKIE_OPTS_READABLE, maxAge: tokens.expires_in });
  cookies.set(COOKIE_REFRESH, tokens.refresh_token, { ...COOKIE_OPTS, maxAge: 86400 * 30 });
  cookies.set(COOKIE_ID, tokens.id_token, { ...COOKIE_OPTS, maxAge: tokens.expires_in });

  // Clean up PKCE cookies.
  cookies.delete(COOKIE_PKCE, { path: '/' });
  cookies.delete(COOKIE_STATE, { path: '/' });

  if (mode === 'silent') return silentResultHtml(true);
  if (mode === 'popup') return popupResultHtml();

  const returnTo = decoded?.returnTo && decoded.returnTo.startsWith('/') ? decoded.returnTo : '/dashboard';
  throw redirect(302, returnTo);
};

// ─── PostMessage HTML responses ─────────────────────────────────────

function silentResultHtml(ok: boolean): Response {
  // Minimal, self-contained HTML. Posts a single message to the parent (the
  // hidden iframe's containing window) and removes itself. The parent is
  // expected to remove the iframe element regardless of result.
  const body = `<!doctype html><html><body><script>
    (function () {
      try {
        if (window.parent && window.parent !== window) {
          window.parent.postMessage({ type: 'silent-renew-result', ok: ${ok ? 'true' : 'false'} }, location.origin);
        }
      } catch (e) {}
    })();
  </script></body></html>`;
  return new Response(body, {
    status: 200,
    headers: {
      'content-type': 'text/html; charset=utf-8',
      'cache-control': 'no-store',
    },
  });
}

function popupResultHtml(): Response {
  const body = `<!doctype html><html><body><script>
    (function () {
      try {
        if (window.opener) {
          window.opener.postMessage({ type: 'login-complete' }, location.origin);
        }
      } catch (e) {}
      window.close();
    })();
  </script><p>Sign-in complete. You may close this window.</p></body></html>`;
  return new Response(body, {
    status: 200,
    headers: {
      'content-type': 'text/html; charset=utf-8',
      'cache-control': 'no-store',
    },
  });
}
