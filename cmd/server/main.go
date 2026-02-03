package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Harshitk-cp/engram/internal/api"
	"github.com/Harshitk-cp/engram/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	if err := config.Load(); err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	dbURL := config.DatabaseURL()
	if dbURL == "" {
		logger.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Fatal("failed to ping database", zap.Error(err))
	}
	logger.Info("connected to database")

	app := api.NewApp(pool, logger)

	// Start background services
	app.Tuner.Start()
	app.Expirer.Start()
	app.Decay.Start()
	app.Consolidation.Start()

	addr := config.ServerAddr()
	srv := &http.Server{
		Addr:    addr,
		Handler: app.Router,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("server starting", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	<-quit
	logger.Info("shutting down server")

	// Stop background services
	app.Tuner.Stop()
	app.Expirer.Stop()
	app.Decay.Stop()
	app.Consolidation.Stop()

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Fatal("server forced to shutdown", zap.Error(err))
	}

	logger.Info("server stopped")
}
