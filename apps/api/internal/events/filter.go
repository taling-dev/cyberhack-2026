package events

import "strings"

// rolePerm maps each Keycloak realm role to the NATS subject patterns that
// users with that role are allowed to receive over SSE.
//
// Patterns use NATS subject wildcards:
//   - "lot.>"              all lot.* events
//   - "qc.job.approved"    a single specific subject
//   - ">"                  all subjects
//
// Roles not in this table receive no events. ADMIN/MANAGER are intentionally
// granted ">" so the audit page works without a separate broadcast plane.
var rolePerm = map[string][]string{
	"OPERATOR":        {"lot.>", "warehouse.slot_assigned", "qc.job.failed", "qc.job.completed", "dispatch.>"},
	"QC_SUPERVISOR":   {"lot.>", "qc.>"},
	"WAREHOUSE_STAFF": {"lot.>", "warehouse.>", "qc.job.approved", "qc.job.completed", "dispatch.>"},
	"MANAGER":         {">"},
	"ADMIN":           {">"},
}

// AllowedSubjects returns the union of subject patterns granted to any of the
// given roles. Used for documentation/testing — actual matching uses Allow.
func AllowedSubjects(roles []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, r := range roles {
		for _, pat := range rolePerm[r] {
			if _, ok := seen[pat]; ok {
				continue
			}
			seen[pat] = struct{}{}
			out = append(out, pat)
		}
	}
	return out
}

// matchPattern returns true if subject matches the given NATS-style pattern.
// Supports `>` (multi-token wildcard, only at end) and `*` (single token).
func matchPattern(subject, pattern string) bool {
	if pattern == ">" {
		return true
	}
	subjTokens := strings.Split(subject, ".")
	patTokens := strings.Split(pattern, ".")
	for i, p := range patTokens {
		if p == ">" {
			// match any number of remaining tokens
			return i <= len(subjTokens)
		}
		if i >= len(subjTokens) {
			return false
		}
		if p == "*" {
			continue
		}
		if p != subjTokens[i] {
			return false
		}
	}
	return len(subjTokens) == len(patTokens)
}

// Allow returns true if a user with the given roles and userSub should
// receive the event with the given subject and envelope.
//
// The decision combines:
//   - Outer (role-based): does any of the user's roles permit this subject?
//   - Inner (owner-scoped): is the user's role OPERATOR, and if so does
//     env.OwnerUserID match userSub? Operators only see events for resources
//     they own (lots they created and the QC/warehouse events derived from
//     those lots).
//
// If env is nil (e.g. malformed publish), the event is dropped. ADMIN/MANAGER
// short-circuit owner filtering — they see everything.
func Allow(subject string, env *Envelope, roles []string, userSub string) bool {
	if env == nil {
		return false
	}
	// 1. Role check — at least one role must permit this subject.
	allowedByRole := false
	hasPrivileged := false
	hasOperator := false
	for _, r := range roles {
		if r == "ADMIN" || r == "MANAGER" {
			hasPrivileged = true
		}
		if r == "OPERATOR" {
			hasOperator = true
		}
		for _, pat := range rolePerm[r] {
			if matchPattern(subject, pat) {
				allowedByRole = true
				break
			}
		}
		if allowedByRole {
			break
		}
	}
	if !allowedByRole {
		return false
	}
	// 2. Owner check — only when the *only* matching role is OPERATOR.
	// If the user also holds a privileged role (ADMIN/MANAGER) they bypass
	// owner filtering. This matches the UX: a manager who is also an operator
	// for testing purposes still sees everything.
	if hasPrivileged {
		return true
	}
	if hasOperator {
		// Operator must own the resource. Empty owner_user_id means we can't
		// confirm ownership — drop conservatively.
		if env.OwnerUserID == "" || env.OwnerUserID != userSub {
			return false
		}
	}
	return true
}
