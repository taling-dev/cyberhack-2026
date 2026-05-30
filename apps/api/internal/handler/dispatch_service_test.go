package handler

import (
	"testing"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	dispatchv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/dispatch/v1"
)

// TestDispatchTransitions asserts the dispatch lifecycle FSM: the happy path
// PENDING → SCHEDULED → IN_TRANSIT → DELIVERED, CANCELLED reachable from every
// non-terminal state, and no transitions out of the terminal states. This is
// the contract the SvelteKit nextStatus() helper mirrors.
func TestDispatchTransitions(t *testing.T) {
	P := dispatchv1.DispatchStatus_DISPATCH_STATUS_PENDING
	S := dispatchv1.DispatchStatus_DISPATCH_STATUS_SCHEDULED
	IT := dispatchv1.DispatchStatus_DISPATCH_STATUS_IN_TRANSIT
	D := dispatchv1.DispatchStatus_DISPATCH_STATUS_DELIVERED
	C := dispatchv1.DispatchStatus_DISPATCH_STATUS_CANCELLED

	cases := []struct {
		from, to dispatchv1.DispatchStatus
		want     bool
	}{
		{P, S, true},
		{S, IT, true},
		{IT, D, true},
		{P, C, true},
		{S, C, true},
		{IT, C, true},
		// Illegal skips / reversals.
		{P, IT, false},
		{P, D, false},
		{S, D, false},
		{S, P, false},
		{IT, S, false},
		// Terminal states allow nothing.
		{D, C, false},
		{D, IT, false},
		{C, S, false},
		{C, D, false},
	}
	for _, c := range cases {
		got := dispatchTransitions[c.from][c.to]
		if got != c.want {
			t.Errorf("transition %s → %s = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

// TestDispatchStatusRoundTrip verifies the proto↔DB enum mappings are lossless
// for every non-unspecified status, and that UNSPECIFIED defaults safely.
func TestDispatchStatusRoundTrip(t *testing.T) {
	all := []dispatchv1.DispatchStatus{
		dispatchv1.DispatchStatus_DISPATCH_STATUS_PENDING,
		dispatchv1.DispatchStatus_DISPATCH_STATUS_SCHEDULED,
		dispatchv1.DispatchStatus_DISPATCH_STATUS_IN_TRANSIT,
		dispatchv1.DispatchStatus_DISPATCH_STATUS_DELIVERED,
		dispatchv1.DispatchStatus_DISPATCH_STATUS_CANCELLED,
	}
	for _, s := range all {
		if got := dispatchStatusFromDB(dispatchStatusToDB(s)); got != s {
			t.Errorf("round-trip %s = %s", s, got)
		}
	}
	// Unknown DB value maps to UNSPECIFIED; UNSPECIFIED proto maps to PENDING default.
	if dispatchStatusFromDB(db.DispatchesStatus("BOGUS")) != dispatchv1.DispatchStatus_DISPATCH_STATUS_UNSPECIFIED {
		t.Error("unknown DB status should map to UNSPECIFIED")
	}
	if dispatchStatusToDB(dispatchv1.DispatchStatus_DISPATCH_STATUS_UNSPECIFIED) != db.DispatchesStatusPENDING {
		t.Error("UNSPECIFIED should default to PENDING in DB")
	}
}
