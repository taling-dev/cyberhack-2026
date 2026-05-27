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
