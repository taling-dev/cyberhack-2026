// Cross-tab toast deduplication. Multiple browser tabs each connect their
// own SSE streams, so the same domain event reaches the user N times. To
// avoid showing the same toast N times, we record event IDs to localStorage
// (shared across tabs of the same origin) with a TTL.
//
// Returns true if this is the first time we've seen the event_id within the
// TTL window, false if it was already seen.

const STORAGE_KEY = 'simaops:seenEvents';
const TTL_MS = 30_000;

interface SeenMap {
  [eventId: string]: number;
}

function read(): SeenMap {
  if (typeof localStorage === 'undefined') return {};
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    return JSON.parse(raw) as SeenMap;
  } catch {
    return {};
  }
}

function write(map: SeenMap) {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(map));
  } catch {
    // localStorage full or disabled — best-effort.
  }
}

/**
 * markSeen returns true if the event is new (caller should show the toast).
 * Returns false if another tab/page already toasted this event recently.
 *
 * Also garbage-collects entries older than the TTL on each call so the
 * map doesn't grow unbounded.
 */
export function markSeen(eventId: string): boolean {
  if (!eventId) return true; // always show events without ids
  const now = Date.now();
  const map = read();
  // GC expired entries.
  for (const id of Object.keys(map)) {
    if (now - map[id] > TTL_MS) delete map[id];
  }
  if (map[eventId]) {
    write(map);
    return false;
  }
  map[eventId] = now;
  write(map);
  return true;
}
