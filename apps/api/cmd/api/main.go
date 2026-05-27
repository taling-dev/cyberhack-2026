package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/auth"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/events"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/handler"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/middleware"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/storage"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/telemetry"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// OpenTelemetry
	ctx := context.Background()
	shutdownTracer := telemetry.Init(ctx, "simaops-api")
	defer shutdownTracer()

	// Database connection
	dsn := os.Getenv("TIDB_DSN")
	if dsn == "" {
		dsn = "root:@tcp(localhost:4000)/simaops?parseTime=true"
	}
	dbConn, err := sql.Open("mysql", dsn)
	if err != nil {
		slog.Error("failed to open database", "err", err)
		os.Exit(1)
	}
	defer dbConn.Close()
	dbConn.SetMaxOpenConns(25)
	dbConn.SetMaxIdleConns(5)
	dbConn.SetConnMaxLifetime(5 * time.Minute)

	mux := http.NewServeMux()

	// Connect RPC handlers
	minioClient, err := storage.NewMinIOClient()
	if err != nil {
		slog.Warn("MinIO client init failed (non-fatal for dev)", "err", err)
	}

	// Health + metrics endpoints
	mux.HandleFunc("GET /healthz", handler.Healthz)
	mux.HandleFunc("GET /readyz", handler.ReadyzHandler(dbConn, minioClient))
	mux.Handle("GET /metrics", promhttp.Handler())

	// SSE event hub: connects to NATS, subscribes to qc.>, lot.>, warehouse.>,
	// audit.>, and fans messages out to all locally-connected SSE clients on
	// this pod. Per-pod fan-out keeps NATS state minimal across replicas.
	maxSSEPerUser := events.MaxConnsPerUser
	if v := os.Getenv("MAX_SSE_PER_USER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxSSEPerUser = n
		}
	}
	hub := events.NewHub(maxSSEPerUser)

	// NATS connection for the SSE hub. Best-effort: if NATS is unreachable at
	// startup we log and continue — the SSE endpoint will simply not deliver
	// events until NATS reconnects, but the rest of the API still works.
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}
	natsConn, err := nats.Connect(natsURL,
		nats.Name("simaops-api-sse"),
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		slog.Warn("NATS connect failed (SSE will be non-functional)", "err", err)
	}
	if natsConn != nil {
		if _, err := events.StartSubscriber(ctx, natsConn, hub); err != nil {
			slog.Warn("SSE subscriber failed to start", "err", err)
		}
	}

	mux.HandleFunc("GET /events", handler.EventsHandler(hub))
	mux.HandleFunc("POST /admin/sse/kick", handler.AdminSSEKickHandler(hub))

	handler.RegisterConnectHandlers(mux, dbConn, minioClient, hub)

	// Pre-listen clock check: bail out if our local clock is more than 30s
	// off Keycloak's. This catches catastrophically misconfigured nodes at
	// boot rather than letting them serve traffic with broken JWT validation.
	// Keycloak unreachable → log + continue (we can't tell if we're skewed,
	// so treat as observability-only at startup).
	if err := handler.StartupClockCheck(ctx); err != nil {
		slog.Error("startup clock check failed", "err", err)
		os.Exit(1)
	}

	// Background goroutine: every 30s, sample lot counts by status and update
	// the simaops_api_lots_by_status gauge. Drives the LotsStuckIn* alerts.
	go startLotsByStatusUpdater(dbConn)

	// Wrap with middleware (order: outer → inner)
	// RequestID → Logger → BodyLimit → CORS → Metrics → JWT → RBAC → Idempotency → Audit → handlers
	jwtMw := auth.NewJWTMiddleware()
	h := middleware.RequestID(
		middleware.Logger(logger,
			middleware.BodyLimit(
				middleware.CORS(
					middleware.Metrics(
						jwtMw.Wrap(
							auth.RBACMiddleware(
								middleware.Idempotency(dbConn,
									middleware.Audit(dbConn, mux),
								),
							),
						),
					),
				),
			),
		),
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      h,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("starting server", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")

	// Drain SSE clients first so they receive a server-draining frame with a
	// jittered retry value. Without this the load balancer would slam every
	// client into reconnect at the same instant.
	hub.DrainWithJitter(30 * time.Second)

	if natsConn != nil {
		if err := natsConn.Drain(); err != nil {
			slog.Warn("nats drain", "err", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// startLotsByStatusUpdater polls the lots table every 30s and updates the
// simaops_api_lots_by_status gauge. Runs for the lifetime of the process.
//
// Failures are logged but non-fatal — the gauge will simply hold its last
// value if the DB is briefly unavailable. This is more useful than zeroing
// the gauge during a transient outage (avoids spurious alert resolves).
func startLotsByStatusUpdater(db *sql.DB) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	tick := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rows, err := db.QueryContext(ctx,
			"SELECT status, COUNT(*) FROM lots GROUP BY status")
		if err != nil {
			slog.Warn("lots_by_status query failed", "err", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var status string
			var count float64
			if err := rows.Scan(&status, &count); err != nil {
				continue
			}
			middleware.SetLotsByStatus(status, count)
		}
	}

	tick() // immediate seed so the gauge isn't empty for 30s
	for range ticker.C {
		tick()
	}
}
