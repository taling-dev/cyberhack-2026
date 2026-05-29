import { redirect } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import {
  buildLogoutUrl,
  COOKIE_ACCESS,
  COOKIE_REFRESH,
  COOKIE_ID,
  COOKIE_PKCE,
  COOKIE_STATE
} from '$lib/server/auth';

export const GET: RequestHandler = async ({ cookies }) => {
  const idToken = cookies.get(COOKIE_ID);

  // Clear all session cookies (incl. any short-lived PKCE/state cookies left
  // over from an interrupted login).
  cookies.delete(COOKIE_ACCESS, { path: '/' });
  cookies.delete(COOKIE_REFRESH, { path: '/' });
  cookies.delete(COOKIE_ID, { path: '/' });
  cookies.delete(COOKIE_PKCE, { path: '/' });
  cookies.delete(COOKIE_STATE, { path: '/' });

  // Redirect to Keycloak logout
  const logoutUrl = buildLogoutUrl(idToken);
  throw redirect(302, logoutUrl);
};
