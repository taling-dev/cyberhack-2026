package middleware

import (
	"net/http"
)

// MaxBytes limits incoming request body size to prevent DoS via large payloads.
// 10 MiB is generous for RPC + image upload metadata. Image uploads themselves
// go directly to MinIO via presigned URLs and don't pass through the API.
const MaxRequestBytes = 10 * 1024 * 1024

// BodyLimit caps incoming request body size at MaxRequestBytes.
func BodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
		next.ServeHTTP(w, r)
	})
}
