package handler

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/auth"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/events"
)

// fakeAuthMiddleware injects fixed claims so we can drive the SSE handler
// without a real Keycloak. Mirrors what auth.JWTMiddleware does on success.
func fakeAuthMiddleware(sub string, roles []string, exp int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), auth.ClaimsKey, &auth.Claims{
			Sub:   sub,
			Roles: roles,
			Exp:   exp,
		})
		ctx = context.WithValue(ctx, auth.AccessTokenExpKey, exp)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TestEventsHandler_SendsConnectionInfoFirst(t *testing.T) {
	hub := events.NewHub(0)
	exp := time.Now().Add(5 * time.Minute).Unix()
	srv := httptest.NewServer(fakeAuthMiddleware("alice", []string{"ADMIN"}, exp, EventsHandler(hub)))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	if got := resp.Header.Get("X-Token-Expires-At"); got == "" {
		t.Errorf("missing X-Token-Expires-At header")
	}

	// Read enough lines to find the first `event: connection-info` block.
	reader := bufio.NewReader(resp.Body)
	deadline := time.Now().Add(2 * time.Second)
	sawConnInfo := false
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.HasPrefix(line, "event: connection-info") {
			sawConnInfo = true
			break
		}
	}
	if !sawConnInfo {
		t.Errorf("did not see connection-info event")
	}
}

func TestEventsHandler_503WhenDraining(t *testing.T) {
	hub := events.NewHub(0)
	hub.DrainWithJitter(time.Second)
	exp := time.Now().Add(5 * time.Minute).Unix()
	srv := httptest.NewServer(fakeAuthMiddleware("alice", []string{"ADMIN"}, exp, EventsHandler(hub)))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

func TestEventsHandler_DispatchedEventReachesClient(t *testing.T) {
	hub := events.NewHub(0)
	exp := time.Now().Add(5 * time.Minute).Unix()
	srv := httptest.NewServer(fakeAuthMiddleware("alice", []string{"ADMIN"}, exp, EventsHandler(hub)))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	// Wait briefly for the client to register with the hub.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if hub.ConnectionCount() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if hub.ConnectionCount() == 0 {
		t.Fatalf("client did not register with hub")
	}

	// Build a real envelope and dispatch it via the hub.
	raw, err := events.NewEnvelope("lot.created", "actor", "alice", "lot-1",
		map[string]string{"lot_number": "LOT-X"})
	if err != nil {
		t.Fatalf("envelope: %v", err)
	}
	hub.Dispatch("lot.created", raw)

	// Read the stream looking for our event.
	reader := bufio.NewReader(resp.Body)
	timeoutAt := time.Now().Add(2 * time.Second)
	sawLotCreated := false
	for time.Now().Before(timeoutAt) {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.HasPrefix(line, "event: lot.created") {
			sawLotCreated = true
			break
		}
	}
	if !sawLotCreated {
		t.Errorf("did not receive lot.created event")
	}
}

// TestEventsHandler_KicksOnTokenExpiry verifies that the SSE stream is
// force-closed shortly after the access token's `exp` is reached. The
// client should observe EOF (read returns io.EOF) within a small window
// past the expiry, not the full lifetime of the connection.
func TestEventsHandler_KicksOnTokenExpiry(t *testing.T) {
	hub := events.NewHub(0)
	// Token expires 1s from now — handler should close the stream within
	// ~1s + tokenExpiryGrace (5s) = at most ~6s. Use 10s timeout for slack.
	exp := time.Now().Add(1 * time.Second).Unix()
	srv := httptest.NewServer(fakeAuthMiddleware("alice", []string{"ADMIN"}, exp, EventsHandler(hub)))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	// Drain the stream — when the timer fires, the connection closes and
	// ReadString returns an error (EOF or "connection reset").
	reader := bufio.NewReader(resp.Body)
	timeoutAt := time.Now().Add(10 * time.Second)
	closedAt := time.Time{}
	for time.Now().Before(timeoutAt) {
		_, err := reader.ReadString('\n')
		if err != nil {
			closedAt = time.Now()
			break
		}
	}
	if closedAt.IsZero() {
		t.Fatalf("stream did not close within 10s of token expiry")
	}

	// Sanity: the close happened *after* the token's exp, not before.
	expTime := time.Unix(exp, 0)
	if closedAt.Before(expTime) {
		t.Errorf("stream closed at %v, before token exp %v", closedAt, expTime)
	}
	// And not absurdly late — within exp + 8s.
	if closedAt.After(expTime.Add(8 * time.Second)) {
		t.Errorf("stream closed at %v, more than 8s after token exp %v", closedAt, expTime)
	}
}

// TestEventsHandler_DoesNotKickAlreadyExpiredToken verifies that when a
// client reconnects with a token that is *already past exp* (but still
// within the JWT leeway window so the middleware accepted it), we do NOT
// schedule another kick. Otherwise the same client would bounce in a tight
// loop reusing the same expired cookie. The connection should ride out
// the heartbeat interval without being closed by the expiry timer.
func TestEventsHandler_DoesNotKickAlreadyExpiredToken(t *testing.T) {
	hub := events.NewHub(0)
	// Token already expired 30 seconds ago — within typical 60s leeway, so
	// the JWT middleware would still accept it; our handler should NOT
	// re-kick.
	exp := time.Now().Add(-30 * time.Second).Unix()
	srv := httptest.NewServer(fakeAuthMiddleware("alice", []string{"ADMIN"}, exp, EventsHandler(hub)))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	// Watch for ~3 seconds — long enough to observe the connection-info
	// frame, then verify NO close occurs (the expiry timer is intentionally
	// not scheduled in this case).
	reader := bufio.NewReader(resp.Body)
	deadline := time.Now().Add(3 * time.Second)
	gotConnInfo := false
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("stream closed unexpectedly at %v (would loop with same expired token): %v",
				time.Now(), err)
		}
		if strings.HasPrefix(line, "event: connection-info") {
			gotConnInfo = true
		}
	}
	if !gotConnInfo {
		t.Errorf("did not see connection-info frame")
	}
}
