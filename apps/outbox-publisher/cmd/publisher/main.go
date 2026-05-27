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
	nc, err := nats.Connect(natsURL, nats.Name("simaops-outbox-publisher"))
	if err != nil {
		slog.Error("failed to connect to NATS", "err", err)
		os.Exit(1)
	}
	defer nc.Close()

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
				pollLoop(leaderCtx, db, js)
			},
			OnStoppedLeading: func() {
				slog.Info("lost leadership", "identity", identity)
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
			publishPending(ctx, db, js)
		}
	}
}

func publishPending(ctx context.Context, db *sql.DB, js jetstream.JetStream) {
	rows, err := db.QueryContext(ctx,
		"SELECT id, event_type, payload_json, retry_count FROM outbox_events WHERE status = 'PENDING' ORDER BY created_at ASC LIMIT 100")
	if err != nil {
		slog.Error("poll query failed", "err", err)
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
			_, _ = db.ExecContext(ctx,
				"UPDATE outbox_events SET status = 'FAILED' WHERE id = ?", p.id)
			slog.Warn("event marked FAILED after max retries", "id", p.id, "retry_count", p.retryCount)
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

		_, err := js.PublishMsg(pubCtx, msg, jetstream.WithMsgID(p.id))
		if err != nil {
			slog.Warn("publish failed, will retry", "id", p.id, "err", err)
			_, _ = db.ExecContext(pubCtx,
				"UPDATE outbox_events SET retry_count = retry_count + 1 WHERE id = ?", p.id)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			continue
		}

		_, err = db.ExecContext(pubCtx,
			"UPDATE outbox_events SET status = 'PUBLISHED', published_at = CURRENT_TIMESTAMP WHERE id = ?", p.id)
		if err != nil {
			slog.Error("failed to mark published", "id", p.id, "err", err)
			span.RecordError(err)
		} else {
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

// startHealthServer exposes /healthz on port 8082 for Kubernetes probes.
// /healthz returns 200 if the process is alive (not necessarily the leader).
func startHealthServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

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
