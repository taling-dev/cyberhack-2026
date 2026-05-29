package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// Claims extracted from the Keycloak JWT.
type Claims struct {
	Sub      string   `json:"sub"`
	Username string   `json:"preferred_username"`
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Roles    []string // extracted from realm_access.roles
	Exp      int64    // raw `exp` claim (unix seconds), used by SSE handler
}

type ctxKey string

const (
	ClaimsKey ctxKey = "auth_claims"
	// AccessTokenExpKey carries the parsed `exp` claim (unix seconds) for any
	// downstream handler (specifically the SSE handler) that needs the
	// token's lifetime for the connection-info hint.
	AccessTokenExpKey ctxKey = "auth_access_token_exp"
)

// Auth-failure reason constants mirrored to the X-Auth-Failure-Reason
// response header. The web client uses these to pick the correct recovery
// tier:
//   - expired  → try /auth/heartbeat?force=true (Tier 0)
//   - revoked  → silent OIDC renew (Tier 1)
//   - missing  → silent OIDC renew (Tier 1)
//   - invalid  → popup re-login (Tier 2) — token is corrupt, refresh won't help
const (
	AuthFailReasonExpired = "expired"
	AuthFailReasonInvalid = "invalid"
	AuthFailReasonRevoked = "revoked"
	AuthFailReasonMissing = "missing"
)

// JWTLeeway is the symmetric tolerance applied to `exp`/`nbf` validation so
// brief clock skew between the API pod and Keycloak doesn't spuriously reject
// otherwise-valid tokens. MUST be a typed time.Duration — `jwt.WithLeeway`
// accepts time.Duration, and an untyped int constant `60` would be silently
// converted to 60ns, defeating the whole purpose of leeway.
//
// Exported so the SSE handler can use the same window when scheduling its
// forced-disconnect timer (otherwise the SSE expiry timer can outlive the
// JWT validity window or under-shoot it).
const JWTLeeway = 60 * time.Second

// GetClaims extracts auth claims from context (set by JWTMiddleware).
func GetClaims(ctx context.Context) *Claims {
	if c, ok := ctx.Value(ClaimsKey).(*Claims); ok {
		return c
	}
	return nil
}

// GetAccessTokenExp returns the `exp` claim (unix seconds) of the request's
// access token, or 0 if not authenticated.
func GetAccessTokenExp(ctx context.Context) int64 {
	if v, ok := ctx.Value(AccessTokenExpKey).(int64); ok {
		return v
	}
	return 0
}

// JWTMiddleware verifies the Authorization: Bearer <token> header against Keycloak JWKS.
// SECURITY: This middleware FAILS CLOSED — if JWKS cannot be fetched or a token cannot
// be cryptographically verified, requests are rejected. There is no fallback to
// unverified token parsing in any mode.
type JWTMiddleware struct {
	jwks    keyfunc.Keyfunc
	issuer  string
	jwksURL string
	mu      sync.Once
	initErr error
}

func NewJWTMiddleware() *JWTMiddleware {
	issuer := getEnv("KEYCLOAK_ISSUER", "http://localhost:8080/realms/simaops")
	jwksBase := getEnv("KEYCLOAK_INTERNAL_URL", issuer)
	return &JWTMiddleware{
		issuer:  issuer,
		jwksURL: jwksBase + "/protocol/openid-connect/certs",
	}
}

func (m *JWTMiddleware) init() {
	k, err := keyfunc.NewDefault([]string{m.jwksURL})
	if err != nil {
		m.initErr = fmt.Errorf("init JWKS from %s: %w", m.jwksURL, err)
		slog.Error("JWKS init failed", "url", m.jwksURL, "err", err)
		return
	}
	m.jwks = k
	slog.Info("JWKS initialized", "url", m.jwksURL, "issuer", m.issuer)
}

// writeAuthError responds with a 401 carrying both the standard
// WWW-Authenticate header and the SimaOps-specific X-Auth-Failure-Reason so
// the web client can choose the right recovery flow.
func writeAuthError(w http.ResponseWriter, reason string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="invalid_token", error_description=%q`, reason))
	w.Header().Set("X-Auth-Failure-Reason", reason)
	http.Error(w, fmt.Sprintf(`{"code":"unauthenticated","reason":%q}`, reason), http.StatusUnauthorized)
}

// requiresAuth returns true for paths that must be authenticated. The /events
// endpoint is added here in addition to the existing /simaops. RPC paths so
// SSE connections without a Bearer token are rejected with 401 + reason.
func requiresAuth(path string) bool {
	if strings.Contains(path, "/simaops.") {
		return true
	}
	if path == "/events" {
		return true
	}
	if strings.HasPrefix(path, "/admin/") {
		return true
	}
	return false
}

// Wrap returns middleware that verifies JWT and injects claims into context.
func (m *JWTMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Do(m.init)

		// Skip auth for paths that don't require it (health/metrics/etc).
		if !requiresAuth(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Fail-closed: if JWKS init failed, reject all auth-required requests with 503.
		if m.initErr != nil {
			http.Error(w, `{"code":"unavailable","message":"identity provider unreachable"}`, http.StatusServiceUnavailable)
			return
		}

		// Extract bearer token. Missing token → structured 401 missing.
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			// For RPC paths the RBAC layer handles role checks; we only
			// short-circuit on the SSE path which has no RBAC layer below.
			if r.URL.Path == "/events" {
				writeAuthError(w, AuthFailReasonMissing)
				return
			}
			next.ServeHTTP(w, r) // RBAC will reject with 401 for RPCs
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		// Cryptographically verify the token. Apply leeway so up to 60s of
		// clock skew between Keycloak and this API pod doesn't reject otherwise
		// valid tokens.
		token, err := jwt.Parse(tokenStr, m.jwks.KeyfuncCtx(r.Context()),
			jwt.WithIssuer(m.issuer),
			jwt.WithExpirationRequired(),
			jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "ES256", "ES384"}),
			jwt.WithLeeway(JWTLeeway),
		)
		if err != nil {
			reason := classifyTokenError(err)
			writeAuthError(w, reason)
			return
		}

		mapClaims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			writeAuthError(w, AuthFailReasonInvalid)
			return
		}

		claims := extractClaims(mapClaims)
		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		if claims.Exp > 0 {
			ctx = context.WithValue(ctx, AccessTokenExpKey, claims.Exp)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// classifyTokenError maps a jwt.Parse error to a stable failure reason. We
// only treat ErrTokenExpired as recoverable via refresh; everything else is
// either signature/format trouble or a genuine revocation.
func classifyTokenError(err error) string {
	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return AuthFailReasonExpired
	case errors.Is(err, jwt.ErrTokenNotValidYet):
		// Token issued slightly in the future relative to our clock — treat
		// as transient/expired so the client retries via Tier 0 refresh.
		return AuthFailReasonExpired
	case errors.Is(err, jwt.ErrTokenSignatureInvalid),
		errors.Is(err, jwt.ErrTokenMalformed),
		errors.Is(err, jwt.ErrTokenInvalidIssuer),
		errors.Is(err, jwt.ErrTokenInvalidAudience):
		return AuthFailReasonInvalid
	default:
		return AuthFailReasonInvalid
	}
}

func extractClaims(m jwt.MapClaims) *Claims {
	c := &Claims{
		Sub:      getString(m, "sub"),
		Username: getString(m, "preferred_username"),
		Email:    getString(m, "email"),
		Name:     getString(m, "name"),
	}
	// `exp` is a numeric date — float seconds in JSON.
	if expFloat, ok := m["exp"].(float64); ok {
		c.Exp = int64(expFloat)
	}
	if ra, ok := m["realm_access"].(map[string]interface{}); ok {
		if roles, ok := ra["roles"].([]interface{}); ok {
			for _, r := range roles {
				if s, ok := r.(string); ok {
					c.Roles = append(c.Roles, s)
				}
			}
		}
	}
	return c
}

func getString(m jwt.MapClaims, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
