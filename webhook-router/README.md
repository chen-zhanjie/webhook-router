# webhook-router

`webhook-router` is a lightweight Webhook routing service. It accepts third-party Webhook callbacks, writes events to Redis Stream per App, and delivers them through SSE and HTTP Callback.

## Features

- Channel Webhook endpoint: `POST /webhooks/{channel_id}`
- Channel secret via `X-Relay-Secret` or `?secret=...`
- Redis Stream queue/cache per App: `relay:app:{app_id}:events`
- SSE endpoint: `GET /apps/{app_id}/events?token=...`
- Standard SSE `Last-Event-ID` replay
- HTTP Callback delivery with `X-Relay-Callback-Secret`
- Callback retry with exponential backoff
- `/healthz` and `/stats`

## Run Locally

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

Send a Webhook:

```bash
curl -X POST 'http://localhost:18080/webhooks/gewe-main' \
  -H 'X-Relay-Secret: channel-secret' \
  -H 'Content-Type: application/json' \
  -d '{"hello":"world"}'
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

Production configuration should be mounted from outside the image. Upgrade by replacing the image/container while reusing the existing `config.yaml` and Redis instance.

## Deploy

This repository includes a local-build deployment script for the 1Panel server:

```bash
./deploy.sh init
./deploy.sh deploy
```

The script builds `webhook-router:latest` locally for `linux/amd64`, streams the image to `root@120.79.241.27`, and restarts the remote Docker Compose service in `/opt/webhook-router`.

The remote `config.yaml` is created only when missing and is never overwritten during upgrades. Edit `/opt/webhook-router/config.yaml` on the server before the first real deployment.

If Redis runs on the server host, use this Redis address from inside the container:

```yaml
redis:
  addr: "host.docker.internal:6379"
```
