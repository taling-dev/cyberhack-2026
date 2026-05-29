package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/storage"
)

// Healthz is a liveness probe — the process is alive and responsive.
func Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ReadyzHandler returns a readiness handler that verifies dependencies are reachable.
// Returns 200 only when TiDB, MinIO are reachable. NATS is checked via the outbox
// publisher; Keycloak is checked at JWT init.
func ReadyzHandler(db *sql.DB, minioClient *storage.MinIOClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		checks := map[string]string{}
		ok := true

		// DB ping
		if db != nil {
			if err := db.PingContext(ctx); err != nil {
				checks["tidb"] = "fail: " + err.Error()
				ok = false
			} else {
				checks["tidb"] = "ok"
			}
		} else {
			checks["tidb"] = "missing"
			ok = false
		}

		// MinIO ping
		if minioClient != nil {
			if err := minioClient.Ping(ctx); err != nil {
				checks["minio"] = "fail: " + err.Error()
				ok = false
			} else {
				checks["minio"] = "ok"
			}
		} else {
			checks["minio"] = "missing"
			// MinIO is non-fatal for readiness — only QC upload requires it.
		}

		// Clock skew vs Keycloak. Fatal only after 3 consecutive failures
		// (>60s skew) — a single transient Keycloak hiccup shouldn't kick
		// us out of the load balancer.
		skewCheck, skewOK := CheckSkewForReadiness(ctx)
		checks["clock_skew"] = skewCheck
		if !skewOK {
			ok = false
		}

		w.Header().Set("Content-Type", "application/json")
		if ok {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": map[bool]string{true: "ready", false: "not_ready"}[ok],
			"checks": checks,
		})
	}
}

// Readyz is a placeholder maintained for backward compat — main.go now uses ReadyzHandler.
func Readyz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}
