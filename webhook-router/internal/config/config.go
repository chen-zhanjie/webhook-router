package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", s, err)
	}
	d.Duration = parsed
	return nil
}

type Config struct {
	Server   ServerConfig `yaml:"server"`
	SSE      SSEConfig    `yaml:"sse"`
	Redis    RedisConfig  `yaml:"redis"`
	Queue    QueueConfig  `yaml:"queue"`
	Cache    CacheConfig  `yaml:"cache"`
	Files    FileConfig   `yaml:"files"`
	Log      LogConfig    `yaml:"log"`
	Channels []Channel    `yaml:"channels"`
	Apps     []App        `yaml:"apps"`
	Routes   []Route      `yaml:"routes"`
}

type ServerConfig struct {
	Listen          string   `yaml:"listen"`
	ReadTimeout     Duration `yaml:"read_timeout"`
	WriteTimeout    Duration `yaml:"write_timeout"`
	ShutdownTimeout Duration `yaml:"shutdown_timeout"`
	MaxBodyBytes    int64    `yaml:"max_body_bytes"`
}

type SSEConfig struct {
	HeartbeatInterval Duration `yaml:"heartbeat_interval"`
	ConnectionBuffer  int      `yaml:"connection_buffer_size"`
	SlowClientPolicy  string   `yaml:"slow_client_policy"`
}

type RedisConfig struct {
	Addr         string `yaml:"addr"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	KeyPrefix    string `yaml:"key_prefix"`
	StreamMaxLen int64  `yaml:"stream_max_len"`
}

type QueueConfig struct {
	Type string `yaml:"type"`
}

type CacheConfig struct {
	Type            string   `yaml:"type"`
	MaxEventsPerApp int64    `yaml:"max_events_per_app"`
	TTL             Duration `yaml:"ttl"`
}

type FileConfig struct {
	StorageDir string   `yaml:"storage_dir"`
	TTL        Duration `yaml:"ttl"`
	MaxBytes   int64    `yaml:"max_bytes"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type Channel struct {
	ID      string `yaml:"id"`
	Secret  string `yaml:"secret"`
	Enabled *bool  `yaml:"enabled"`
}

func (c Channel) IsEnabled() bool { return c.Enabled == nil || *c.Enabled }

type App struct {
	ID       string      `yaml:"id"`
	Token    string      `yaml:"token"`
	Enabled  *bool       `yaml:"enabled"`
	Delivery AppDelivery `yaml:"delivery"`
}

func (a App) IsEnabled() bool { return a.Enabled == nil || *a.Enabled }

type AppDelivery struct {
	SSE      SSEDelivery      `yaml:"sse"`
	Callback CallbackDelivery `yaml:"callback"`
}

type SSEDelivery struct {
	Enabled *bool `yaml:"enabled"`
}

func (d SSEDelivery) IsEnabled(defaultValue bool) bool {
	if d.Enabled == nil {
		return defaultValue
	}
	return *d.Enabled
}

type CallbackDelivery struct {
	Enabled        bool     `yaml:"enabled"`
	URL            string   `yaml:"url"`
	Secret         string   `yaml:"secret"`
	Timeout        Duration `yaml:"timeout"`
	MaxAttempts    int      `yaml:"max_attempts"`
	InitialBackoff Duration `yaml:"initial_backoff"`
	MaxBackoff     Duration `yaml:"max_backoff"`
}

type Route struct {
	ID      string `yaml:"id"`
	Channel string `yaml:"channel"`
	App     string `yaml:"app"`
	Enabled *bool  `yaml:"enabled"`
}

func (r Route) IsEnabled() bool { return r.Enabled == nil || *r.Enabled }

type Registry struct {
	Config    Config
	Channels  map[string]Channel
	Apps      map[string]App
	Routes    []Route
	ByChannel map[string][]Route
}

func Load(path string) (*Registry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := defaultConfig()
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return NewRegistry(cfg)
}

func NewRegistry(cfg Config) (*Registry, error) {
	applyDefaults(&cfg)
	reg := &Registry{
		Config:    cfg,
		Channels:  map[string]Channel{},
		Apps:      map[string]App{},
		Routes:    []Route{},
		ByChannel: map[string][]Route{},
	}
	if err := reg.validate(); err != nil {
		return nil, err
	}
	return reg, nil
}

func defaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Listen:          ":18080",
			ReadTimeout:     Duration{10 * time.Second},
			WriteTimeout:    Duration{0},
			ShutdownTimeout: Duration{10 * time.Second},
			MaxBodyBytes:    1 << 20,
		},
		SSE: SSEConfig{
			HeartbeatInterval: Duration{15 * time.Second},
			ConnectionBuffer:  64,
			SlowClientPolicy:  "disconnect",
		},
		Redis: RedisConfig{
			Addr:         "127.0.0.1:6379",
			KeyPrefix:    "relay",
			StreamMaxLen: 10000,
		},
		Queue: QueueConfig{Type: "redis_stream"},
		Cache: CacheConfig{Type: "redis_stream", MaxEventsPerApp: 10000, TTL: Duration{24 * time.Hour}},
		Files: FileConfig{StorageDir: "/tmp/webhook-router-files", TTL: Duration{10 * time.Minute}, MaxBytes: 50 << 20},
		Log:   LogConfig{Level: "info", Format: "json"},
	}
}

func applyDefaults(cfg *Config) {
	def := defaultConfig()
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = def.Server.Listen
	}
	if cfg.Server.ReadTimeout.Duration == 0 {
		cfg.Server.ReadTimeout = def.Server.ReadTimeout
	}
	if cfg.Server.ShutdownTimeout.Duration == 0 {
		cfg.Server.ShutdownTimeout = def.Server.ShutdownTimeout
	}
	if cfg.Server.MaxBodyBytes == 0 {
		cfg.Server.MaxBodyBytes = def.Server.MaxBodyBytes
	}
	if cfg.SSE.HeartbeatInterval.Duration == 0 {
		cfg.SSE.HeartbeatInterval = def.SSE.HeartbeatInterval
	}
	if cfg.SSE.ConnectionBuffer == 0 {
		cfg.SSE.ConnectionBuffer = def.SSE.ConnectionBuffer
	}
	if cfg.SSE.SlowClientPolicy == "" {
		cfg.SSE.SlowClientPolicy = def.SSE.SlowClientPolicy
	}
	if cfg.Redis.Addr == "" {
		cfg.Redis.Addr = def.Redis.Addr
	}
	if cfg.Redis.KeyPrefix == "" {
		cfg.Redis.KeyPrefix = def.Redis.KeyPrefix
	}
	if cfg.Redis.StreamMaxLen == 0 {
		cfg.Redis.StreamMaxLen = def.Redis.StreamMaxLen
	}
	if cfg.Queue.Type == "" {
		cfg.Queue.Type = def.Queue.Type
	}
	if cfg.Cache.Type == "" {
		cfg.Cache.Type = def.Cache.Type
	}
	if cfg.Cache.MaxEventsPerApp == 0 {
		cfg.Cache.MaxEventsPerApp = def.Cache.MaxEventsPerApp
	}
	if cfg.Cache.TTL.Duration == 0 {
		cfg.Cache.TTL = def.Cache.TTL
	}
	if cfg.Files.StorageDir == "" {
		cfg.Files.StorageDir = def.Files.StorageDir
	}
	if cfg.Files.TTL.Duration == 0 {
		cfg.Files.TTL = def.Files.TTL
	}
	if cfg.Files.MaxBytes == 0 {
		cfg.Files.MaxBytes = def.Files.MaxBytes
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = def.Log.Level
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = def.Log.Format
	}

	for i := range cfg.Apps {
		cb := &cfg.Apps[i].Delivery.Callback
		if cb.Timeout.Duration == 0 {
			cb.Timeout = Duration{10 * time.Second}
		}
		if cb.MaxAttempts == 0 {
			cb.MaxAttempts = 5
		}
		if cb.InitialBackoff.Duration == 0 {
			cb.InitialBackoff = Duration{time.Second}
		}
		if cb.MaxBackoff.Duration == 0 {
			cb.MaxBackoff = Duration{60 * time.Second}
		}
	}
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func (r *Registry) validate() error {
	var errs []error
	if r.Config.Server.MaxBodyBytes <= 0 {
		errs = append(errs, errors.New("server.max_body_bytes must be > 0"))
	}
	if r.Config.SSE.HeartbeatInterval.Duration <= 0 {
		errs = append(errs, errors.New("sse.heartbeat_interval must be > 0"))
	}
	if r.Config.SSE.ConnectionBuffer <= 0 {
		errs = append(errs, errors.New("sse.connection_buffer_size must be > 0"))
	}
	if r.Config.Redis.Addr == "" {
		errs = append(errs, errors.New("redis.addr is required"))
	}
	if r.Config.Redis.StreamMaxLen <= 0 {
		errs = append(errs, errors.New("redis.stream_max_len must be > 0"))
	}
	if r.Config.Queue.Type != "redis_stream" {
		errs = append(errs, errors.New("queue.type must be redis_stream"))
	}
	if r.Config.Cache.Type != "redis_stream" {
		errs = append(errs, errors.New("cache.type must be redis_stream"))
	}
	if r.Config.Cache.MaxEventsPerApp <= 0 {
		errs = append(errs, errors.New("cache.max_events_per_app must be > 0"))
	}
	if r.Config.Cache.TTL.Duration <= 0 {
		errs = append(errs, errors.New("cache.ttl must be > 0"))
	}
	if r.Config.Files.StorageDir == "" {
		errs = append(errs, errors.New("files.storage_dir is required"))
	}
	if r.Config.Files.TTL.Duration <= 0 {
		errs = append(errs, errors.New("files.ttl must be > 0"))
	}
	if r.Config.Files.MaxBytes <= 0 {
		errs = append(errs, errors.New("files.max_bytes must be > 0"))
	}

	for _, ch := range r.Config.Channels {
		if !idPattern.MatchString(ch.ID) {
			errs = append(errs, fmt.Errorf("channels[%s].id is invalid", ch.ID))
		}
		if ch.Secret == "" {
			errs = append(errs, fmt.Errorf("channels[%s].secret is required", ch.ID))
		}
		if _, exists := r.Channels[ch.ID]; exists {
			errs = append(errs, fmt.Errorf("duplicate channel id %q", ch.ID))
		}
		r.Channels[ch.ID] = ch
	}

	for _, app := range r.Config.Apps {
		if !idPattern.MatchString(app.ID) {
			errs = append(errs, fmt.Errorf("apps[%s].id is invalid", app.ID))
		}
		if _, exists := r.Apps[app.ID]; exists {
			errs = append(errs, fmt.Errorf("duplicate app id %q", app.ID))
		}
		sseEnabled := app.Delivery.SSE.IsEnabled(!app.Delivery.Callback.Enabled)
		if !sseEnabled && !app.Delivery.Callback.Enabled {
			errs = append(errs, fmt.Errorf("apps[%s] must enable sse or callback", app.ID))
		}
		if sseEnabled && app.Token == "" {
			errs = append(errs, fmt.Errorf("apps[%s].token is required when sse is enabled", app.ID))
		}
		if app.Delivery.Callback.Enabled {
			if app.Delivery.Callback.URL == "" {
				errs = append(errs, fmt.Errorf("apps[%s].delivery.callback.url is required", app.ID))
			}
			if _, err := url.ParseRequestURI(app.Delivery.Callback.URL); err != nil {
				errs = append(errs, fmt.Errorf("apps[%s].delivery.callback.url is invalid: %w", app.ID, err))
			}
			if app.Delivery.Callback.Timeout.Duration <= 0 || app.Delivery.Callback.MaxAttempts <= 0 || app.Delivery.Callback.InitialBackoff.Duration <= 0 || app.Delivery.Callback.MaxBackoff.Duration <= 0 {
				errs = append(errs, fmt.Errorf("apps[%s].delivery.callback retry settings must be > 0", app.ID))
			}
		}
		r.Apps[app.ID] = app
	}

	routeIDs := map[string]struct{}{}
	for _, route := range r.Config.Routes {
		if !idPattern.MatchString(route.ID) {
			errs = append(errs, fmt.Errorf("routes[%s].id is invalid", route.ID))
		}
		if _, exists := routeIDs[route.ID]; exists {
			errs = append(errs, fmt.Errorf("duplicate route id %q", route.ID))
		}
		routeIDs[route.ID] = struct{}{}
		if _, exists := r.Channels[route.Channel]; !exists {
			errs = append(errs, fmt.Errorf("routes[%s].channel %q not found", route.ID, route.Channel))
		}
		if _, exists := r.Apps[route.App]; !exists {
			errs = append(errs, fmt.Errorf("routes[%s].app %q not found", route.ID, route.App))
		}
		if route.IsEnabled() {
			r.Routes = append(r.Routes, route)
			r.ByChannel[route.Channel] = append(r.ByChannel[route.Channel], route)
		}
	}

	return errors.Join(errs...)
}

func (r *Registry) SSEEnabled(app App) bool {
	return app.Delivery.SSE.IsEnabled(!app.Delivery.Callback.Enabled)
}
