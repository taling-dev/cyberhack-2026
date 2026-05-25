package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Database
	dsn := getEnv("TIDB_DSN", "root:@tcp(localhost:4000)/simaops?parseTime=true")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		slog.Error("failed to open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// NATS JetStream
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")
	nc, err := nats.Connect(natsURL)
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

	// Ensure stream exists
	ctx := context.Background()
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     "SIMAOPS",
		Subjects: []string{"qc.>", "lot.>", "warehouse.>", "audit.>"},
		Storage:  jetstream.FileStorage,
	})
	if err != nil {
		slog.Error("failed to create stream", "err", err)
		os.Exit(1)
	}

	slog.Info("outbox-publisher started", "nats", natsURL)

	// Poll loop (leader election via Kubernetes Lease deferred to Helm chart — for now single instance)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go pollLoop(ctx, db, js)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down")
	cancel()
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
		"SELECT id, event_type, payload_json FROM outbox_events WHERE status = 'PENDING' ORDER BY created_at ASC LIMIT 100")
	if err != nil {
		slog.Error("poll query failed", "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, eventType string
		var payloadJSON json.RawMessage
		if err := rows.Scan(&id, &eventType, &payloadJSON); err != nil {
			continue
		}

		// Publish to NATS with Msg-Id for deduplication
		_, err := js.Publish(ctx, eventType, payloadJSON,
			jetstream.WithMsgID(id),
		)
		if err != nil {
			slog.Warn("publish failed, will retry", "id", id, "err", err)
			db.ExecContext(ctx, "UPDATE outbox_events SET retry_count = retry_count + 1 WHERE id = ?", id)
			continue
		}

		// Mark as published
		_, err = db.ExecContext(ctx,
			"UPDATE outbox_events SET status = 'PUBLISHED', published_at = CURRENT_TIMESTAMP WHERE id = ?", id)
		if err != nil {
			slog.Error("failed to mark published", "id", id, "err", err)
		} else {
			slog.Info("published", "id", id, "subject", eventType)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
