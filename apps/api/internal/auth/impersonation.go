package auth

import (
	"context"
	"net/http"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
)

// ImpersonationKey carries the real admin's username when a request is being
// impersonated, so the audit layer can attribute "X acting as Y".
const ImpersonationKey ctxKey = "auth_impersonator"

// GetImpersonator returns the real admin username when the request is
// impersonated, or "" otherwise.
func GetImpersonator(ctx context.Context) string {
	if v, ok := ctx.Value(ImpersonationKey).(string); ok {
		return v
	}
	return ""
}

// ImpersonationMiddleware lets an ADMIN act as another user. When the
// REAL verified token carries ADMIN and an `X-Impersonate: <username>` header
// is present, the request's effective claims are swapped to the target user
// (identity + their DB roles). Any other case is ignored — a non-admin can
// never impersonate, and the header is meaningless without a valid ADMIN JWT.
//
// Must be mounted AFTER JWTMiddleware (needs verified claims) and BEFORE RBAC
// (so role checks use the effective user).
func ImpersonationMiddleware(q *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := r.Header.Get("X-Impersonate")
			if target == "" {
				next.ServeHTTP(w, r)
				return
			}
			real := GetClaims(r.Context())
			// Only a verified ADMIN may impersonate. Fail silently (ignore the
			// header) for everyone else — never trust it without ADMIN.
			isAdmin := false
			if real != nil {
				for _, role := range real.Roles {
					if role == "ADMIN" {
						isAdmin = true
						break
					}
				}
			}
			if !isAdmin {
				next.ServeHTTP(w, r)
				return
			}
			// Never impersonate self / no-op.
			if target == real.Username {
				next.ServeHTTP(w, r)
				return
			}
			u, err := q.GetUserByUsername(r.Context(), target)
			if err != nil {
				next.ServeHTTP(w, r) // unknown target: ignore, act as the admin
				return
			}
			roleNames, _ := q.ListUserRoleNames(r.Context(), u.ID)

			eff := &Claims{
				Sub:      u.ID,
				Username: u.Username,
				Email:    u.Email,
				Name:     u.FullName,
				Roles:    roleNames,
				Exp:      real.Exp,
			}
			ctx := context.WithValue(r.Context(), ClaimsKey, eff)
			ctx = context.WithValue(ctx, ImpersonationKey, real.Username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
