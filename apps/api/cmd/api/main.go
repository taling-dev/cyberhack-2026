package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/handler"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/middleware"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

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

	// Health endpoints
	mux.HandleFunc("GET /healthz", handler.Healthz)
	mux.HandleFunc("GET /readyz", handler.Readyz)

	// Connect RPC handlers
	minioClient, err := storage.NewMinIOClient()
	if err != nil {
		slog.Warn("MinIO client init failed (non-fatal for dev)", "err", err)
	}
	handler.RegisterConnectHandlers(mux, dbConn, minioClient)

	// Wrap with middleware
	h := middleware.RequestID(middleware.Logger(logger, middleware.CORS(mux)))

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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
