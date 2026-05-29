package events

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for the SSE bridge. Names are stable and exposed via
// the standard /metrics endpoint, scraped by kube-prometheus-stack.
var (
	// SSEActiveConnections tracks the number of currently-connected SSE
	// clients, labeled by the client's primary role. Useful for capacity
	// planning and detecting reconnect storms.
	SSEActiveConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "simaops",
		Subsystem: "api",
		Name:      "sse_active_connections",
		Help:      "Number of currently-active SSE clients by primary role.",
	}, []string{"role"})

	// SSEEventsSentTotal counts events successfully written to a client's
	// SSE stream, labeled by NATS subject. The sum of this counter divided
	// by SSEActiveConnections approximates events-per-client/sec.
	SSEEventsSentTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "simaops",
		Subsystem: "api",
		Name:      "sse_events_sent_total",
		Help:      "Total events delivered to SSE clients, labeled by subject.",
	}, []string{"subject"})

	// SSEEventsDroppedTotal counts events not delivered for a per-client
	// reason: role_filter (subject not allowed for the user's roles),
	// owner_filter (operator's owner_user_id mismatch), or slow_client
	// (channel buffer was full so we closed the laggy connection).
	SSEEventsDroppedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "simaops",
		Subsystem: "api",
		Name:      "sse_events_dropped_total",
		Help:      "Total events dropped per-client by drop reason.",
	}, []string{"reason"})

	// SSEConnectionsEvictedTotal counts forced LRU evictions when a single
	// user exceeds MAX_SSE_PER_USER. A non-zero rate suggests a stuck client
	// or a user opening many tabs — operationally interesting but not an
	// error in itself.
	SSEConnectionsEvictedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "simaops",
		Subsystem: "api",
		Name:      "sse_connections_evicted_total",
		Help:      "Total SSE connections evicted by per-user LRU.",
	})

	// SSEDispatchPanicsTotal counts panics recovered inside the NATS message
	// handler. Should always be zero. A non-zero value indicates a bug — fire
	// the SSEDispatchPanics PrometheusRule alert.
	SSEDispatchPanicsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "simaops",
		Subsystem: "api",
		Name:      "sse_dispatch_panics_total",
		Help:      "Total panics recovered in the NATS message dispatcher.",
	})

	// SSEClientChannelDepth samples the per-client channel buffer depth at
	// dispatch time. p99 climbing toward 64 indicates we should grow the
	// buffer; consistently above 32 suggests slow consumers.
	SSEClientChannelDepth = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "simaops",
		Subsystem: "api",
		Name:      "sse_client_channel_depth",
		Help:      "Per-client SSE channel depth at dispatch time.",
		Buckets:   []float64{0, 1, 4, 16, 32, 56, 64},
	}, []string{"role"})

	// SSEHubDispatchDurationSeconds measures how long a single Dispatch call
	// takes (lock acquisition + per-client filter + send). Should stay sub-ms.
	SSEHubDispatchDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "simaops",
		Subsystem: "api",
		Name:      "sse_hub_dispatch_duration_seconds",
		Help:      "Time to fan out one event across all connected clients.",
		Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
	})

	// APIClockSkewSeconds reports current API↔Keycloak clock skew, sampled
	// each /readyz poll. Driven by handler/clock.go. Defined here so the
	// readiness handler can update it without depending on the hub.
	APIClockSkewSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "simaops",
		Subsystem: "api",
		Name:      "clock_skew_seconds",
		Help:      "Signed clock skew between this API pod and Keycloak (seconds).",
	})
)
