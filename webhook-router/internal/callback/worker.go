package callback

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/chen-zhanjie/webhook-router/internal/config"
	"github.com/chen-zhanjie/webhook-router/internal/event"
	"github.com/chen-zhanjie/webhook-router/internal/store"
)

type Stats struct {
	mu        sync.RWMutex
	Success   map[string]int64
	Failed    map[string]int64
	Permanent map[string]int64
}

func NewStats() *Stats {
	return &Stats{Success: map[string]int64{}, Failed: map[string]int64{}, Permanent: map[string]int64{}}
}

func (s *Stats) IncSuccess(appID string)   { s.mu.Lock(); s.Success[appID]++; s.mu.Unlock() }
func (s *Stats) IncFailed(appID string)    { s.mu.Lock(); s.Failed[appID]++; s.mu.Unlock() }
func (s *Stats) IncPermanent(appID string) { s.mu.Lock(); s.Permanent[appID]++; s.mu.Unlock() }

func (s *Stats) Snapshot() map[string]map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]map[string]int64{
		"success":          clone(s.Success),
		"failed":           clone(s.Failed),
		"permanent_failed": clone(s.Permanent),
	}
}

func clone(in map[string]int64) map[string]int64 {
	out := map[string]int64{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

type Worker struct {
	store *store.RedisStore
	reg   *config.Registry
	log   *slog.Logger
	stats *Stats
}

func New(store *store.RedisStore, reg *config.Registry, log *slog.Logger, stats *Stats) *Worker {
	return &Worker{store: store, reg: reg, log: log, stats: stats}
}

func (w *Worker) Start(ctx context.Context) {
	for _, app := range w.reg.Apps {
		if app.IsEnabled() && app.Delivery.Callback.Enabled {
			go w.runApp(ctx, app)
		}
	}
}

func (w *Worker) runApp(ctx context.Context, app config.App) {
	lastID, err := w.store.GetCallbackCursor(ctx, app.ID)
	if err != nil {
		w.log.Error("load callback cursor failed", "app", app.ID, "error", err)
	}
	if lastID == "" {
		lastID = "0-0"
	}
	client := &http.Client{Timeout: app.Delivery.Callback.Timeout.Duration}
	for ctx.Err() == nil {
		events, err := w.store.XRead(ctx, app.ID, lastID, 5*time.Second, 10)
		if err != nil {
			w.log.Error("read callback stream failed", "app", app.ID, "error", err)
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
			}
			continue
		}
		for _, e := range events {
			if err := w.deliver(ctx, client, app, e); err != nil {
				w.log.Error("callback permanently failed", "app", app.ID, "stream_id", e.ID, "source_id", e.SourceID, "error", err)
				w.stats.IncPermanent(app.ID)
			} else {
				w.stats.IncSuccess(app.ID)
			}
			lastID = e.ID
			if err := w.store.SetCallbackCursor(ctx, app.ID, lastID); err != nil {
				w.log.Error("save callback cursor failed", "app", app.ID, "stream_id", e.ID, "error", err)
			}
		}
	}
}

func (w *Worker) deliver(ctx context.Context, client *http.Client, app config.App, e event.WebhookEvent) error {
	body, err := event.Marshal(e)
	if err != nil {
		return err
	}
	maxAttempts := app.Delivery.Callback.MaxAttempts
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, app.Delivery.Callback.URL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if app.Delivery.Callback.Secret != "" {
			req.Header.Set("X-Relay-Callback-Secret", app.Delivery.Callback.Secret)
		}
		req.Header.Set("X-Relay-Source-ID", e.SourceID)
		req.Header.Set("X-Relay-Stream-ID", e.ID)
		req.Header.Set("X-Relay-App", app.ID)

		resp, err := client.Do(req)
		if err == nil && resp != nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			err = statusError(resp.StatusCode)
		}
		w.stats.IncFailed(app.ID)
		w.log.Warn("callback attempt failed", "app", app.ID, "stream_id", e.ID, "attempt", attempt, "error", err)
		if attempt == maxAttempts {
			return err
		}
		backoff := nextBackoff(app.Delivery.Callback.InitialBackoff.Duration, app.Delivery.Callback.MaxBackoff.Duration, attempt)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

type statusError int

func (s statusError) Error() string { return http.StatusText(int(s)) }

func nextBackoff(initial, max time.Duration, attempt int) time.Duration {
	multiplier := math.Pow(2, float64(attempt-1))
	d := time.Duration(float64(initial) * multiplier)
	if d > max {
		return max
	}
	return d
}
