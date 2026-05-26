package handler

import (
	"context"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/auth"
)

// userFromCtx returns the authenticated user identifier from JWT claims.
// Falls back to "system" only when claims are absent (should not happen in production).
func userFromCtx(ctx context.Context) string {
	if c := auth.GetClaims(ctx); c != nil {
		if c.Username != "" {
			return c.Username
		}
		if c.Sub != "" {
			return c.Sub
		}
	}
	return "system"
}

// roleFromCtx returns the primary role of the authenticated user.
func roleFromCtx(ctx context.Context) string {
	if c := auth.GetClaims(ctx); c != nil && len(c.Roles) > 0 {
		// Pick highest-privilege role for audit display
		priority := []string{"ADMIN", "MANAGER", "QC_SUPERVISOR", "WAREHOUSE_STAFF", "OPERATOR"}
		for _, p := range priority {
			for _, r := range c.Roles {
				if r == p {
					return r
				}
			}
		}
		return c.Roles[0]
	}
	return "system"
}
