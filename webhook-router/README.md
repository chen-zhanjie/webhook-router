# webhook-router

`webhook-router` is a lightweight Webhook routing service. It receives third-party Webhook callbacks, stores events in Redis Stream per App, and delivers events through SSE and HTTP Callback.

The service is intentionally generic. It does not parse WeChat, Gewe, Feishu, GitHub, or any business-specific payload. It validates, stores, routes, and forwards events.

## What It Does

- Receives Webhooks at `POST /webhooks/{channel_id}`
- Authenticates Channel requests with `X-Relay-Secret` or `?secret=...`
- Writes events to Redis Stream per App: `{key_prefix}:app:{app_id}:events`
- Delivers events through SSE: `GET /apps/{app_id}/events`
- Supports standard SSE `Last-Event-ID` replay
- Delivers events through HTTP Callback
- Retries failed Callback delivery with exponential backoff
- Lets Apps upload temporary files and returns directly accessible relative paths
- Exposes `/healthz` and `/stats`
- Supports Docker image replacement while reusing existing config and Redis

## Documentation

- [Developer Guide](./docs/developer-guide.md)
- [Configuration Guide](./docs/configuration.md)
- [HTTP API Reference](./docs/api.md)
- [Deployment Guide](./docs/deployment.md)

## Quick Start

Start Redis:

```bash
docker run --rm -p 6379:6379 redis:7-alpine
```

Run the service:

```bash
cp config.example.yaml config.yaml
go run ./cmd/webhook-router --config ./config.yaml
```

Open an SSE connection:

```bash
curl -N 'http://localhost:18080/apps/hermes-prod/events?token=app-token'
```

Upload a temporary file for an App:

```bash
curl -F 'file=@./example.jpg' \
  'http://localhost:18080/apps/hermes-prod/files?token=app-token'
```

The response `path` can be opened directly on the same domain and expires after 10 minutes by default.

Send a Webhook:

```bash
curl -X POST 'http://localhost:18080/webhooks/gewe-main' \
  -H 'X-Relay-Secret: channel-secret' \
  -H 'Content-Type: application/json' \
  -d '{"hello":"world"}'
```

Expected Webhook response:

```json
{
  "ok": true,
  "source_id": "src_01J00000000000000000000000",
  "stream_ids": {
    "hermes-prod": "1715330000000-0"
  }
}
```

Expected SSE event:

```text
id: 1715330000000-0
event: webhook
data: {"id":"1715330000000-0","source_id":"src_01J00000000000000000000000","channel":"gewe-main","received_at":"2026-05-10T12:00:00Z","headers":{},"body":{"hello":"world"}}
```

## Project Layout

```text
cmd/webhook-router/       CLI entrypoint
internal/app/             app bootstrapping, graceful shutdown, cleanup worker
internal/broker/          online SSE connection statistics
internal/callback/        HTTP Callback worker and retry logic
internal/config/          YAML config loading, defaults, validation, registry
internal/event/           event model and body wrapping
internal/files/           temporary App file upload, serving, and cleanup
internal/server/          HTTP handlers for Webhook, SSE, files, health, stats
internal/store/           Redis Stream access layer
```

## Build And Test

```bash
go test ./...
go vet ./...
go build ./cmd/webhook-router
```

## Docker

Build:

```bash
docker build -t webhook-router:latest .
```

Run:

```bash
docker run --rm \
  -p 18080:18080 \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  webhook-router:latest
```

Production configuration must be mounted from outside the image. Do not bake `config.yaml`, tokens, or secrets into the image.

## Deploy

This repository includes a local-build deployment script for the current 1Panel server:

```bash
./deploy.sh deploy
```

The script builds `webhook-router:latest` locally for `linux/amd64`, streams the image to `root@120.79.241.27`, and restarts the remote Docker Compose service in `/opt/webhook-router`.

The remote `config.yaml` is created only when missing and is never overwritten during upgrades.

On the current 1Panel server, Redis runs in Docker network `1panel-network` with the alias `redis`. The deployment script attaches `webhook-router` to that network, so the Redis address should be:

```yaml
redis:
  addr: "redis:6379"
```

Redis password is configured only in `/opt/webhook-router/config.yaml` on the server. Do not commit production secrets.

