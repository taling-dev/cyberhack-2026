import { env } from '$env/dynamic/private';
import { env as pubEnv } from '$env/dynamic/public';

// ─── Config ──────────────────────────────────────────────────────

export const KEYCLOAK_ISSUER = env.KEYCLOAK_ISSUER ?? 'http://localhost:8080/realms/simaops';
export const KEYCLOAK_CLIENT_ID = env.KEYCLOAK_CLIENT_ID ?? 'simaops-web';
export const APP_URL = pubEnv.PUBLIC_APP_URL ?? 'http://localhost:5173';

const AUTHORIZATION_ENDPOINT = `${KEYCLOAK_ISSUER}/protocol/openid-connect/auth`;
const TOKEN_ENDPOINT = `${KEYCLOAK_ISSUER}/protocol/openid-connect/token`;
const LOGOUT_ENDPOINT = `${KEYCLOAK_ISSUER}/protocol/openid-connect/logout`;
const USERINFO_ENDPOINT = `${KEYCLOAK_ISSUER}/protocol/openid-connect/userinfo`;

// ─── PKCE ────────────────────────────────────────────────────────

export function generateCodeVerifier(): string {
  const array = new Uint8Array(32);
  crypto.getRandomValues(array);
  return base64UrlEncode(array);
}

export async function generateCodeChallenge(verifier: string): Promise<string> {
  const encoder = new TextEncoder();
  const data = encoder.encode(verifier);
  const digest = await crypto.subtle.digest('SHA-256', data);
  return base64UrlEncode(new Uint8Array(digest));
}

function base64UrlEncode(buffer: Uint8Array): string {
  let str = '';
  for (const byte of buffer) str += String.fromCharCode(byte);
  return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

// ─── Auth URLs ───────────────────────────────────────────────────

export type AuthMode = 'default' | 'silent' | 'popup';

export interface AuthState {
  nonce: string;
  mode: AuthMode;
  returnTo?: string;
}

/**
 * encodeAuthState packs mode/return_to/nonce into the OAuth `state` parameter.
 * The state remains opaque to the auth server but our callback decodes it to
 * decide between redirect / postMessage / return-to-original-url.
 */
export function encodeAuthState(s: AuthState): string {
  const json = JSON.stringify(s);
  return btoa(json).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

export function decodeAuthState(state: string): AuthState | null {
  try {
    const padded = state.replace(/-/g, '+').replace(/_/g, '/');
    const json = atob(padded + '=='.slice(0, (4 - (padded.length % 4)) % 4));
    const obj = JSON.parse(json);
    if (typeof obj.nonce === 'string' && typeof obj.mode === 'string') {
      return obj as AuthState;
    }
    return null;
  } catch {
    return null;
  }
}

export interface BuildAuthorizationUrlOptions {
  state: string;
  codeChallenge: string;
  prompt?: 'none' | 'login' | 'consent';
}

export function buildAuthorizationUrl(opts: BuildAuthorizationUrlOptions): string {
  const params = new URLSearchParams({
    response_type: 'code',
    client_id: KEYCLOAK_CLIENT_ID,
    redirect_uri: `${APP_URL}/auth/callback`,
    scope: 'openid profile email',
    state: opts.state,
    code_challenge: opts.codeChallenge,
    code_challenge_method: 'S256'
  });
  if (opts.prompt) {
    params.set('prompt', opts.prompt);
  }
  return `${AUTHORIZATION_ENDPOINT}?${params}`;
}

export function buildLogoutUrl(idTokenHint?: string): string {
  const params = new URLSearchParams({
    client_id: KEYCLOAK_CLIENT_ID,
    post_logout_redirect_uri: APP_URL
  });
  if (idTokenHint) params.set('id_token_hint', idTokenHint);
  return `${LOGOUT_ENDPOINT}?${params}`;
}

// ─── Token Exchange ──────────────────────────────────────────────

export interface TokenResponse {
  access_token: string;
  refresh_token: string;
  id_token: string;
  expires_in: number;
  token_type: string;
}

export async function exchangeCode(code: string, codeVerifier: string): Promise<TokenResponse> {
  const res = await fetch(TOKEN_ENDPOINT, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      grant_type: 'authorization_code',
      client_id: KEYCLOAK_CLIENT_ID,
      code,
      redirect_uri: `${APP_URL}/auth/callback`,
      code_verifier: codeVerifier
    })
  });
  if (!res.ok) throw new Error(`Token exchange failed: ${res.status}`);
  return res.json();
}

/**
 * Refresh-failure classification.
 *
 * `permanent` — Keycloak rejected the refresh token with `invalid_grant`
 *   (revoked, expired, or rotated). The user must re-authenticate. Cookies
 *   should be cleared.
 *
 * `transient` — network error, 5xx, or other non-deterministic failure.
 *   The refresh token is *probably* still valid; the next request should
 *   retry. Cookies should NOT be cleared on transient errors.
 */
export type RefreshErrorKind = 'permanent' | 'transient';

export class RefreshError extends Error {
  kind: RefreshErrorKind;
  reason: string;
  constructor(kind: RefreshErrorKind, reason: string) {
    super(`refresh failed: ${kind} - ${reason}`);
    this.kind = kind;
    this.reason = reason;
  }
}

/**
 * Refresh the access token. Throws a RefreshError on failure with kind set
 * to either 'permanent' or 'transient' so the caller can decide whether to
 * clear cookies (permanent) or retry (transient).
 */
export async function refreshToken(refreshToken: string): Promise<TokenResponse> {
  let res: Response;
  try {
    res = await fetch(TOKEN_ENDPOINT, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        grant_type: 'refresh_token',
        client_id: KEYCLOAK_CLIENT_ID,
        refresh_token: refreshToken
      })
    });
  } catch (e) {
    // Network error / DNS / connection refused — treat as transient.
    throw new RefreshError('transient', `network: ${(e as Error).message}`);
  }

  if (res.ok) {
    return res.json();
  }

  // Try to parse the OAuth error body to classify.
  let body: { error?: string; error_description?: string } = {};
  try {
    body = await res.json();
  } catch {
    // ignore — fall through to status-based classification
  }

  // 4xx with `invalid_grant` is the canonical "refresh token dead" signal.
  // Other 4xx (invalid_client, etc.) shouldn't happen in steady state — treat
  // as permanent so we don't loop forever.
  if (res.status >= 400 && res.status < 500) {
    return Promise.reject(new RefreshError('permanent', body.error || `http_${res.status}`));
  }

  // 5xx and unexpected statuses → transient.
  throw new RefreshError('transient', body.error || `http_${res.status}`);
}

/**
 * refreshTokenWithRetry retries up to `attempts` times on transient failures
 * with a small fixed backoff. Permanent failures throw immediately so the
 * caller can clear cookies without waiting for retries.
 */
export async function refreshTokenWithRetry(
  rt: string,
  attempts = 3,
  backoffMs = 200
): Promise<TokenResponse> {
  let last: unknown;
  for (let i = 0; i < attempts; i++) {
    try {
      return await refreshToken(rt);
    } catch (err) {
      last = err;
      if (err instanceof RefreshError && err.kind === 'permanent') {
        throw err;
      }
      if (i < attempts - 1) {
        await new Promise((r) => setTimeout(r, backoffMs));
      }
    }
  }
  throw last;
}

// ─── User Info ───────────────────────────────────────────────────

export interface UserInfo {
  sub: string;
  preferred_username: string;
  email: string;
  name: string;
  realm_access?: { roles: string[] };
}

export function parseJwtPayload(token: string): Record<string, any> {
  const parts = token.split('.');
  if (parts.length !== 3) return {};
  const payload = parts[1].replace(/-/g, '+').replace(/_/g, '/');
  return JSON.parse(atob(payload));
}

// ─── Cookie Helpers ──────────────────────────────────────────────

export const COOKIE_ACCESS = 'sa_access';
export const COOKIE_REFRESH = 'sa_refresh';
export const COOKIE_ID = 'sa_id';
export const COOKIE_PKCE = 'sa_pkce';
export const COOKIE_STATE = 'sa_state';

// PUBLIC_APP_URL determines whether cookies should be Secure (https://) or not (http:// for dev)
const isHttps = APP_URL.startsWith('https://');

export const COOKIE_OPTS = {
  path: '/',
  httpOnly: true,
  secure: isHttps,
  sameSite: 'lax' as const
};

// Same options as COOKIE_OPTS (kept for code compat — both cookies HttpOnly now
// since the BFF proxy forwards the token server-side, browser never reads it).
export const COOKIE_OPTS_READABLE = {
  path: '/',
  httpOnly: true,
  secure: isHttps,
  sameSite: 'lax' as const
};
