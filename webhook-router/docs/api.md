# HTTP API Reference

## Health Check

```http
GET /healthz
```

Success:

```json
{
  "ok": true
}
```

Redis unavailable:

```json
{
  "ok": false,
  "error": "redis_unavailable"
}
```

## Stats

```http
GET /stats
```

Response:

```json
{
  "ok": true,
  "uptime_seconds": 208,
  "channels": 1,
  "apps": 1,
  "routes": 1,
  "online_connections": 0,
  "online_by_app": {},
  "stream_lengths": {
    "hermes-prod": 0
  },
  "callback": {
    "success": {},
    "failed": {},
    "permanent_failed": {}
  }
}
```

`/stats` is intended for trusted internal access or protected reverse proxy access.

## Receive Webhook

```http
POST /webhooks/{channel_id}
```

Authentication:

```http
X-Relay-Secret: channel-secret
```

Fallback:

```text
POST /webhooks/{channel_id}?secret=channel-secret
```

JSON request example:

```bash
curl -X POST 'http://localhost:18080/webhooks/gewe-main' \
  -H 'X-Relay-Secret: channel-secret' \
  -H 'Content-Type: application/json' \
  -d '{"hello":"world"}'
```

Success response:

```json
{
  "ok": true,
  "source_id": "src_01J00000000000000000000000",
  "stream_ids": {
    "hermes-prod": "1715330000000-0"
  }
}
```

The response means the Relay accepted the Webhook and wrote events to Redis Stream for routed Apps. It does not mean consumer business logic has completed.

Error response shape:

```json
{
  "ok": false,
  "error": "invalid_channel_secret"
}
```

Common error codes:

| Status | Error | Meaning |
| --- | --- | --- |
| `404` | `channel_not_found` | Channel ID is not configured |
| `403` | `channel_disabled` | Channel exists but is disabled |
| `401` | `invalid_channel_secret` | Secret mismatch |
| `413` | `body_too_large` | Request body exceeds `server.max_body_bytes` |
| `400` | `invalid_body` | JSON body is invalid when treated as JSON |
| `500` | `internal_error` | Redis or internal error |

## SSE Events

```http
GET /apps/{app_id}/events?token=app-token
```

Bearer token is supported:

```http
Authorization: Bearer app-token
```

Success response headers:

```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

Event example:

```text
id: 1715330000000-0
event: webhook
data: {"id":"1715330000000-0","source_id":"src_01J00000000000000000000000","channel":"gewe-main","received_at":"2026-05-10T12:00:00Z","headers":{},"body":{"hello":"world"}}
```

Heartbeat:

```text
: heartbeat
```

Reconnect replay:

```http
Last-Event-ID: 1715330000000-0
```

Replay missed event:

```text
event: replay_missed
data: {"last_event_id":"1715330000000-0","reason":"event_not_in_retention_window"}
```

SSE error codes:

| Status | Error | Meaning |
| --- | --- | --- |
| `404` | `app_not_found` | App ID is not configured |
| `403` | `app_disabled` | App exists but is disabled |
| `401` | `invalid_app_token` | Token mismatch |
| `404` | `sse_not_supported` | SSE is disabled for this App |

## Temporary File Upload

```http
POST /apps/{app_id}/files?token=app-token
```

Bearer token is supported:

```http
Authorization: Bearer app-token
```

Request body must be `multipart/form-data` with file field name `file`:

```bash
curl -F 'file=@./example.jpg' \
  'http://localhost:18080/apps/hermes-prod/files?token=app-token'
```

Success response:

```json
{
  "ok": true,
  "path": "/files/01j00000000000000000000000/example.jpg",
  "expires_at": "2026-05-11T12:10:00Z",
  "size": 12345,
  "filename": "example.jpg"
}
```

`path` is relative to the current service domain and port. For example, with `https://hook.yunzxu.com`:

```text
https://hook.yunzxu.com/files/01j00000000000000000000000/example.jpg
```

Files are temporary and expire according to `files.ttl`, which defaults to `10m`. Expired or missing files return `404`.

Upload error codes:

| Status | Error | Meaning |
| --- | --- | --- |
| `404` | `app_not_found` | App ID is not configured |
| `403` | `app_disabled` | App exists but is disabled |
| `401` | `invalid_app_token` | Token mismatch |
| `403` | `file_upload_not_configured` | App has no token configured for uploads |
| `400` | `file_upload_failed` | Missing `file` field, invalid multipart body, or file too large |

## Temporary File Download

```http
GET /files/{file_id}/{filename}
```

This endpoint is public by design, because the upload response is intended to be shared as a direct temporary URL. The service sets `Cache-Control: no-store` and deletes expired files during periodic cleanup or on access after expiry.

## HTTP Callback Delivery

Callback requests are sent by `webhook-router` to the configured App URL.

Request headers:

```http
Content-Type: application/json
X-Relay-Callback-Secret: callback-secret
X-Relay-Source-ID: src_01J00000000000000000000000
X-Relay-Stream-ID: 1715330000000-0
X-Relay-App: crm-prod
```

Request body:

```json
{
  "id": "1715330000000-0",
  "source_id": "src_01J00000000000000000000000",
  "channel": "gewe-main",
  "received_at": "2026-05-10T12:00:00Z",
  "headers": {},
  "body": {
    "hello": "world"
  }
}
```

Callback success rule:

- `2xx` means success
- non-`2xx`, timeout, or network error means failure and will be retried
- retries use exponential backoff according to App config

## Body Wrapping Rules

| Incoming body | Event fields |
| --- | --- |
| Valid JSON | `body` stores parsed JSON |
| UTF-8 non-JSON text | `body.raw` stores text |
| Binary or invalid UTF-8 | `body_base64` stores base64 |

