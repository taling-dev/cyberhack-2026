package events

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// envelopeBytes builds a marshaled envelope for tests.
func envelopeBytes(t *testing.T, eventType, ownerSub string) []byte {
	t.Helper()
	raw, err := NewEnvelope(eventType, "actor", ownerSub, "resource-1", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("envelope: %v", err)
	}
	return raw
}

func TestHub_RegisterUnregister(t *testing.T) {
	h := NewHub(0)
	c1, err := h.Register("alice", []string{"OPERATOR"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if h.ConnectionCount() != 1 {
		t.Errorf("expected 1 connection, got %d", h.ConnectionCount())
	}
	h.Unregister(c1)
	if h.ConnectionCount() != 0 {
		t.Errorf("expected 0 after unregister, got %d", h.ConnectionCount())
	}
	// Idempotent unregister
	h.Unregister(c1)
}

func TestHub_LRUEviction(t *testing.T) {
	h := NewHub(2)
	c1, _ := h.Register("alice", []string{"OPERATOR"})
	c2, _ := h.Register("alice", []string{"OPERATOR"})
	c3, _ := h.Register("alice", []string{"OPERATOR"})

	if h.ConnectionsForUser("alice") != 2 {
		t.Errorf("expected 2 connections for alice after eviction, got %d", h.ConnectionsForUser("alice"))
	}

	// c1 should be evicted (oldest); its Done channel should be closed.
	select {
	case <-c1.Done:
		// good
	case <-time.After(50 * time.Millisecond):
		t.Errorf("c1 was not closed within 50ms after LRU eviction")
	}
	// c2 and c3 should still be alive.
	if c2.closed.Load() || c3.closed.Load() {
		t.Errorf("c2 or c3 closed unexpectedly: c2=%v c3=%v", c2.closed.Load(), c3.closed.Load())
	}
}

func TestHub_Dispatch_RoleAndOwnerFiltering(t *testing.T) {
	h := NewHub(0)
	op1, _ := h.Register("alice", []string{"OPERATOR"})
	op2, _ := h.Register("bob", []string{"OPERATOR"})
	sup, _ := h.Register("siti", []string{"QC_SUPERVISOR"})
	wh, _ := h.Register("dewi", []string{"WAREHOUSE_STAFF"})
	admin, _ := h.Register("root", []string{"ADMIN"})

	// Publish a lot.created envelope owned by alice.
	h.Dispatch("lot.created", envelopeBytes(t, "lot.created", "alice"))

	// op1 (alice) should receive — owns the lot.
	if !receivedWithin(op1.Ch, 100*time.Millisecond) {
		t.Errorf("op1 (owner) did not receive lot.created")
	}
	// op2 (bob) should NOT receive — different operator.
	if receivedWithin(op2.Ch, 50*time.Millisecond) {
		t.Errorf("op2 (non-owner) should not have received lot.created")
	}
	// supervisor sees all lot.* events regardless of owner.
	if !receivedWithin(sup.Ch, 100*time.Millisecond) {
		t.Errorf("supervisor did not receive lot.created")
	}
	// warehouse staff sees lot.* events too (per role permission).
	if !receivedWithin(wh.Ch, 100*time.Millisecond) {
		t.Errorf("warehouse staff did not receive lot.created")
	}
	// admin sees everything.
	if !receivedWithin(admin.Ch, 100*time.Millisecond) {
		t.Errorf("admin did not receive lot.created")
	}

	// QC subjects: warehouse staff sees only the specific allowed ones.
	h.Dispatch("qc.job.created", envelopeBytes(t, "qc.job.created", "alice"))
	if receivedWithin(wh.Ch, 50*time.Millisecond) {
		t.Errorf("warehouse staff should NOT receive qc.job.created")
	}
	h.Dispatch("qc.job.approved", envelopeBytes(t, "qc.job.approved", "alice"))
	if !receivedWithin(wh.Ch, 100*time.Millisecond) {
		t.Errorf("warehouse staff should receive qc.job.approved")
	}
}

func TestHub_Dispatch_SlowClientGetsKicked(t *testing.T) {
	h := NewHub(0)
	c, _ := h.Register("alice", []string{"ADMIN"}) // admin so subject filter won't drop

	// Fill the buffer beyond capacity. ClientChannelBuffer=64 + one over.
	for i := 0; i < ClientChannelBuffer+5; i++ {
		h.Dispatch("audit.log_created", envelopeBytes(t, "audit.log_created", "alice"))
	}

	// After overflow, the channel should be closed.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if c.closed.Load() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("slow client was not closed after channel overflow")
}

func TestHub_KickUser(t *testing.T) {
	h := NewHub(0)
	c1, _ := h.Register("alice", []string{"OPERATOR"})
	c2, _ := h.Register("alice", []string{"OPERATOR"})
	cOther, _ := h.Register("bob", []string{"OPERATOR"})

	got := h.KickUser("alice")
	if got != 2 {
		t.Errorf("expected 2 kicked, got %d", got)
	}
	if !c1.closed.Load() || !c2.closed.Load() {
		t.Errorf("expected alice's clients to be closed")
	}
	if cOther.closed.Load() {
		t.Errorf("bob's client should NOT be closed")
	}
	if h.ConnectionsForUser("alice") != 0 {
		t.Errorf("expected 0 connections for alice, got %d", h.ConnectionsForUser("alice"))
	}
}

func TestHub_DrainWithJitter(t *testing.T) {
	h := NewHub(0)
	c1, _ := h.Register("alice", []string{"OPERATOR"})
	c2, _ := h.Register("bob", []string{"QC_SUPERVISOR"})

	if h.IsDraining() {
		t.Errorf("expected hub not draining initially")
	}
	h.DrainWithJitter(5 * time.Second)
	if !h.IsDraining() {
		t.Errorf("expected hub draining after DrainWithJitter")
	}

	// Both clients should receive a drain event then be closed.
	got1 := receivedWithin(c1.Ch, 100*time.Millisecond)
	got2 := receivedWithin(c2.Ch, 100*time.Millisecond)
	// channel may be closed before we drain it — accept either path.
	if !got1 && !c1.closed.Load() {
		t.Errorf("c1: no drain event and not closed")
	}
	if !got2 && !c2.closed.Load() {
		t.Errorf("c2: no drain event and not closed")
	}

	// New registrations should fail during drain.
	if _, err := h.Register("eve", []string{"OPERATOR"}); err == nil {
		t.Errorf("expected Register to fail during drain")
	}
}

func TestHub_Dispatch_MalformedEnvelope(t *testing.T) {
	h := NewHub(0)
	c, _ := h.Register("alice", []string{"ADMIN"})
	h.Dispatch("lot.created", []byte("not json"))
	if receivedWithin(c.Ch, 50*time.Millisecond) {
		t.Errorf("expected malformed payload to be dropped, got an event")
	}
}

func TestHub_Concurrent_RegisterDispatchUnregister(t *testing.T) {
	h := NewHub(0)
	const N = 50
	var wg sync.WaitGroup
	var dispatched atomic.Int64

	// Spin up clients
	clients := make([]*Client, N)
	for i := 0; i < N; i++ {
		c, err := h.Register("alice", []string{"ADMIN"})
		if err != nil && i < h.maxConnsPerUser {
			t.Fatalf("register: %v", err)
		}
		clients[i] = c
	}

	// Fan out N events while clients drain in parallel.
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			h.Dispatch("audit.log_created", envelopeBytes(t, "audit.log_created", "alice"))
			dispatched.Add(1)
		}
	}()
	go func() {
		defer wg.Done()
		for _, c := range clients {
			if c == nil {
				continue
			}
			go func(c *Client) {
				for {
					select {
					case <-c.Done:
						return
					case <-c.Ch:
						// drain
					}
				}
			}(c)
		}
	}()
	wg.Wait()
	// Cleanup
	for _, c := range clients {
		if c != nil {
			h.Unregister(c)
		}
	}
}

// receivedWithin reads one event from ch with a timeout. Returns true if a
// non-nil event came through, false on timeout or close.
func receivedWithin(ch chan *Event, timeout time.Duration) bool {
	select {
	case evt, ok := <-ch:
		return ok && evt != nil
	case <-time.After(timeout):
		return false
	}
}

// (Decoy unused-import guard; ensures encoding/json stays imported even if we
// remove envelopeBytes' use of map literal in future refactors.)
var _ = json.Marshal
