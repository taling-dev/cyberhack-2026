package events

import "testing"

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		subject string
		pattern string
		want    bool
	}{
		{"lot.created", "lot.>", true},
		{"lot.status_changed", "lot.>", true},
		{"qc.job.created", "qc.>", true},
		{"qc.job.created", "lot.>", false},
		{"qc.job.created", "qc.job.created", true},
		{"qc.job.created", "qc.job.approved", false},
		{"qc.job.approved", "qc.job.approved", true},
		{"warehouse.slot_assigned", ">", true},
		{"audit.log_created", ">", true},
		{"audit.log_created", "audit.>", true},
		{"qc.job.created", "qc.*", false}, // qc.* is one token; qc.job.created is two
		{"qc.created", "qc.*", true},
	}
	for _, c := range cases {
		if got := matchPattern(c.subject, c.pattern); got != c.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", c.subject, c.pattern, got, c.want)
		}
	}
}

func TestAllow_RoleBased(t *testing.T) {
	env := &Envelope{OwnerUserID: "alice"}
	cases := []struct {
		name    string
		subject string
		roles   []string
		userSub string
		owner   string
		want    bool
	}{
		{"operator sees own lot.created", "lot.created", []string{"OPERATOR"}, "alice", "alice", true},
		{"operator NOT see other lot.created", "lot.created", []string{"OPERATOR"}, "bob", "alice", false},
		{"operator NOT see qc.job.created", "qc.job.created", []string{"OPERATOR"}, "alice", "alice", false},
		{"qc supervisor sees qc.job.needs_human_review", "qc.job.needs_human_review", []string{"QC_SUPERVISOR"}, "siti", "alice", true},
		{"qc supervisor sees lot.status_changed regardless of owner", "lot.status_changed", []string{"QC_SUPERVISOR"}, "siti", "alice", true},
		{"warehouse staff sees qc.job.approved", "qc.job.approved", []string{"WAREHOUSE_STAFF"}, "dewi", "alice", true},
		{"warehouse staff does NOT see qc.job.created", "qc.job.created", []string{"WAREHOUSE_STAFF"}, "dewi", "alice", false},
		{"admin sees everything", "audit.log_created", []string{"ADMIN"}, "root", "alice", true},
		{"manager bypasses owner filter even with operator role", "lot.created", []string{"MANAGER", "OPERATOR"}, "boss", "alice", true},
		{"unknown role gets nothing", "lot.created", []string{"GUEST"}, "x", "alice", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env.OwnerUserID = c.owner
			if got := Allow(c.subject, env, c.roles, c.userSub); got != c.want {
				t.Errorf("Allow(%q) = %v, want %v", c.subject, got, c.want)
			}
		})
	}
}

func TestAllow_NilEnvelope(t *testing.T) {
	if Allow("lot.created", nil, []string{"ADMIN"}, "x") {
		t.Error("expected nil envelope to be dropped")
	}
}

func TestAllow_OperatorEmptyOwner(t *testing.T) {
	// If owner_user_id is empty we can't verify ownership — drop conservatively.
	env := &Envelope{OwnerUserID: ""}
	if Allow("lot.created", env, []string{"OPERATOR"}, "alice") {
		t.Error("expected operator to be dropped when owner is empty")
	}
}
