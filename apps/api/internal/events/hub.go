// Package events also defines the in-process broker (Hub) that fans NATS
// messages out to all connected SSE clients on a single API pod.
//
// Design choices:
//   - Single hub per process. Each API pod runs its own hub; cross-pod
//     fan-out happens at the NATS layer via core (non-JetStream) subscriptions.
//   - Per-client buffered channel (size 64). On full, the client is closed
//     immediately so we never block the dispatcher — the EventSource will
//     reconnect via its retry directive.
//   - LRU eviction per user: at most MAX_SSE_PER_USER concurrent connections
//     for a single subject. Defends against runaway tabs / DDoS.
//   - Panic recovery in the NATS handler. If parsing or dispatch panics, we
//     bump a counter and continue — the dispatcher goroutine never dies.
package events

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// MaxConnsPerUser is the per-user concurrent connection cap. Override via
// MAX_SSE_PER_USER env var in main.go on hub construction.
const MaxConnsPerUser = 10

// ClientChannelBuffer is the per-client buffered-channel size. Sized so that
// a brief network hiccup doesn't drop events; tuned with the channel-depth
// histogram.
const ClientChannelBuffer = 64

// Event is a fully-prepared SSE frame ready to be written by the per-client
// goroutine. The hub does the JSON parsing once and shares the result.
type Event struct {
	Subject  string
	Envelope *Envelope
	Raw      []byte // original NATS message bytes (envelope JSON)
}

// Client represents one SSE-connected user. Lives for the duration of a
// single HTTP request.
type Client struct {
	ID          string
	UserSub     string
	Roles       []string
	PrimaryRole string
	Ch          chan *Event
	Done        chan struct{} // closed when the client disconnects
	ConnectedAt time.Time
	closed      atomic.Bool
}

// closeChan signals the client is finished by closing Done. We never close
// Ch directly — that would risk "send on closed channel" panics in the
// dispatcher. Reading from Ch after Done is closed is safe; consumers should
// always select on both channels.
func (c *Client) closeChan() {
	if c.closed.CompareAndSwap(false, true) {
		close(c.Done)
	}
}

// PrimaryRoleOf picks the most privileged role for metrics labeling.
// Order: ADMIN > MANAGER > QC_SUPERVISOR > WAREHOUSE_STAFF > OPERATOR.
func PrimaryRoleOf(roles []string) string {
	priority := []string{"ADMIN", "MANAGER", "QC_SUPERVISOR", "WAREHOUSE_STAFF", "OPERATOR"}
	for _, p := range priority {
		for _, r := range roles {
			if r == p {
				return r
			}
		}
	}
	return "unknown"
}

// Hub is the in-process broker. Concurrency model:
//   - mu guards clients and byUser maps. Reads use RLock during Dispatch.
//   - draining is set during graceful shutdown; new Register calls return
//     an error; existing clients receive a final server-draining event.
//   - maxConnsPerUser is configurable for tests.
type Hub struct {
	mu              sync.RWMutex
	clients         map[*Client]struct{}
	byUser          map[string][]*Client // append-only-then-LRU-shifted
	draining        atomic.Bool
	maxConnsPerUser int
}

// NewHub returns a fresh hub. maxConnsPerUser of 0 falls back to MaxConnsPerUser.
func NewHub(maxConnsPerUser int) *Hub {
	if maxConnsPerUser <= 0 {
		maxConnsPerUser = MaxConnsPerUser
	}
	return &Hub{
		clients:         make(map[*Client]struct{}),
		byUser:          make(map[string][]*Client),
		maxConnsPerUser: maxConnsPerUser,
	}
}

// Register adds a client to the hub. Enforces the per-user cap with LRU
// eviction (oldest connection for that user is closed first). Returns an
// error if the hub is draining — the caller should respond 503.
func (h *Hub) Register(userSub string, roles []string) (*Client, error) {
	if h.draining.Load() {
		return nil, fmt.Errorf("hub is draining")
	}
	c := &Client{
		ID:          uuid.NewString(),
		UserSub:     userSub,
		Roles:       roles,
		PrimaryRole: PrimaryRoleOf(roles),
		Ch:          make(chan *Event, ClientChannelBuffer),
		Done:        make(chan struct{}),
		ConnectedAt: time.Now(),
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Re-check draining under lock (could have flipped between Load and Lock).
	if h.draining.Load() {
		return nil, fmt.Errorf("hub is draining")
	}

	// LRU evict if user already has the cap.
	if existing := h.byUser[userSub]; len(existing) >= h.maxConnsPerUser {
		oldest := existing[0]
		oldest.closeChan()
		delete(h.clients, oldest)
		h.byUser[userSub] = existing[1:]
		SSEConnectionsEvictedTotal.Inc()
	}

	h.clients[c] = struct{}{}
	h.byUser[userSub] = append(h.byUser[userSub], c)
	SSEActiveConnections.WithLabelValues(c.PrimaryRole).Inc()
	return c, nil
}

// Unregister removes a client and closes its channel. Idempotent.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; !ok {
		return
	}
	delete(h.clients, c)
	if conns, ok := h.byUser[c.UserSub]; ok {
		for i, x := range conns {
			if x == c {
				h.byUser[c.UserSub] = append(conns[:i], conns[i+1:]...)
				break
			}
		}
		if len(h.byUser[c.UserSub]) == 0 {
			delete(h.byUser, c.UserSub)
		}
	}
	c.closeChan()
	SSEActiveConnections.WithLabelValues(c.PrimaryRole).Dec()
}

// KickUser closes all SSE connections for a single user. Used by the admin
// kick endpoint and by AssignRole/RevokeRole so role changes take immediate
// effect (next reconnect carries the new role list).
func (h *Hub) KickUser(userSub string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns, ok := h.byUser[userSub]
	if !ok {
		return 0
	}
	for _, c := range conns {
		c.closeChan()
		delete(h.clients, c)
		SSEActiveConnections.WithLabelValues(c.PrimaryRole).Dec()
	}
	delete(h.byUser, userSub)
	return len(conns)
}

// Dispatch fans a single message out to all eligible clients. Subject is the
// NATS subject (== event_type); data is the envelope JSON bytes.
func (h *Hub) Dispatch(subject string, data []byte) {
	t0 := time.Now()
	defer func() {
		SSEHubDispatchDurationSeconds.Observe(time.Since(t0).Seconds())
	}()

	env, err := ParseEnvelope(data)
	if err != nil {
		// Malformed envelope — drop and log via the panic counter analog.
		SSEEventsDroppedTotal.WithLabelValues("malformed_envelope").Inc()
		return
	}
	evt := &Event{Subject: subject, Envelope: env, Raw: data}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		// Skip clients that have already closed (LRU evict, drain, etc).
		// This is the first guard; the second-stage select below catches the
		// race between the check and the send.
		if c.closed.Load() {
			continue
		}
		if !Allow(subject, env, c.Roles, c.UserSub) {
			// Distinguish role vs owner for diagnostics.
			if owner := env.OwnerUserID; owner != "" && owner != c.UserSub && hasOnlyOperator(c.Roles) {
				SSEEventsDroppedTotal.WithLabelValues("owner_filter").Inc()
			} else {
				SSEEventsDroppedTotal.WithLabelValues("role_filter").Inc()
			}
			continue
		}
		// Sample channel depth before send.
		SSEClientChannelDepth.WithLabelValues(c.PrimaryRole).Observe(float64(len(c.Ch)))
		select {
		case <-c.Done:
			// Client disconnected between the closed.Load() check above and
			// here — safe skip, no send attempted.
		case c.Ch <- evt:
			SSEEventsSentTotal.WithLabelValues(subject).Inc()
		default:
			// Slow client — close the Done channel so its handler exits and
			// it gets unregistered from the hub. The buffered Ch is GC'd
			// after the handler returns.
			c.closeChan()
			SSEEventsDroppedTotal.WithLabelValues("slow_client").Inc()
		}
	}
}

// hasOnlyOperator returns true if the user's roles include OPERATOR but no
// privileged role. Used for drop-reason classification.
func hasOnlyOperator(roles []string) bool {
	hasOp := false
	for _, r := range roles {
		if r == "ADMIN" || r == "MANAGER" {
			return false
		}
		if r == "OPERATOR" {
			hasOp = true
		}
	}
	return hasOp
}

// DrainWithJitter signals shutdown, sends a final server-draining frame to
// each client with a jittered retry value, and closes their channels. The
// jittered retry spreads reconnects across [1s, window] to avoid thundering
// herd on the load balancer.
func (h *Hub) DrainWithJitter(window time.Duration) {
	h.draining.Store(true)
	if window < time.Second {
		window = time.Second
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		retryMs := 1000 + rand.IntN(int(window.Milliseconds())) //nolint:gosec // not security-sensitive
		drainEnv := &Envelope{
			EventID:    uuid.NewString(),
			EventType:  "server-draining",
			OccurredAt: time.Now().UTC(),
			ResourceID: "",
			Payload:    []byte("null"),
		}
		drainEvt := &Event{
			Subject:  "server-draining",
			Envelope: drainEnv,
			// Raw payload is unused for the drain event — the SSE handler
			// uses Subject + retryHint to format the frame.
			Raw: []byte(fmt.Sprintf(`{"retry_ms":%d}`, retryMs)),
		}
		// Best-effort send before closing Done. Non-blocking: if the buffer is
		// full or the client is already gone, skip.
		select {
		case c.Ch <- drainEvt:
		default:
		}
		c.closeChan()
		SSEActiveConnections.WithLabelValues(c.PrimaryRole).Dec()
	}
	h.clients = make(map[*Client]struct{})
	h.byUser = make(map[string][]*Client)
}

// IsDraining returns true if DrainWithJitter has been called. New SSE
// handshakes should respond 503 in this state.
func (h *Hub) IsDraining() bool {
	return h.draining.Load()
}

// ConnectionCount returns the total number of registered clients (for tests
// and /readyz introspection).
func (h *Hub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ConnectionsForUser returns the count for a specific user. Useful for
// integration tests that verify the cap.
func (h *Hub) ConnectionsForUser(userSub string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.byUser[userSub])
}

// Run starts the hub's background goroutines (currently none — kept as a
// stable API for future expansion such as periodic stats logging). Returns
// when ctx is canceled.
func (h *Hub) Run(ctx context.Context) {
	<-ctx.Done()
}
