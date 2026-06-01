import { redirect, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import {
  exchangeCode,
  decodeAuthState,
  parseJwtPayload,
  COOKIE_ACCESS,
  COOKIE_REFRESH,
  COOKIE_ID,
  COOKIE_PKCE,
  COOKIE_STATE,
  COOKIE_OPTS,
  REFRESH_COOKIE_MAX_AGE,
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

  // Validate state against the cookie we set in /auth/login. This is the
  // CSRF/replay defense for the authorization-code flow and must FAIL CLOSED:
  // a callback that carries no state, or whose state doesn't match the cookie
  // we issued, is rejected. The one exception is the silent (prompt=none)
  // flow — if its short-lived state cookie has been evicted we still surface a
  // clean failure to the parent frame rather than a hard error page.
  const savedState = cookies.get(`${COOKIE_STATE}_${mode}`);
  if (!state || !savedState || state !== savedState) {
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

  const codeVerifier = cookies.get(`${COOKIE_PKCE}_${mode}`);
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

  // OIDC nonce validation: the id_token's `nonce` claim must match the value
  // we put in the original authorize URL (encoded in our `state` cookie too).
  // Without this, a stolen authorize-response can be replayed against the
  // token endpoint as long as the PKCE verifier remained available — nonce
  // closes that window. Per the OIDC spec this is REQUIRED for any flow that
  // returns an id_token.
  try {
    const idPayload = parseJwtPayload(tokens.id_token);
    const expected = decoded?.nonce;
    if (!expected || idPayload.nonce !== expected) {
      if (mode === 'silent') return silentResultHtml(false);
      throw error(400, 'Nonce mismatch');
    }
  } catch (err) {
    if (mode === 'silent') return silentResultHtml(false);
    // Re-throw SvelteKit errors as-is; only catch the parseJwtPayload throw.
    if (err && typeof err === 'object' && 'status' in err) throw err;
    throw error(400, 'Invalid id_token');
  }

  // Set session cookies.
  cookies.set(COOKIE_ACCESS, tokens.access_token, { ...COOKIE_OPTS, maxAge: tokens.expires_in });
  cookies.set(COOKIE_REFRESH, tokens.refresh_token, { ...COOKIE_OPTS, maxAge: REFRESH_COOKIE_MAX_AGE });
  cookies.set(COOKIE_ID, tokens.id_token, { ...COOKIE_OPTS, maxAge: tokens.expires_in });

  // Clean up PKCE cookies (per-mode).
  cookies.delete(`${COOKIE_PKCE}_${mode}`, { path: '/' });
  cookies.delete(`${COOKIE_STATE}_${mode}`, { path: '/' });

  if (mode === 'silent') return silentResultHtml(true);
  if (mode === 'popup') return popupResultHtml();

  // Reject protocol-relative URLs ("//evil.com" — `startsWith('/')` alone
  // would let an attacker craft /auth/login?return_to=//evil.com and the
  // browser would resolve it to https://evil.com after we redirect).
  const rawReturn = decoded?.returnTo;
  const safeReturn =
    rawReturn && rawReturn.startsWith('/') && !rawReturn.startsWith('//') ? rawReturn : '/dashboard';
  throw redirect(302, safeReturn);
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
