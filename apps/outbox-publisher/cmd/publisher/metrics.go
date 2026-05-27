package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Outbox publisher metrics.
//
// Exported on /metrics on port 8082 (same as the health endpoint).
//
// All metrics are namespaced `simaops_outbox_*` for consistency with the API
// service (`simaops_api_*`) and AI worker (`simaops_ai_*`) namespaces.
var (
	// outboxEventsPublishedTotal — counter of events successfully published to NATS,
	// labeled by event subject (e.g. "qc.job.created"). Tracks throughput per stream.
	outboxEventsPublishedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "simaops",
		Subsystem: "outbox",
		Name:      "events_published_total",
		Help:      "Total events successfully published to NATS JetStream by the outbox publisher.",
	}, []string{"event_type"})

	// outboxEventsFailedTotal — counter of events that exhausted retries and were
	// marked FAILED, labeled by event subject. Should ideally stay flat. A non-zero
	// rate indicates a real problem (NATS down, malformed payload, etc.).
	outboxEventsFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "simaops",
		Subsystem: "outbox",
		Name:      "events_failed_total",
		Help:      "Total events that hit max retries and were marked FAILED.",
	}, []string{"event_type"})

	// outboxPublishDurationSeconds — histogram of NATS PublishMsg latency.
	// Helps detect NATS slowness or network jitter.
	outboxPublishDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "simaops",
		Subsystem: "outbox",
		Name:      "publish_duration_seconds",
		Help:      "NATS publish latency for outbox events.",
		Buckets:   prometheus.DefBuckets, // 5ms ... 10s
	}, []string{"event_type"})

	// outboxBacklogSize — gauge of currently-PENDING events in the outbox table.
	// Sampled once per poll cycle. Should normally be small (< 10). If it grows
	// unboundedly, the publisher is falling behind and PrometheusRule alert
	// `OutboxBacklog` will fire.
	outboxBacklogSize = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "simaops",
		Subsystem: "outbox",
		Name:      "backlog_size",
		Help:      "Number of PENDING outbox events currently waiting to publish.",
	})

	// outboxIsLeader — gauge: 1 if this pod currently holds the leader lease,
	// 0 otherwise. Sum across pods should always equal 1 in steady state.
	// Useful for alerting on lease lapse (no leader for >30s).
	outboxIsLeader = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "simaops",
		Subsystem: "outbox",
		Name:      "is_leader",
		Help:      "1 if this pod currently holds the leader lease, 0 otherwise.",
	})

	// outboxPollCyclesTotal — counter of poll loop iterations. Used to detect
	// stalled poll loops (rate should match the ticker frequency).
	outboxPollCyclesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "simaops",
		Subsystem: "outbox",
		Name:      "poll_cycles_total",
		Help:      "Total poll loop iterations since process start.",
	})
)
