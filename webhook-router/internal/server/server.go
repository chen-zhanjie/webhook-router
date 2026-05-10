package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/chen-zhanjie/webhook-router/internal/broker"
	"github.com/chen-zhanjie/webhook-router/internal/callback"
	"github.com/chen-zhanjie/webhook-router/internal/config"
	"github.com/chen-zhanjie/webhook-router/internal/event"
	"github.com/chen-zhanjie/webhook-router/internal/store"
)

type Server struct {
	reg     *config.Registry
	store   *store.RedisStore
	broker  *broker.Broker
	cbStats *callback.Stats
	log     *slog.Logger
	started time.Time
}

func New(reg *config.Registry, store *store.RedisStore, broker *broker.Broker, cbStats *callback.Stats, log *slog.Logger) *Server {
	return &Server{reg: reg, store: store, broker: broker, cbStats: cbStats, log: log, started: time.Now()}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", s.handleHealthz)
	r.Get("/stats", s.handleStats)
	r.Post("/webhooks/{channel_id}", s.handleWebhook)
	r.Get("/apps/{app_id}/events", s.handleSSE)
	return r
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "redis_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	appIDs := make([]string, 0, len(s.reg.Apps))
	for appID := range s.reg.Apps {
		appIDs = append(appIDs, appID)
	}
	streamLengths, err := s.store.Stats(r.Context(), appIDs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "stats_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                 true,
		"uptime_seconds":     int64(time.Since(s.started).Seconds()),
		"channels":           len(s.reg.Channels),
		"apps":               len(s.reg.Apps),
		"routes":             len(s.reg.Routes),
		"online_connections": s.broker.TotalOnline(),
		"online_by_app":      s.broker.Snapshot(),
		"stream_lengths":     streamLengths,
		"callback":           s.cbStats.Snapshot(),
	})
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channel_id")
	ch, ok := s.reg.Channels[channelID]
	if !ok {
		writeError(w, http.StatusNotFound, "channel_not_found")
		return
	}
	if !ch.IsEnabled() {
		writeError(w, http.StatusForbidden, "channel_disabled")
		return
	}
	if got := channelSecret(r); got != ch.Secret {
		writeError(w, http.StatusUnauthorized, "invalid_channel_secret")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.reg.Config.Server.MaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "body_too_large")
		return
	}
	bodyValue, bodyBase64, err := event.BuildBody(body, r.Header.Get("Content-Type"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body")
		return
	}
	routes := s.reg.ByChannel[channelID]
	sourceID, err := newSourceID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	base := event.WebhookEvent{
		SourceID:   sourceID,
		Channel:    channelID,
		ReceivedAt: time.Now().UTC(),
		Headers:    cloneHeader(r.Header),
		Body:       bodyValue,
		BodyBase64: bodyBase64,
	}
	streamIDs := map[string]string{}
	seenApps := map[string]struct{}{}
	for _, route := range routes {
		app, ok := s.reg.Apps[route.App]
		if !ok || !app.IsEnabled() {
			continue
		}
		if _, seen := seenApps[app.ID]; seen {
			continue
		}
		seenApps[app.ID] = struct{}{}
		id, err := s.store.AddEvent(r.Context(), app.ID, base)
		if err != nil {
			s.log.Error("write app stream failed", "app", app.ID, "channel", channelID, "source_id", sourceID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		streamIDs[app.ID] = id
	}
	s.log.Info("webhook accepted",
		"channel", channelID,
		"source_id", sourceID,
		"routes", len(routes),
		"apps", len(streamIDs),
		"stream_ids", streamIDs,
		"headers", base.Headers,
		"body", base.Body,
		"body_base64", base.BodyBase64,
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "source_id": sourceID, "stream_ids": streamIDs})
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "app_id")
	app, ok := s.reg.Apps[appID]
	if !ok {
		writeError(w, http.StatusNotFound, "app_not_found")
		return
	}
	if !app.IsEnabled() {
		writeError(w, http.StatusForbidden, "app_disabled")
		return
	}
	if !s.reg.SSEEnabled(app) {
		writeError(w, http.StatusNotFound, "sse_not_supported")
		return
	}
	if got := appToken(r); got != app.Token {
		writeError(w, http.StatusUnauthorized, "invalid_app_token")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "sse_not_supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	client := s.broker.Register(appID)
	defer s.broker.Unregister(client)
	s.log.Info("sse connected", "app", appID)
	defer s.log.Info("sse disconnected", "app", appID)

	lastID := r.Header.Get("Last-Event-ID")
	if lastID != "" {
		newLastID, missed, err := s.replay(r.Context(), w, flusher, appID, lastID)
		if err != nil {
			s.log.Error("sse replay failed", "app", appID, "last_event_id", lastID, "error", err)
		} else if newLastID != "" {
			lastID = newLastID
		} else if missed {
			latest, err := s.store.LatestID(r.Context(), appID)
			if err == nil && latest != "" {
				lastID = latest
			}
		}
	} else {
		lastID = "$"
	}

	ticker := broker.Heartbeat(s.reg.Config.SSE.HeartbeatInterval.Duration)
	defer ticker.Stop()
	for {
		events, err := s.store.XRead(r.Context(), appID, lastID, time.Second, 100)
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			s.log.Error("sse read stream failed", "app", appID, "last_id", lastID, "error", err)
			return
		}
		for _, e := range events {
			if err := writeSSE(w, "webhook", e.ID, e); err != nil {
				return
			}
			lastID = e.ID
		}
		if len(events) > 0 {
			flusher.Flush()
		}

		select {
		case <-client.Done():
			return
		case <-ticker.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		default:
			// Continue polling Redis Stream.
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) replay(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, appID, lastID string) (string, bool, error) {
	first, err := s.store.FirstID(ctx, appID)
	if err != nil {
		return "", false, err
	}
	if first != "" {
		cmp, err := store.CompareStreamID(lastID, first)
		if err != nil {
			return "", false, err
		}
		if cmp < 0 {
			payload := map[string]any{"last_event_id": lastID, "reason": "event_not_in_retention_window"}
			if err := writeSSE(w, "replay_missed", "", payload); err != nil {
				return "", true, err
			}
			flusher.Flush()
			latest, err := s.store.LatestID(ctx, appID)
			return latest, true, err
		}
	}
	events, err := s.store.ReadAfter(ctx, appID, lastID, 1000)
	if err != nil {
		return "", false, err
	}
	newLastID := lastID
	for _, e := range events {
		if err := writeSSE(w, "webhook", e.ID, e); err != nil {
			return "", false, err
		}
		newLastID = e.ID
	}
	flusher.Flush()
	return newLastID, false, nil
}

func channelSecret(r *http.Request) string {
	if value := r.Header.Get("X-Relay-Secret"); value != "" {
		return value
	}
	return r.URL.Query().Get("secret")
}

func appToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("Bearer "):])
	}
	return r.URL.Query().Get("token")
}

func writeSSE(w io.Writer, eventName, id string, data any) error {
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if eventName != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", eventName); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", raw)
	return err
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": code})
}

func cloneHeader(h http.Header) http.Header {
	out := http.Header{}
	for k, values := range h {
		out[k] = append([]string(nil), values...)
	}
	return out
}

func newSourceID() (string, error) {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id, err := ulid.New(ulid.Timestamp(time.Now()), entropy)
	if err != nil {
		return "", err
	}
	return "src_" + id.String(), nil
}
