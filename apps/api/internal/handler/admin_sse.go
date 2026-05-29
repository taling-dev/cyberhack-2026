package handler

import (
	"encoding/json"
	"net/http"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/auth"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/events"
)

// AdminSSEKickHandler returns an HTTP handler that closes all SSE connections
// for a single user. ADMIN-only. Used by:
//   - manual operator: POST /admin/sse/kick?user=<sub>
//   - role-change auto-kick: AssignRole / RevokeRole call hub.KickUser
//     directly without going through this HTTP endpoint
//
// Returns the number of connections kicked. 200 with `{"kicked": N}` even if
// the user has zero open connections (idempotent).
func AdminSSEKickHandler(hub *events.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Defensive: enforce role here even though the registered route is
		// admin-only via the JWT middleware path check + RBAC gate. If
		// someone misconfigures the mux this prevents privilege escalation.
		claims := auth.GetClaims(r.Context())
		if claims == nil || !hasRole(claims.Roles, "ADMIN") {
			http.Error(w, `{"code":"permission_denied"}`, http.StatusForbidden)
			return
		}

		userSub := r.URL.Query().Get("user")
		if userSub == "" {
			http.Error(w, `{"code":"invalid_argument","message":"user query param required"}`, http.StatusBadRequest)
			return
		}

		kicked := hub.KickUser(userSub)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"kicked": kicked,
			"user":   userSub,
		})
	}
}

func hasRole(roles []string, want string) bool {
	for _, r := range roles {
		if r == want {
			return true
		}
	}
	return false
}
