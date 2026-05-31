package events

import (
	"strings"
	"sync"
)

// rolePerm maps each builtin Keycloak realm role to the NATS subject patterns
// that users with that role are allowed to receive over SSE.
//
// Patterns use NATS subject wildcards:
//   - "lot.>"              all lot.* events
//   - "qc.job.approved"    a single specific subject
//   - ">"                  all subjects
//
// Builtin roles are fixed here. CUSTOM roles get their subjects derived from
// their RPC permission grants (see customRolePerm / SetCustomRoleSubjects),
// so a custom role with, say, dispatch permissions also receives dispatch.*
// events. ADMIN/MANAGER are granted ">".
var rolePerm = map[string][]string{
	"OPERATOR":        {"lot.>", "warehouse.slot_assigned", "qc.job.failed", "qc.job.completed", "dispatch.>"},
	"QC_SUPERVISOR":   {"lot.>", "qc.>"},
	"WAREHOUSE_STAFF": {"lot.>", "warehouse.>", "qc.job.approved", "qc.job.completed", "dispatch.>"},
	"MANAGER":         {">"},
	"ADMIN":           {">"},
}

// customRolePerm holds SSE subject patterns for non-builtin roles, derived from
// their RPC grants. Refreshed at startup and after any role mutation. Guarded
// for concurrent read (filter) / write (admin refresh).
var (
	customMu       sync.RWMutex
	customRolePerm = map[string][]string{}
)

// ServiceSubjectPrefixes maps an RPC service token (as it appears in a Connect
// path "/simaops.<svc>.v1.<Svc>Service/Method") to the SSE subject prefixes a
// role granted any RPC on that service should receive. lot/qc are read by most
// flows, so any granted role also gets lot.> for context.
var ServiceSubjectPrefixes = map[string][]string{
	"lot":       {"lot.>"},
	"qc":        {"lot.>", "qc.>"},
	"warehouse": {"lot.>", "warehouse.>"},
	"dispatch":  {"lot.>", "dispatch.>"},
	"audit":     {"audit.>"},
}

// SubjectsForGrants derives the SSE subject patterns a role should receive from
// its granted RPC paths. Used to keep custom-role realtime access in lockstep
// with the role's actual RPC permissions.
func SubjectsForGrants(rpcPaths []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range rpcPaths {
		// path: /simaops.<svc>.v1.<Svc>Service/Method
		i := strings.Index(p, "simaops.")
		if i < 0 {
			continue
		}
		rest := p[i+len("simaops."):]
		svc := rest[:strings.Index(rest+".", ".")]
		for _, sub := range ServiceSubjectPrefixes[svc] {
			if _, ok := seen[sub]; ok {
				continue
			}
			seen[sub] = struct{}{}
			out = append(out, sub)
		}
	}
	return out
}

// SetCustomRoleSubjects replaces the custom-role SSE subject table. pairs maps
// roleName -> rpcPaths; builtin roles are ignored (their subjects are fixed).
func SetCustomRoleSubjects(roleGrants map[string][]string) {
	next := make(map[string][]string, len(roleGrants))
	for role, paths := range roleGrants {
		if _, builtin := rolePerm[role]; builtin {
			continue
		}
		if subs := SubjectsForGrants(paths); len(subs) > 0 {
			next[role] = subs
		}
	}
	customMu.Lock()
	customRolePerm = next
	customMu.Unlock()
}

// subjectsForRole returns the SSE patterns for a role: builtin table first,
// then the data-driven custom table.
func subjectsForRole(role string) []string {
	if pats, ok := rolePerm[role]; ok {
		return pats
	}
	customMu.RLock()
	pats := customRolePerm[role]
	customMu.RUnlock()
	return pats
}

// AllowedSubjects returns the union of subject patterns granted to any of the
// given roles. Used for documentation/testing — actual matching uses Allow.
func AllowedSubjects(roles []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, r := range roles {
		for _, pat := range subjectsForRole(r) {
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
		for _, pat := range subjectsForRole(r) {
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
