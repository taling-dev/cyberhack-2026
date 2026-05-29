package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

const (
	streamName = "SIMAOPS"
	maxRetries = 10
	leaseName  = "simaops-outbox-publisher-leader"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx := context.Background()
	shutdown := initOTel(ctx, "simaops-outbox-publisher")
	defer shutdown()

	dsn := getEnv("TIDB_DSN", "root:@tcp(localhost:4000)/simaops?parseTime=true")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		slog.Error("failed to open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	natsURL := getEnv("NATS_URL", "nats://localhost:4222")
	// Reconnect options: NATS goes through brief blips during cluster
	// rolling restarts. -1 = retry forever, 2s spacing. The handlers below
	// log a structured event on each transition so an operator (or a NATS
	// reconnect alert) can see what happened.
	nc, err := nats.Connect(natsURL,
		nats.Name("simaops-outbox-publisher"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.Warn("nats disconnected", "err", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			slog.Info("nats reconnected", "url", c.ConnectedUrl())
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			slog.Error("nats connection permanently closed — exiting")
			os.Exit(1) // let the deployment restart us
		}),
	)
	if err != nil {
		slog.Error("failed to connect to NATS", "err", err)
		os.Exit(1)
	}
	// Drain (not Close) so any in-flight publishes are awaited on shutdown.
	defer func() {
		if err := nc.Drain(); err != nil {
			slog.Warn("nats drain error", "err", err)
		}
	}()

	js, err := jetstream.New(nc)
	if err != nil {
		slog.Error("failed to create JetStream context", "err", err)
		os.Exit(1)
	}

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     streamName,
		Subjects: []string{"qc.>", "lot.>", "warehouse.>", "audit.>"},
		Storage:  jetstream.FileStorage,
		MaxAge:   7 * 24 * time.Hour,
		// 1 GiB cap — at our event rate (~tens of events/sec peak, ~200 bytes
		// each) one week of retention fits in tens of MiB. The cap protects
		// against unbounded disk usage if a future bug emits high-frequency
		// events. NATS will drop oldest first when the limit is reached.
		MaxBytes: 1 * 1024 * 1024 * 1024,
		// Replicas defaults to 1 (current cluster has a single nats-0 pod).
		// When we scale NATS to 3 nodes for production HA, set Replicas: 3
		// here and the stream will be migrated automatically by JetStream.
	})
	if err != nil {
		slog.Error("failed to create stream", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		slog.Info("shutdown signal received")
		cancel()
	}()

	// Try leader election in cluster; fall back to singleton outside cluster
	if err := runWithLeaderElection(ctx, db, js); err != nil {
		slog.Error("leader election error", "err", err)
		os.Exit(1)
	}
}

func runWithLeaderElection(ctx context.Context, db *sql.DB, js jetstream.JetStream) error {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		slog.Info("not running in cluster, using singleton mode (no leader election)")
		pollLoop(ctx, db, js)
		return nil
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}

	identity := os.Getenv("POD_NAME")
	if identity == "" {
		identity = "outbox-publisher-" + uuid.NewString()[:8]
	}
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "simaops"
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: namespace,
		},
		Client: clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: identity,
		},
	}

	slog.Info("starting leader election",
		"lease", leaseName,
		"namespace", namespace,
		"identity", identity,
	)

	// Health server — separate goroutine, reports leadership status.
	go startHealthServer(ctx)

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   15 * time.Second,
		RenewDeadline:   10 * time.Second,
		RetryPeriod:     2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(leaderCtx context.Context) {
				slog.Info("became leader, starting poll loop", "identity", identity)
				outboxIsLeader.Set(1)
				// Recovery: any rows left in PUBLISHING by a prior crashed
				// leader belong to no one now. Reset them to PENDING so we
				// re-claim and re-publish (NATS Nats-Msg-Id dedup keeps
				// stream consumers from seeing duplicates).
				if res, err := db.ExecContext(leaderCtx,
					"UPDATE outbox_events SET status = 'PENDING' WHERE status = 'PUBLISHING'"); err != nil {
					slog.Warn("startup reset of stuck PUBLISHING rows failed", "err", err)
				} else if n, _ := res.RowsAffected(); n > 0 {
					slog.Info("recovered stuck PUBLISHING rows", "count", n)
				}
				pollLoop(leaderCtx, db, js)
			},
			OnStoppedLeading: func() {
				slog.Info("lost leadership", "identity", identity)
				outboxIsLeader.Set(0)
			},
			OnNewLeader: func(leader string) {
				if leader != identity {
					slog.Info("observed new leader", "leader", leader, "self", identity)
				}
			},
		},
	})
	return nil
}

func pollLoop(ctx context.Context, db *sql.DB, js jetstream.JetStream) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			outboxPollCyclesTotal.Inc()
			updateBacklogGauge(ctx, db)
			publishPending(ctx, db, js)
		}
	}
}

// updateBacklogGauge samples the outbox table once per poll for the
// `simaops_outbox_backlog_size` gauge. This drives the OutboxBacklog alert.
func updateBacklogGauge(ctx context.Context, db *sql.DB) {
	var n float64
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM outbox_events WHERE status = 'PENDING'").Scan(&n); err != nil {
		// On error, leave gauge at last known value rather than zeroing.
		return
	}
	outboxBacklogSize.Set(n)
}

// publishPending implements the two-phase claim pattern:
//
//   Phase 1 (claim):  UPDATE … SET status='PUBLISHING' WHERE status='PENDING'
//                     ORDER BY created_at LIMIT N. This is atomic in the DB
//                     and gives us exclusive ownership of a batch — even if
//                     leader election races during a handoff, only one
//                     publisher can hold the PUBLISHING rows at any time.
//
//   Phase 2 (publish): for each claimed row, publish to NATS with
//                     Nats-Msg-Id deduplication, then transition
//                     PUBLISHING → PUBLISHED on success or PUBLISHING →
//                     PENDING (with retry_count++) on transient failure or
//                     PUBLISHING → FAILED if the retry budget is exhausted.
//
//   Recovery:         on publisher startup, ResetStuckPublishingEvents is
//                     called once before pollLoop begins. Any rows left in
//                     PUBLISHING by a crashed prior leader are returned to
//                     PENDING so the new leader re-publishes them.
//
// The previous one-phase pattern (`SELECT … WHERE status='PENDING'` then
// publish, then `UPDATE PUBLISHED`) was vulnerable to leader handoffs: a
// failover after publish-but-before-mark would let the next leader re-publish
// the same row. NATS Nats-Msg-Id absorbs the duplicate in the JetStream
// stream layer, but core NATS subscribers (the SSE hub) still receive a
// duplicate. With the claim phase, only one leader at a time can own the
// row — so a failover immediately after publish leaves the row in
// PUBLISHING, and the new leader's reset moves it to PENDING for re-publish.
func publishPending(ctx context.Context, db *sql.DB, js jetstream.JetStream) {
	// Phase 1: claim.
	const batchSize = 100
	res, err := db.ExecContext(ctx,
		"UPDATE outbox_events SET status = 'PUBLISHING' WHERE status = 'PENDING' ORDER BY created_at ASC LIMIT ?",
		batchSize,
	)
	if err != nil {
		slog.Error("claim phase failed", "err", err)
		return
	}
	claimed, _ := res.RowsAffected()
	if claimed == 0 {
		return // nothing to do this tick
	}

	// Phase 2: read the claimed batch.
	rows, err := db.QueryContext(ctx,
		"SELECT id, event_type, payload_json, retry_count FROM outbox_events WHERE status = 'PUBLISHING' ORDER BY created_at ASC LIMIT ?",
		batchSize,
	)
	if err != nil {
		slog.Error("read-claimed phase failed", "err", err)
		return
	}

	type pending struct {
		id, eventType string
		payload       json.RawMessage
		retryCount    int
	}
	var batch []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.eventType, &p.payload, &p.retryCount); err != nil {
			continue
		}
		batch = append(batch, p)
	}
	rows.Close()

	tracer := otel.Tracer("outbox-publisher")
	propagator := otel.GetTextMapPropagator()

	for _, p := range batch {
		if p.retryCount >= maxRetries {
			// Move PUBLISHING → FAILED. Bumping retry_count is informative
			// for operators tailing the table.
			_, _ = db.ExecContext(ctx,
				"UPDATE outbox_events SET status = 'FAILED', retry_count = retry_count + 1 WHERE id = ? AND status = 'PUBLISHING'", p.id)
			outboxEventsFailedTotal.WithLabelValues(p.eventType).Inc()
			slog.Warn("event marked FAILED after max retries",
				"id", p.id, "retry_count", p.retryCount)
			continue
		}

		pubCtx, span := tracer.Start(ctx, "outbox.publish", trace.WithAttributes(
			attribute.String("event.id", p.id),
			attribute.String("event.type", p.eventType),
			attribute.Int("event.retry_count", p.retryCount),
		))

		header := nats.Header{}
		propagator.Inject(pubCtx, propagation.HeaderCarrier(header))
		header.Set("Nats-Msg-Id", p.id)

		msg := &nats.Msg{
			Subject: p.eventType,
			Data:    p.payload,
			Header:  header,
		}

		publishStart := time.Now()
		_, err := js.PublishMsg(pubCtx, msg, jetstream.WithMsgID(p.id))
		outboxPublishDurationSeconds.WithLabelValues(p.eventType).Observe(time.Since(publishStart).Seconds())
		if err != nil {
			// Transient publish failure — release back to PENDING so the
			// next poll cycle retries. retry_count++ ensures we eventually
			// graduate to FAILED if NATS is permanently broken.
			slog.Warn("publish failed, releasing to PENDING for retry",
				"id", p.id, "err", err)
			_, _ = db.ExecContext(pubCtx,
				"UPDATE outbox_events SET status = 'PENDING', retry_count = retry_count + 1 WHERE id = ? AND status = 'PUBLISHING'", p.id)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			continue
		}

		// Successful publish — transition PUBLISHING → PUBLISHED.
		_, err = db.ExecContext(pubCtx,
			"UPDATE outbox_events SET status = 'PUBLISHED', published_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'PUBLISHING'", p.id)
		if err != nil {
			slog.Error("failed to mark published", "id", p.id, "err", err)
			span.RecordError(err)
		} else {
			outboxEventsPublishedTotal.WithLabelValues(p.eventType).Inc()
			slog.Info("published",
				"id", p.id,
				"subject", p.eventType,
				"trace_id", traceIDFromCtx(pubCtx),
			)
		}
		span.End()
	}
}

func initOTel(ctx context.Context, serviceName string) func() {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return func() {}
	}
	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		slog.Warn("OTel exporter init failed (tracing disabled)", "err", err)
		return func() {}
	}
	res, _ := resource.New(ctx, resource.WithAttributes(semconv.ServiceNameKey.String(serviceName)))
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp), sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return func() { _ = tp.Shutdown(context.Background()) }
}

func traceIDFromCtx(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ""
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// startHealthServer exposes /healthz and /metrics on port 8082.
//   /healthz — 200 if the process is alive (not necessarily the leader)
//   /metrics — Prometheus exposition for the simaops_outbox_* metric family
func startHealthServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{Addr: ":8082", Handler: mux, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("health server error", "err", err)
		}
	}()
	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}
