package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CORS adds CORS headers using an explicit origin allowlist from CORS_ALLOWED_ORIGINS.
// Setting Access-Control-Allow-Origin to a reflected Origin while sending
// Access-Control-Allow-Credentials: true is a CSRF vulnerability — instead, we only
// echo back origins that match the configured allowlist.
//
// The ingress-nginx layer also enforces CORS for the public hostname; this middleware
// is a defense-in-depth layer for in-cluster traffic (e.g., dev with port-forwarding).
func CORS(next http.Handler) http.Handler {
	allowed := parseAllowedOrigins()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && isOriginAllowed(origin, allowed) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms, Authorization, X-Request-Id, Idempotency-Key")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func parseAllowedOrigins() []string {
	v := os.Getenv("CORS_ALLOWED_ORIGINS")
	if v == "" {
		// Default: allow common dev origins. Production must set this env var.
		return []string{
			"http://localhost:5173",
			"http://localhost:3000",
			"https://app.161.118.244.229.sslip.io",
			"http://app.161.118.244.229.sslip.io",
		}
	}
	out := strings.Split(v, ",")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

func isOriginAllowed(origin string, allowed []string) bool {
	for _, a := range allowed {
		if a == origin {
			return true
		}
	}
	return false
}
