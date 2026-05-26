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
type JWTMiddleware struct {
	jwks     keyfunc.Keyfunc
	issuer   string
	audience string
	mu       sync.Once
	initErr  error
}

func NewJWTMiddleware() *JWTMiddleware {
	return &JWTMiddleware{
		issuer:   getEnv("KEYCLOAK_ISSUER", "http://localhost:8080/realms/simaops"),
		audience: getEnv("KEYCLOAK_CLIENT_ID", "simaops-web"),
	}
}

func (m *JWTMiddleware) init() {
	jwksURL := m.issuer + "/protocol/openid-connect/certs"
	k, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		m.initErr = fmt.Errorf("failed to init JWKS from %s: %w", jwksURL, err)
		slog.Warn("JWKS init failed (auth disabled)", "err", m.initErr)
		return
	}
	m.jwks = k
}

// Wrap returns middleware that verifies JWT and injects claims into context.
// If JWKS is unavailable (dev mode), it passes through with a dev claims fallback.
func (m *JWTMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Do(m.init)

		// Extract bearer token
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			// No token — in DEMO_MODE, inject a default operator user
			if os.Getenv("DEMO_MODE") == "true" {
				ctx := context.WithValue(r.Context(), ClaimsKey, &Claims{
					Sub:      "u-operator",
					Username: "operator",
					Email:    "operator@simaops.local",
					Name:     "Budi Operator (Demo)",
					Roles:    []string{"OPERATOR", "QC_SUPERVISOR", "WAREHOUSE_STAFF", "MANAGER", "ADMIN"},
				})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			// No token and no demo mode — allow through with nil claims (RBAC will block protected RPCs)
			next.ServeHTTP(w, r)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		// If JWKS failed to init, use dev fallback (parse without verification)
		if m.initErr != nil {
			claims := parseUnverified(tokenStr)
			if claims != nil {
				ctx := context.WithValue(r.Context(), ClaimsKey, claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Verify token
		token, err := jwt.Parse(tokenStr, m.jwks.KeyfuncCtx(r.Context()),
			jwt.WithIssuer(m.issuer),
			jwt.WithAudience(m.audience),
			jwt.WithExpirationRequired(),
		)
		if err != nil {
			http.Error(w, `{"code":"unauthenticated","message":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		mapClaims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
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
	// Extract roles from realm_access.roles
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

func parseUnverified(tokenStr string) *Claims {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return nil
	}
	if m, ok := token.Claims.(jwt.MapClaims); ok {
		return extractClaims(m)
	}
	return nil
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
