// Realtime client store. Owns the EventSource lifecycle plus the auth-refresh
// state machine.
//
// Responsibilities:
//   1. Open EventSource at `/api/v1/events`. Reopen on disconnect with
//      exponential backoff (1s → 30s, jittered). Multiple disconnects within a
//      short window escalate to longer backoffs.
//   2. Send a /auth/heartbeat fetch every 60s so the BFF rotates the access
//      cookie before the API forces a reconnect.
//   3. On `connection-info` event, schedule a client-clock-driven reconnect
//      timer 60s before the access token expires. Clock-skew is corrected
//      using the server's reported `serverTime`.
//   4. On 401/auth-failure, run the three-tier recovery ladder:
//        Tier 0 — /auth/heartbeat?force=true
//        Tier 1 — silentRenew() iframe
//        Tier 2 — set state to 'session-expired' (modal triggered)
//   5. Dispatch domain events to TanStack Query invalidations and a recent-
//      events ring buffer used by the row-highlight action and toaster.
//
// The store is a Svelte 5 $state-rune object; consumers can read `state.*`
// reactively in components (e.g. the topbar pulse).

import { browser } from '$app/environment';
import type { QueryClient } from '@tanstack/svelte-query';
import { silentRenew } from '$lib/auth/silentRenew';

export type RealtimeStatus =
  | 'idle'
  | 'connecting'
  | 'live'
  | 'reconnecting'
  | 're-authenticating'
  | 'session-expired';

export interface RealtimeEvent {
  id: string;
  subject: string;
  envelope: {
    event_id: string;
    event_type: string;
    occurred_at: string;
    actor_id: string;
    owner_user_id: string;
    resource_id: string;
    payload: any;
  };
  receivedAt: number;
}

export interface ConnectionInfo {
  tokenExpiresAt: number;
  refreshExpiresAt: number;
  serverTime: number;
  /** server clock minus client clock, in seconds */
  skew: number;
}

export interface RealtimeState {
  status: RealtimeStatus;
  connectionInfo: ConnectionInfo | null;
  recentEvents: RealtimeEvent[]; // newest first, capped at MAX_RECENT
  lastEventAt: number;
}

const MAX_RECENT = 50;
const HEARTBEAT_INTERVAL_MS = 60_000;
// Reconnect 60s before the access token expires so the new SSE connection
// picks up a freshly rotated cookie.
const RECONNECT_BEFORE_EXPIRY_MS = 60_000;
// Show the "session ending soon" warning 5 minutes before the refresh token
// expires.
const SESSION_ENDING_WARN_MS = 5 * 60_000;

interface ConnectOptions {
  /** Called once per event (after invalidation runs). For toaster integration. */
  onEvent?: (e: RealtimeEvent) => void;
  /** Called when the warning timer fires. */
  onSessionEnding?: () => void;
}

export interface RealtimeHandle {
  state: RealtimeState;
  disconnect(): void;
}

/**
 * connectRealtime opens the SSE stream and returns a handle. Always call
 * `disconnect()` on cleanup (onDestroy / route change to anonymous).
 */
export function connectRealtime(queryClient: QueryClient, opts: ConnectOptions = {}): RealtimeHandle {
  const state: RealtimeState = $state({
    status: 'idle',
    connectionInfo: null,
    recentEvents: [],
    lastEventAt: 0,
  });

  if (!browser) {
    return { state, disconnect: () => {} };
  }

  let es: EventSource | null = null;
  let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  let reconnectTokenTimer: ReturnType<typeof setTimeout> | null = null;
  let sessionEndingTimer: ReturnType<typeof setTimeout> | null = null;
  let backoffTimer: ReturnType<typeof setTimeout> | null = null;
  let consecutiveFailures = 0;
  let cancelled = false;
  let recovering = false; // guards against re-entry during tier dispatch
  let consecutive401s = 0;

  function clearTimer(t: ReturnType<typeof setTimeout> | ReturnType<typeof setInterval> | null) {
    if (t !== null) clearTimeout(t as any);
  }

  function clearAllTimers() {
    if (heartbeatTimer) {
      clearInterval(heartbeatTimer);
      heartbeatTimer = null;
    }
    clearTimer(reconnectTokenTimer);
    reconnectTokenTimer = null;
    clearTimer(sessionEndingTimer);
    sessionEndingTimer = null;
    clearTimer(backoffTimer);
    backoffTimer = null;
  }

  function closeEventSource() {
    if (es) {
      try {
        es.close();
      } catch {
        // ignore
      }
      es = null;
    }
  }

  function scheduleHeartbeat() {
    if (heartbeatTimer) clearInterval(heartbeatTimer);
    heartbeatTimer = setInterval(async () => {
      try {
        const res = await fetch('/auth/heartbeat', { credentials: 'same-origin' });
        if (res.status === 401) {
          consecutive401s++;
          // A heartbeat 401 means hooks.server.ts already attempted a refresh
          // and it failed (revoked) — or there were no cookies (missing).
          // Either way Tier-0 force-refresh is futile, so default a missing
          // header to 'revoked' (skips Tier-0, goes straight to silent renew)
          // rather than 'expired' (which would waste a doomed force-refresh).
          const reason = res.headers.get('X-Auth-Failure-Reason') ?? 'revoked';
          await recoverFromAuthFail(reason);
        } else {
          consecutive401s = 0;
        }
      } catch {
        // Network blip — heartbeat will fire again in 60s. Don't escalate.
      }
    }, HEARTBEAT_INTERVAL_MS);
  }

  function nowSec() {
    return Math.floor(Date.now() / 1000);
  }

  function scheduleReconnectFromInfo(info: ConnectionInfo) {
    clearTimer(reconnectTokenTimer);
    if (info.tokenExpiresAt <= 0) return;
    const targetSec = info.tokenExpiresAt + info.skew - RECONNECT_BEFORE_EXPIRY_MS / 1000;
    const delayMs = Math.max(1000, (targetSec - nowSec()) * 1000);
    reconnectTokenTimer = setTimeout(() => {
      // Proactive reconnect — cookie should already be fresh from the heartbeat.
      reconnect('token-rotation');
    }, delayMs);

    clearTimer(sessionEndingTimer);
    if (info.refreshExpiresAt > 0) {
      const warnSec = info.refreshExpiresAt + info.skew - SESSION_ENDING_WARN_MS / 1000;
      const warnDelay = Math.max(0, (warnSec - nowSec()) * 1000);
      if (warnDelay > 0) {
        sessionEndingTimer = setTimeout(() => {
          opts.onSessionEnding?.();
        }, warnDelay);
      }
    }
  }

  async function recoverFromAuthFail(reason: string): Promise<void> {
    if (recovering) return;
    recovering = true;
    state.status = 're-authenticating';
    closeEventSource();
    try {
      // Tier 0: force-refresh the access cookie.
      if (reason === 'expired') {
        try {
          const res = await fetch('/auth/heartbeat?force=true', { credentials: 'same-origin' });
          if (res.ok) {
            consecutive401s = 0;
            connect();
            return;
          }
        } catch {
          // fall through
        }
      }
      // Tier 1: silent OIDC renew via iframe.
      const ok = await silentRenew();
      if (ok) {
        consecutive401s = 0;
        connect();
        return;
      }
      // Tier 2: session expired; modal handles it.
      state.status = 'session-expired';
    } finally {
      recovering = false;
    }
  }

  function reconnect(_reason: string) {
    closeEventSource();
    if (cancelled) return;
    state.status = 'reconnecting';
    connect();
  }

  function backoffDelayMs() {
    // Exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s; jitter ±25%.
    const base = Math.min(30_000, 1000 * 2 ** Math.min(consecutiveFailures, 5));
    const jitter = base * (0.75 + Math.random() * 0.5);
    return Math.floor(jitter);
  }

  function handleConnectionInfo(raw: string) {
    try {
      const data = JSON.parse(raw) as { tokenExpiresAt: number; refreshExpiresAt: number; serverTime: number };
      const skew = nowSec() - data.serverTime;
      state.connectionInfo = { ...data, skew };
      scheduleReconnectFromInfo(state.connectionInfo);
    } catch {
      // ignore malformed connection-info
    }
  }

  function pushEvent(e: RealtimeEvent) {
    state.recentEvents = [e, ...state.recentEvents].slice(0, MAX_RECENT);
    state.lastEventAt = e.receivedAt;
    invalidateForEvent(queryClient, e);
    // Dispatch a highlight DOM event so list pages can react via the
    // `use:highlightOnChange` action without coupling to this module.
    if (typeof window !== 'undefined') {
      const lotId =
        e.envelope.payload?.lot_id ??
        (e.subject.startsWith('lot.') ? e.envelope.resource_id : undefined);
      window.dispatchEvent(
        new CustomEvent('simaops:highlight', {
          detail: { resourceId: e.envelope.resource_id, lotId },
        }),
      );
    }
    opts.onEvent?.(e);
  }

  function attachListeners(source: EventSource) {
    source.addEventListener('connection-info', (ev) => {
      handleConnectionInfo((ev as MessageEvent).data);
      state.status = 'live';
      consecutiveFailures = 0;
      // Reconnect implies we may have missed events; full TanStack invalidate.
      void queryClient.invalidateQueries();
    });
    source.addEventListener('server-draining', () => {
      // Server signaled drain. The server already sent a `retry:` directive
      // so EventSource will pick that up automatically; we just go to
      // reconnecting state.
      state.status = 'reconnecting';
    });
    // Generic message handler — not used since we listen per event type below.
    source.onerror = () => {
      // The browser's native EventSource auto-reconnect would race the API's
      // token-expiry kick (every 120-300s the API closes the stream when the
      // JWT exp passes). If we let it reconnect with the SAME expired cookie
      // the API just rejected, we get a tight 401-retry storm. Instead:
      //   1. Close the EventSource so the browser stops auto-reconnecting.
      //   2. After 2 consecutive failures (= the kick is real, not a
      //      transient blip), trigger the auth-recovery ladder which forces
      //      a refresh and reopens the stream with a fresh cookie.
      //   3. Otherwise schedule a manual backoff reconnect.
      consecutiveFailures++;
      state.status = 'reconnecting';
      closeEventSource();
      if (consecutiveFailures >= 2 || consecutive401s >= 3) {
        void recoverFromAuthFail('expired');
      } else {
        clearTimer(backoffTimer);
        backoffTimer = setTimeout(() => connect(), backoffDelayMs());
      }
    };
    // Domain events: SSE allows arbitrary event names. We attach a listener
    // for every subject we care about.
    for (const subject of WATCHED_SUBJECTS) {
      source.addEventListener(subject, (ev) => {
        try {
          const me = ev as MessageEvent;
          const env = JSON.parse(me.data);
          pushEvent({
            id: env.event_id ?? me.lastEventId ?? '',
            subject,
            envelope: env,
            receivedAt: Date.now(),
          });
        } catch {
          // ignore malformed event
        }
      });
    }
  }

  function connect() {
    if (cancelled) return;
    closeEventSource();
    state.status = state.status === 'idle' ? 'connecting' : state.status;
    try {
      es = new EventSource('/api/v1/events', { withCredentials: true });
    } catch {
      state.status = 'reconnecting';
      backoffTimer = setTimeout(() => connect(), backoffDelayMs());
      return;
    }
    attachListeners(es);
  }

  function disconnect() {
    cancelled = true;
    clearAllTimers();
    closeEventSource();
    state.status = 'idle';
  }

  // Bootstrap.
  scheduleHeartbeat();
  connect();

  return { state, disconnect };
}

// ─── Static event-to-query-key mapping ──────────────────────────────────

const WATCHED_SUBJECTS = [
  'lot.created',
  'lot.status_changed',
  'qc.job.created',
  'qc.job.completed',
  'qc.job.needs_human_review',
  'qc.job.reviewed',
  'qc.job.failed',
  'qc.job.approved',
  'warehouse.slot_assigned',
  'audit.log_created',
] as const;

function invalidateForEvent(qc: QueryClient, e: RealtimeEvent) {
  const env = e.envelope;
  const resourceId = env.resource_id;
  const payload = (env.payload ?? {}) as Record<string, any>;
  const lotId: string | undefined = payload.lot_id ?? (e.subject.startsWith('lot.') ? resourceId : undefined);

  switch (e.subject) {
    case 'lot.created':
    case 'lot.status_changed':
      qc.invalidateQueries({ queryKey: ['lots'] });
      qc.invalidateQueries({ queryKey: ['lot', resourceId] });
      qc.invalidateQueries({ queryKey: ['lot-timeline', resourceId] });
      qc.invalidateQueries({ queryKey: ['dashboard-ops'] });
      qc.invalidateQueries({ queryKey: ['nav-badges'] });
      // Warehouse pending count derives from lots in QC_APPROVED state.
      if (e.subject === 'lot.status_changed') {
        qc.invalidateQueries({ queryKey: ['warehouse-pending'] });
        qc.invalidateQueries({ queryKey: ['warehouse-queue'] });
        qc.invalidateQueries({ queryKey: ['qc-review-lots'] });
      }
      break;
    case 'qc.job.created':
    case 'qc.job.completed':
    case 'qc.job.needs_human_review':
    case 'qc.job.failed':
      qc.invalidateQueries({ queryKey: ['qc-jobs'] });
      qc.invalidateQueries({ queryKey: ['qc-review-lots'] });
      qc.invalidateQueries({ queryKey: ['qc-job', resourceId] });
      qc.invalidateQueries({ queryKey: ['qc-result', resourceId] });
      if (lotId) {
        qc.invalidateQueries({ queryKey: ['lot-timeline', lotId] });
        qc.invalidateQueries({ queryKey: ['lot', lotId] });
      }
      qc.invalidateQueries({ queryKey: ['dashboard-qc'] });
      qc.invalidateQueries({ queryKey: ['dashboard-ops'] });
      qc.invalidateQueries({ queryKey: ['nav-badges'] });
      break;
    case 'qc.job.reviewed':
    case 'qc.job.approved':
      qc.invalidateQueries({ queryKey: ['qc-jobs'] });
      qc.invalidateQueries({ queryKey: ['qc-review-lots'] });
      qc.invalidateQueries({ queryKey: ['qc-job', resourceId] });
      qc.invalidateQueries({ queryKey: ['lots'] });
      if (lotId) {
        qc.invalidateQueries({ queryKey: ['lot', lotId] });
        qc.invalidateQueries({ queryKey: ['lot-timeline', lotId] });
      }
      qc.invalidateQueries({ queryKey: ['warehouse-pending'] });
      qc.invalidateQueries({ queryKey: ['warehouse-queue'] });
      qc.invalidateQueries({ queryKey: ['dashboard-qc'] });
      qc.invalidateQueries({ queryKey: ['dashboard-ops'] });
      qc.invalidateQueries({ queryKey: ['nav-badges'] });
      break;
    case 'warehouse.slot_assigned':
      qc.invalidateQueries({ queryKey: ['warehouse-slots'] });
      qc.invalidateQueries({ queryKey: ['warehouse-pending'] });
      qc.invalidateQueries({ queryKey: ['warehouse-queue'] });
      qc.invalidateQueries({ queryKey: ['lots'] });
      if (lotId) {
        qc.invalidateQueries({ queryKey: ['lot', lotId] });
        qc.invalidateQueries({ queryKey: ['lot-timeline', lotId] });
      }
      qc.invalidateQueries({ queryKey: ['dashboard-warehouse'] });
      qc.invalidateQueries({ queryKey: ['dashboard-ops'] });
      qc.invalidateQueries({ queryKey: ['nav-badges'] });
      break;
    case 'audit.log_created':
      qc.invalidateQueries({ queryKey: ['audit-logs'] });
      break;
  }
}
