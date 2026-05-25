import { redirect, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import {
  exchangeCode,
  COOKIE_ACCESS,
  COOKIE_REFRESH,
  COOKIE_ID,
  COOKIE_PKCE,
  COOKIE_STATE,
  COOKIE_OPTS
} from '$lib/server/auth';

export const GET: RequestHandler = async ({ url, cookies }) => {
  const code = url.searchParams.get('code');
  const state = url.searchParams.get('state');
  const errorParam = url.searchParams.get('error');

  if (errorParam) {
    throw error(400, `Auth error: ${errorParam}`);
  }

  if (!code || !state) {
    throw error(400, 'Missing code or state');
  }

  // Validate state
  const savedState = cookies.get(COOKIE_STATE);
  if (state !== savedState) {
    throw error(400, 'State mismatch');
  }

  // Get PKCE verifier
  const codeVerifier = cookies.get(COOKIE_PKCE);
  if (!codeVerifier) {
    throw error(400, 'Missing PKCE verifier');
  }

  // Exchange code for tokens
  const tokens = await exchangeCode(code, codeVerifier);

  // Set session cookies
  cookies.set(COOKIE_ACCESS, tokens.access_token, { ...COOKIE_OPTS, maxAge: tokens.expires_in });
  cookies.set(COOKIE_REFRESH, tokens.refresh_token, { ...COOKIE_OPTS, maxAge: 86400 * 30 });
  cookies.set(COOKIE_ID, tokens.id_token, { ...COOKIE_OPTS, maxAge: tokens.expires_in });

  // Clean up PKCE cookies
  cookies.delete(COOKIE_PKCE, { path: '/' });
  cookies.delete(COOKIE_STATE, { path: '/' });

  throw redirect(302, '/dashboard');
};
