package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/auth"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/events"
)

// heartbeatInterval is the cadence at which we send `:heartbeat\n\n` SSE
// comment frames to keep the connection alive across nginx/Cloudflare idle
// timeouts. 15s is short enough to survive 30s default proxy timeouts and
// long enough to be unobtrusive in network logs.
const heartbeatInterval = 15 * time.Second

// EventsHandler returns an http.HandlerFunc that streams realtime events to
// authenticated users via Server-Sent Events.
//
// Behavior:
//   - 503 if the hub is currently draining (rolling update in progress).
//   - 401 (handled by JWT middleware) if no/invalid token.
//   - First frame: `event: connection-info` carrying tokenExpiresAt,
//     refreshExpiresAt, and serverTime so the browser can schedule its own
//     reconnect timer using a single (browser) clock.
//   - Subsequent frames: real domain events, formatted as
//     `id:<event_id>\nevent:<subject>\ndata:<envelope-json>\n\n`.
//   - 15s heartbeat comments to defeat proxy idle timeouts.
//   - On graceful drain, the hub injects a final server-draining event with
//     a per-client jittered `retry:` value before closing.
func EventsHandler(hub *events.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if hub.IsDraining() {
			http.Error(w, `{"code":"unavailable","reason":"draining"}`, http.StatusServiceUnavailable)
			return
		}

		claims := auth.GetClaims(r.Context())
		if claims == nil {
			// JWT middleware should have rejected this; defensive 401.
			http.Error(w, `{"code":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		// Use the same identifier resolution as handler.userFromCtx so the
		// SSE owner filter aligns with the value services write to created_by
		// (preferred_username, falling back to JWT sub).
		userSub := claims.Username
		if userSub == "" {
			userSub = claims.Sub
		}
		roles := claims.Roles
		accessExp := auth.GetAccessTokenExp(r.Context())

		// The BFF parses the refresh-token cookie and forwards its `exp` here
		// so we can include it in connection-info without calling Keycloak.
		refreshExp := int64(0)
		if v := r.Header.Get("X-Refresh-Expires-At"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				refreshExp = n
			}
		}

		client, err := hub.Register(userSub, roles)
		if err != nil {
			http.Error(w, `{"code":"unavailable","reason":"draining"}`, http.StatusServiceUnavailable)
			return
		}
		defer hub.Unregister(client)

		// Disable the global write timeout for this long-lived response. The
		// http.NewResponseController API was added in Go 1.20+ and lets us
		// override the Server-level WriteTimeout per-handler. It also unwraps
		// any middleware-installed http.ResponseWriter so SetWriteDeadline /
		// Flush still find the underlying connection.
		rc := http.NewResponseController(w)
		_ = rc.SetWriteDeadline(time.Time{}) // no deadline

		// SSE response headers.
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache, no-transform")
		h.Set("Connection", "keep-alive")
		// Disable nginx buffering so chunks arrive immediately.
		h.Set("X-Accel-Buffering", "no")
		// Hint to the client (and the BFF for diagnostics) when this token expires.
		if accessExp > 0 {
			h.Set("X-Token-Expires-At", strconv.FormatInt(accessExp, 10))
		}
		w.WriteHeader(http.StatusOK)

		flush := func() {
			// NewResponseController.Flush walks any middleware wrappers to
			// find the underlying http.Flusher. Casting w.(http.Flusher)
			// directly would fail because logger/metrics/idempotency/audit
			// middleware all wrap the writer without forwarding the Flusher
			// interface.
			_ = rc.Flush()
		}

		// Initial frames: a connect comment + retry directive + connection-info event.
		_, _ = fmt.Fprintf(w, ":connected\nretry: 1000\n\n")
		flush()

		infoPayload, _ := json.Marshal(map[string]int64{
			"tokenExpiresAt":   accessExp,
			"refreshExpiresAt": refreshExp,
			"serverTime":       time.Now().Unix(),
		})
		_, _ = fmt.Fprintf(w, "event: connection-info\ndata: %s\n\n", infoPayload)
		flush()

		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-client.Done:
				// Hub closed the client (drain, kick, or LRU evict). The
				// per-client send loop in Dispatch may still have queued the
				// drain event — drain Ch first, then return.
				for {
					select {
					case evt, ok := <-client.Ch:
						if !ok {
							return
						}
						writeFrame(w, evt)
						flush()
					default:
						return
					}
				}
			case evt, ok := <-client.Ch:
				if !ok {
					return
				}
				writeFrame(w, evt)
				flush()
			case <-ticker.C:
				if _, err := fmt.Fprintf(w, ":heartbeat\n\n"); err != nil {
					return
				}
				flush()
			}
		}
	}
}

// writeFrame serializes one Event into SSE wire format. server-draining
// events carry a per-client jittered `retry:` value as their first line so
// the browser uses that interval on the next reconnect.
func writeFrame(w http.ResponseWriter, evt *events.Event) {
	if evt == nil || evt.Envelope == nil {
		return
	}
	if evt.Subject == "server-draining" {
		var hint struct {
			RetryMs int `json:"retry_ms"`
		}
		_ = json.Unmarshal(evt.Raw, &hint)
		if hint.RetryMs <= 0 {
			hint.RetryMs = 5000
		}
		_, _ = fmt.Fprintf(w, "retry: %d\nevent: %s\ndata: {}\n\n", hint.RetryMs, evt.Subject)
		return
	}
	_, _ = fmt.Fprintf(w,
		"id: %s\nevent: %s\ndata: %s\n\n",
		evt.Envelope.EventID, evt.Subject, evt.Raw,
	)
}
