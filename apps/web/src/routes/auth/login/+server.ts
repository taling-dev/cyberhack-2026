import { redirect } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import {
  generateCodeVerifier,
  generateCodeChallenge,
  buildAuthorizationUrl,
  COOKIE_PKCE,
  COOKIE_STATE,
  COOKIE_OPTS
} from '$lib/server/auth';

export const GET: RequestHandler = async ({ cookies }) => {
  const codeVerifier = generateCodeVerifier();
  const codeChallenge = await generateCodeChallenge(codeVerifier);
  const state = crypto.randomUUID();

  // Store PKCE verifier and state in cookies for callback validation
  cookies.set(COOKIE_PKCE, codeVerifier, { ...COOKIE_OPTS, maxAge: 600 });
  cookies.set(COOKIE_STATE, state, { ...COOKIE_OPTS, maxAge: 600 });

  const authUrl = buildAuthorizationUrl(state, codeChallenge);
  throw redirect(302, authUrl);
};
