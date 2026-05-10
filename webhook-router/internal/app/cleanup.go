package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/chen-zhanjie/webhook-router/internal/config"
	"github.com/chen-zhanjie/webhook-router/internal/store"
)

func startStreamCleanup(ctx context.Context, redisStore *store.RedisStore, reg *config.Registry, logger *slog.Logger) {
	interval := reg.Config.Cache.TTL.Duration / 2
	if interval < time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cutoff := time.Now().Add(-reg.Config.Cache.TTL.Duration)
				for appID := range reg.Apps {
					if err := redisStore.TrimBefore(ctx, appID, cutoff); err != nil {
						logger.Warn("trim app stream failed", "app", appID, "error", err)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
