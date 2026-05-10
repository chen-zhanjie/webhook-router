package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/chen-zhanjie/webhook-router/internal/broker"
	"github.com/chen-zhanjie/webhook-router/internal/callback"
	"github.com/chen-zhanjie/webhook-router/internal/config"
	"github.com/chen-zhanjie/webhook-router/internal/server"
	"github.com/chen-zhanjie/webhook-router/internal/store"
)

func Run(configPath, version string) error {
	reg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := newLogger(reg.Config.Log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	redisStore := store.NewRedisStore(reg.Config.Redis)
	defer redisStore.Close()
	if err := redisStore.Ping(ctx); err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}

	b := broker.New(reg.Config.SSE.ConnectionBuffer)
	cbStats := callback.NewStats()
	callback.New(redisStore, reg, logger, cbStats).Start(ctx)
	startStreamCleanup(ctx, redisStore, reg, logger)

	srv := server.New(reg, redisStore, b, cbStats, logger)
	httpServer := &http.Server{
		Addr:         reg.Config.Server.Listen,
		Handler:      srv.Router(),
		ReadTimeout:  reg.Config.Server.ReadTimeout.Duration,
		WriteTimeout: reg.Config.Server.WriteTimeout.Duration,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("webhook-router started", "listen", reg.Config.Server.Listen, "version", version)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), reg.Config.Server.ShutdownTimeout.Duration)
		defer cancel()
		logger.Info("webhook-router shutting down")
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func newLogger(cfg config.LogConfig) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	if strings.ToLower(cfg.Format) == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
