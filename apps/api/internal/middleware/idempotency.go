package middleware

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/auth"
)

// Mutation RPC paths that require idempotency protection.
var idempotentRPCs = map[string]bool{
	"/simaops.lot.v1.LotService/CreateLot":                          true,
	"/simaops.lot.v1.LotService/UpdateLotStatus":                    true,
	"/simaops.qc.v1.QCService/CreateQCUploadUrl":                    true,
	"/simaops.qc.v1.QCService/CreateQCJob":                          true,
	"/simaops.qc.v1.QCService/ReviewQC":                             true,
	"/simaops.qc.v1.QCService/RetryQCJob":                           true,
	"/simaops.warehouse.v1.WarehouseService/AssignSlot":             true,
	"/simaops.admin.v1.AdminService/AssignRole":                     true,
	"/simaops.admin.v1.AdminService/RevokeRole":                     true,
}

// Idempotency middleware checks for duplicate requests using the idempotency_keys table.
func Idempotency(dbConn *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !idempotentRPCs[path] {
			next.ServeHTTP(w, r)
			return
		}

		// Read the request body to extract idempotency_key and compute hash
		body, err := io.ReadAll(r.Body)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		// Extract idempotency_key from JSON body
		var reqBody struct {
			IdempotencyKey string `json:"idempotencyKey"`
		}
		json.Unmarshal(body, &reqBody)

		if reqBody.IdempotencyKey == "" {
			// No idempotency key — pass through
			next.ServeHTTP(w, r)
			return
		}

		// Get user ID from auth context
		userID := "anonymous"
		if claims := auth.GetClaims(r.Context()); claims != nil {
			userID = claims.Sub
		}

		// Hash: sha256(user_id + operation + idempotency_key)
		h := sha256.New()
		h.Write([]byte(userID))
		h.Write([]byte(path))
		h.Write([]byte(reqBody.IdempotencyKey))
		keyHash := hex.EncodeToString(h.Sum(nil))

		// Check if key exists
		var cachedResponse sql.NullString
		err = dbConn.QueryRowContext(r.Context(),
			"SELECT response_json FROM idempotency_keys WHERE key_hash = ? AND created_at > ?",
			keyHash, time.Now().Add(-24*time.Hour),
		).Scan(&cachedResponse)

		if err == nil && cachedResponse.Valid {
			// Return cached response
			IncIdempotencyHit("hit")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Idempotency-Replayed", "true")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(cachedResponse.String))
			return
		}

		// First request — capture the response
		IncIdempotencyHit("miss")
		rec := &responseRecorder{ResponseWriter: w, body: &bytes.Buffer{}, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		// Store response if successful (2xx)
		if rec.status >= 200 && rec.status < 300 {
			responseJSON := rec.body.String()
			_, _ = dbConn.ExecContext(r.Context(),
				"INSERT INTO idempotency_keys (key_hash, user_id, operation, response_json) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE key_hash=key_hash",
				keyHash, userID, path, responseJSON,
			)
		}
	})
}

type responseRecorder struct {
	http.ResponseWriter
	body   *bytes.Buffer
	status int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// Unwrap exposes the underlying writer for http.NewResponseController.
func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
