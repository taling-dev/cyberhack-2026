import { redirect } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import {
  generateCodeVerifier,
  generateCodeChallenge,
  buildAuthorizationUrl,
  encodeAuthState,
  type AuthMode,
  COOKIE_PKCE,
  COOKIE_STATE,
  COOKIE_OPTS,
} from '$lib/server/auth';

/**
 * GET /auth/login — start the OIDC authorization-code flow.
 *
 * Query params:
 *   ?silent=1     — invisible iframe re-auth (Tier 1 of the recovery ladder).
 *                   Adds prompt=none to the authorize URL so Keycloak only
 *                   succeeds if the SSO session is still alive.
 *   ?popup=1      — interactive login in a popup window (Tier 2). The
 *                   callback responds with postMessage HTML instead of a
 *                   redirect so the parent window keeps its state.
 *   ?return_to=…  — path to redirect to after a successful default-mode login.
 */
export const GET: RequestHandler = async ({ cookies, url }) => {
  const silent = url.searchParams.get('silent') === '1';
  const popup = url.searchParams.get('popup') === '1';
  const returnTo = url.searchParams.get('return_to') ?? undefined;

  const mode: AuthMode = silent ? 'silent' : popup ? 'popup' : 'default';

  const codeVerifier = generateCodeVerifier();
  const codeChallenge = await generateCodeChallenge(codeVerifier);
  const nonce = crypto.randomUUID();
  const state = encodeAuthState({ nonce, mode, returnTo });

  // Store PKCE verifier and state in cookies for callback validation.
  cookies.set(COOKIE_PKCE, codeVerifier, { ...COOKIE_OPTS, maxAge: 600 });
  cookies.set(COOKIE_STATE, state, { ...COOKIE_OPTS, maxAge: 600 });

  const authUrl = buildAuthorizationUrl({
    state,
    codeChallenge,
    prompt: silent ? 'none' : undefined,
  });
  throw redirect(302, authUrl);
};
