# Configuration Guide

`webhook-router` uses YAML configuration. Production config is mounted into the container and is not part of the Docker image.

Default path:

```bash
webhook-router --config ./config.yaml
```

## Minimal Example

```yaml
server:
  listen: ":18080"

redis:
  addr: "redis:6379"
  password: "replace-with-redis-password"

channels:
  - id: gewe-main
    secret: "replace-with-channel-secret"
    enabled: true

apps:
  - id: hermes-prod
    token: "replace-with-app-token"
    enabled: true
    delivery:
      sse:
        enabled: true
      callback:
        enabled: false

routes:
  - id: gewe-main-to-hermes-prod
    channel: gewe-main
    app: hermes-prod
    enabled: true
```

## Full Example

See [`../config.example.yaml`](../config.example.yaml).

## Add A Channel

```yaml
channels:
  - id: gewe-test
    secret: "replace-with-channel-secret"
    enabled: true
```

Webhook URL:

```text
https://your-domain.com/webhooks/gewe-test
```

Secret header:

```http
X-Relay-Secret: replace-with-channel-secret
```

Fallback query secret:

```text
https://your-domain.com/webhooks/gewe-test?secret=replace-with-channel-secret
```

## Add An SSE App

```yaml
apps:
  - id: debug-client
    token: "replace-with-app-token"
    enabled: true
    delivery:
      sse:
        enabled: true
      callback:
        enabled: false
```

SSE URL:

```text
https://your-domain.com/apps/debug-client/events?token=replace-with-app-token
```

Bearer token is also supported:

```http
Authorization: Bearer replace-with-app-token
```

## Add A Callback App

```yaml
apps:
  - id: crm-prod
    enabled: true
    delivery:
      sse:
        enabled: false
      callback:
        enabled: true
        url: "https://crm.example.com/webhook/gewe"
        secret: "replace-with-callback-secret"
        timeout: "10s"
        max_attempts: 5
        initial_backoff: "1s"
        max_backoff: "60s"
```

Callback requests include:

```http
X-Relay-Callback-Secret: replace-with-callback-secret
```

The consumer app should compare this header with its configured secret.

## Temporary File Hosting

Temporary file hosting is configured globally. Apps authenticate uploads with their existing App token. Download URLs are public and expire automatically.

```yaml
files:
  storage_dir: "/tmp/webhook-router-files"
  ttl: "10m"
  max_bytes: 52428800
```

Fields:

| Field | Meaning | Default |
| --- | --- | --- |
| `storage_dir` | Local directory used to store temporary files and metadata | `/tmp/webhook-router-files` |
| `ttl` | How long uploaded files remain available | `10m` |
| `max_bytes` | Maximum upload size for one request | `52428800` |

Upload URL for an App:

```text
https://your-domain.com/apps/debug-client/files?token=replace-with-app-token
```

Returned paths look like `/files/{file_id}/{filename}` and can be opened directly on the same domain.

## Add A Route

```yaml
routes:
  - id: gewe-main-to-debug-client
    channel: gewe-main
    app: debug-client
    enabled: true
```

## Apply Changes

After editing config on the server:

```bash
cd /opt/webhook-router
docker compose restart webhook-router
```

Verify:

```bash
curl http://127.0.0.1:18080/healthz
curl http://127.0.0.1:18080/stats
```

## Production Notes

- Do not commit production `config.yaml`.
- Rotate `channels[].secret`, `apps[].token`, and callback secrets when leaked.
- Current MVP stores incoming headers as-is in Redis Stream.
- Redis password is configured on the server only.

