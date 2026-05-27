package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

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
}

type ctxKey string

const ClaimsKey ctxKey = "auth_claims"

// GetClaims extracts auth claims from context (set by JWTMiddleware).
func GetClaims(ctx context.Context) *Claims {
	if c, ok := ctx.Value(ClaimsKey).(*Claims); ok {
		return c
	}
	return nil
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

// Wrap returns middleware that verifies JWT and injects claims into context.
// Endpoints not requiring auth (health/metrics) bypass the middleware via the RBAC table.
func (m *JWTMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Do(m.init)

		// Skip auth for health/metrics paths — they are gated by lack of /simaops. prefix in RBAC.
		if !strings.Contains(r.URL.Path, "/simaops.") {
			next.ServeHTTP(w, r)
			return
		}

		// Fail-closed: if JWKS init failed, reject all RPC requests with 503.
		if m.initErr != nil {
			http.Error(w, `{"code":"unavailable","message":"identity provider unreachable"}`, http.StatusServiceUnavailable)
			return
		}

		// Extract bearer token. RBAC handles the case of missing/invalid token (returns 401).
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			next.ServeHTTP(w, r) // RBAC will reject with 401
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		// Cryptographically verify the token. NEVER fall back to parseUnverified.
		token, err := jwt.Parse(tokenStr, m.jwks.KeyfuncCtx(r.Context()),
			jwt.WithIssuer(m.issuer),
			jwt.WithExpirationRequired(),
			jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "ES256", "ES384"}),
		)
		if err != nil {
			http.Error(w, `{"code":"unauthenticated","message":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		mapClaims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			http.Error(w, `{"code":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}

		claims := extractClaims(mapClaims)
		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func extractClaims(m jwt.MapClaims) *Claims {
	c := &Claims{
		Sub:      getString(m, "sub"),
		Username: getString(m, "preferred_username"),
		Email:    getString(m, "email"),
		Name:     getString(m, "name"),
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
