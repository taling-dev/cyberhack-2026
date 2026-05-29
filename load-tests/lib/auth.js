// lib/auth.js
//
// Per-VU INDEPENDENT-session token manager for the SimaOps Keycloak realm.
//
// The realm config we must live with (read from the live cluster):
//
//   accessTokenLifespan        = 300   (5-min access tokens)
//   revokeRefreshToken         = true  ┐ every refresh token is SINGLE-USE;
//   refreshTokenMaxReuse       = 0     ┘ reusing one returns invalid_grant
//   bruteForceProtected        = true  ┐ concurrent/rapid logins for the
//   failureFactor              = 5     ├ SAME user return spurious 401
//   quickLoginCheckMilliSeconds= 1000  ┘ "Invalid user credentials"
//
// What this means for the harness:
//
//   * A single token cannot survive a 22-min run — we MUST refresh.
//   * Refresh tokens rotate and are single-use, so two callers must
//     never share one refresh token (the first use invalidates it for
//     the second).
//   * Rapid concurrent password grants for one user trip brute-force
//     detection (empirically: 9 of 10 concurrent budi logins 401'd
//     even with the correct password).
//
// Both earlier strategies failed because they SHARED tokens via
// setup(): all VUs held one refresh token, the first refresh consumed
// it, the other 19 VUs 401'd on refresh and fell back to password
// grants, and 20 concurrent password grants then tripped brute-force.
//
// This version gives every VU its OWN session per user:
//
//   * No setup() pre-warm, no shared tokens. Each VU logs in lazily on
//     first use and stores its own access+refresh token in a per-VU
//     Map (module state is per-VU in k6).
//   * Refresh uses the VU's OWN rotating refresh token — independent
//     sessions refresh in parallel without collision (verified live).
//   * Initial logins are naturally staggered by ramp-up (0→20 VUs over
//     60s ≈ one login per user every ~3s) and each VU's token expiry is
//     staggered by its own login time, so refreshes never bunch into a
//     burst.
//   * passwordGrant() retries up to 4 times with >1.3s spacing on a
//     transient 401 — a safety net for the rare case where two VUs do
//     hit the same user inside the 1s quick-login window during
//     ramp-up. The spacing stays above quickLoginCheckMilliSeconds so a
//     retry never itself counts as a quick-login failure.
//
// Public API (unchanged so scenarios don't need edits):
//
//   import {
//     setupAuth, seedTokenCache,
//     getOperatorToken, getSupervisorToken, getWarehouseToken,
//     getAdminToken, getManagerToken, KC, API,
//   } from './auth.js';
//
// setupAuth/seedTokenCache are retained as a connectivity check / no-op
// for backward compatibility with the scenario files.

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';

export const KC = __ENV.KC_URL || 'https://auth.161.118.244.229.sslip.io';
export const API = __ENV.API_URL || 'https://api.161.118.244.229.sslip.io';

const USERS = [
  { username: 'budi',  role: 'OPERATOR' },
  { username: 'siti',  role: 'QC_SUPERVISOR' },
  { username: 'agus',  role: 'WAREHOUSE_STAFF' },
  { username: 'dewi',  role: 'MANAGER' },
  { username: 'admin', role: 'ADMIN' },
];
const PASSWORD = 'password123';
const CLIENT_ID = 'simaops-web';
const TOKEN_URL = `${KC}/realms/simaops/protocol/openid-connect/token`;

// Refresh when fewer than this many seconds of access-token life remain.
// Small per-VU jitter keeps refreshes from aligning even if two VUs
// happened to log in at the same instant.
const REFRESH_LEEWAY_BASE_S = 45;
const REFRESH_LEEWAY_JITTER_S = 30;

// passwordGrant retry policy — spacing MUST stay above
// quickLoginCheckMilliSeconds (1000ms) so a retry is never itself
// counted as a quick-login failure by Keycloak's brute-force detector.
const PW_RETRY_MAX = 4;
const PW_RETRY_BASE_S = 1.3;

// Per-VU cache: username → { access_token, access_expires_at_unix,
//                            refresh_token, refresh_expires_at_unix }
const cache = new Map();

const authTokenRequests       = new Counter('auth_token_requests');
const authTokenPasswordGrants = new Counter('auth_token_password_grants');
const authTokenRefreshGrants  = new Counter('auth_token_refresh_grants');
const authTokenGrantFailures  = new Counter('auth_token_grant_failures');

function postToken(form, grantTag, username, retryTag) {
  const tags = { rpc: 'auth_login', grant: grantTag, user: username };
  if (retryTag) tags.retry = retryTag;
  const res = http.post(TOKEN_URL, form, {
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    tags,
  });
  authTokenRequests.add(1);
  return res;
}

function tokenFromResponse(json) {
  const nowS = Math.floor(Date.now() / 1000);
  return {
    access_token:            json.access_token,
    access_expires_at_unix:  nowS + (json.expires_in         || 300),
    refresh_token:           json.refresh_token,
    refresh_expires_at_unix: nowS + (json.refresh_expires_in || 1800),
  };
}

// passwordGrant — full resource-owner-password login with bounded retry
// on transient brute-force 401s. Throws only after exhausting retries.
function passwordGrant(username) {
  const form = {
    client_id: CLIENT_ID,
    username,
    password: PASSWORD,
    grant_type: 'password',
  };
  let lastStatus = 0;
  let lastBody = '';
  for (let attempt = 0; attempt <= PW_RETRY_MAX; attempt++) {
    if (attempt > 0) {
      // Linear backoff above the 1s quick-login window, plus jitter.
      sleep(PW_RETRY_BASE_S * attempt + Math.random() * 0.4);
    }
    authTokenPasswordGrants.add(1);
    const res = postToken(form, 'password', username, attempt ? String(attempt) : undefined);
    if (res.status === 200) {
      check(res, { 'password grant 200': () => true });
      return tokenFromResponse(res.json());
    }
    lastStatus = res.status;
    lastBody = res.body;
    authTokenGrantFailures.add(1, { grant: 'password', status: String(res.status) });
    // 4xx other than 401 (e.g. 400 bad request) won't fix on retry.
    if (res.status !== 401 && res.status < 500) break;
  }
  check(null, { 'password grant 200': () => false });
  throw new Error(`password grant failed for ${username} after ${PW_RETRY_MAX} retries: status=${lastStatus} body=${lastBody}`);
}

// refreshGrant — exchange the VU's own (single-use) refresh token for a
// new access+refresh pair. Returns null on any failure so the caller
// falls back to passwordGrant().
function refreshGrant(username, refreshToken) {
  authTokenRefreshGrants.add(1);
  const res = postToken({
    client_id: CLIENT_ID,
    refresh_token: refreshToken,
    grant_type: 'refresh_token',
  }, 'refresh', username);
  const ok = res.status === 200;
  check(res, { 'refresh grant 200': (r) => r.status === 200 });
  if (!ok) {
    authTokenGrantFailures.add(1, { grant: 'refresh', status: String(res.status) });
    return null;
  }
  return tokenFromResponse(res.json());
}

// setupAuth — connectivity fail-fast only. Runs once before any VU
// starts; logs in ONE user serially to surface a dead Keycloak early.
// Returns no tokens — sessions are established lazily per VU so they are
// never shared (shared single-use refresh tokens were the previous bug).
export function setupAuth() {
  const res = postToken({
    client_id: CLIENT_ID,
    username: USERS[0].username,
    password: PASSWORD,
    grant_type: 'password',
  }, 'password', USERS[0].username);
  if (res.status !== 200) {
    throw new Error(`setup connectivity check failed: Keycloak token endpoint returned ${res.status}`);
  }
  return {};
}

// seedTokenCache — retained as a no-op for scenario backward
// compatibility. Per-VU sessions are now established lazily in
// tokenFor(); there is nothing to seed.
export function seedTokenCache(_setupData) { /* no-op by design */ }

function vuLeeway() {
  const vu = (typeof __VU === 'number' ? __VU : 0);
  let h = (vu ^ 0xdeadbeef) >>> 0;
  h = Math.imul(h, 0x85ebca6b) >>> 0;
  h = (h ^ (h >>> 13)) >>> 0;
  h = Math.imul(h, 0xc2b2ae35) >>> 0;
  h = (h ^ (h >>> 16)) >>> 0;
  return REFRESH_LEEWAY_BASE_S + (h % REFRESH_LEEWAY_JITTER_S);
}

// tokenFor — returns a valid access token for `username`, using this
// VU's own session:
//   1. cached access token still fresh → return it.
//   2. near expiry but own refresh token still valid → refresh_token
//      grant (rotates the refresh token; no brute-force counter).
//   3. no/expired refresh token, or refresh failed → password grant
//      (with bounded retry).
function tokenFor(username) {
  const nowS = Math.floor(Date.now() / 1000);
  const cached = cache.get(username);
  if (cached && cached.access_expires_at_unix - nowS > vuLeeway()) {
    return cached.access_token;
  }

  if (cached && cached.refresh_token &&
      cached.refresh_expires_at_unix - nowS > 30) {
    const refreshed = refreshGrant(username, cached.refresh_token);
    if (refreshed) {
      cache.set(username, refreshed);
      return refreshed.access_token;
    }
    // refresh failed (token rotated out / session gone) → password grant.
  }

  const fresh = passwordGrant(username);
  cache.set(username, fresh);
  return fresh.access_token;
}

export function getOperatorToken()   { return tokenFor('budi'); }
export function getSupervisorToken() { return tokenFor('siti'); }
export function getWarehouseToken()  { return tokenFor('agus'); }
export function getManagerToken()    { return tokenFor('dewi'); }
export function getAdminToken()      { return tokenFor('admin'); }

export function getTokenByRole(role) {
  const u = USERS.find((u) => u.role === role);
  if (!u) throw new Error(`unknown role: ${role}`);
  return tokenFor(u.username);
}
