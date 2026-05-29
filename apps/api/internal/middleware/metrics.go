package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "simaops",
			Subsystem: "api",
			Name:      "http_request_duration_seconds",
			Help:      "Duration of HTTP requests in seconds",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"rpc", "code"},
	)

	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "simaops",
			Subsystem: "api",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"rpc", "code"},
	)

	idempotencyHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "simaops",
			Subsystem: "api",
			Name:      "idempotency_total",
			Help:      "Idempotency middleware outcomes",
		},
		[]string{"outcome"}, // hit | miss | conflict
	)

	lotsCreatedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "simaops",
			Subsystem: "api",
			Name:      "lots_created_total",
			Help:      "Total lots created",
		},
	)

	qcReviewedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "simaops",
			Subsystem: "api",
			Name:      "qc_reviewed_total",
			Help:      "Total QC reviews by decision",
		},
		[]string{"decision"}, // approved | rejected | recheck
	)

	warehouseAssignmentsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "simaops",
			Subsystem: "api",
			Name:      "warehouse_assignments_total",
			Help:      "Total warehouse slot assignments",
		},
	)

	// lotsByStatus — gauge of current lot count per status. Updated by a
	// background goroutine in main.go (StartLotsByStatusUpdater). Used by
	// the SimaopsLotsStuckInQCReview and SimaopsLotsStuckInAIProcessing
	// PrometheusRule alerts.
	lotsByStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "simaops",
			Subsystem: "api",
			Name:      "lots_by_status",
			Help:      "Current number of lots per status (sampled every 30s).",
		},
		[]string{"status"},
	)
)

// SetLotsByStatus sets the gauge for one status label. Called by the
// background updater that polls CountLotsByStatusGroup. Status names
// match the LotsStatus enum (e.g. "AWAITING_REVIEW", "AI_PROCESSING").
func SetLotsByStatus(status string, count float64) {
	lotsByStatus.WithLabelValues(status).Set(count)
}

// IncIdempotencyHit increments the idempotency hit counter (called from idempotency middleware).
func IncIdempotencyHit(outcome string) {
	idempotencyHits.WithLabelValues(outcome).Inc()
}

// IncLotCreated, IncQCReviewed, IncWarehouseAssignment — exported counters for handlers.
func IncLotCreated()                            { lotsCreatedTotal.Inc() }
func IncQCReviewed(decision string)             { qcReviewedTotal.WithLabelValues(decision).Inc() }
func IncWarehouseAssignment()                   { warehouseAssignmentsTotal.Inc() }

// statusRecorder captures the status code written to the response.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.wrote = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.status = http.StatusOK
		r.wrote = true
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap allows http.NewResponseController to reach the underlying writer
// for SSE flushing.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Metrics middleware records request duration and counts for Connect RPC paths.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip /metrics, /healthz, /readyz themselves
		if r.URL.Path == "/metrics" || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		duration := time.Since(start).Seconds()

		// Extract RPC name from Connect path: /simaops.lot.v1.LotService/CreateLot → CreateLot
		rpc := r.URL.Path
		if idx := strings.LastIndex(rpc, "/"); idx >= 0 {
			rpc = rpc[idx+1:]
		}

		code := strconv.Itoa(rec.status)
		httpRequestDuration.WithLabelValues(rpc, code).Observe(duration)
		httpRequestsTotal.WithLabelValues(rpc, code).Inc()
	})
}
