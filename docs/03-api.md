# HTTP API 设计

## Webhook 接收

```http
POST /webhooks/{channel_id}
```

### 鉴权

MVP 同时支持 Header 和 Query 两种 Channel secret 传递方式。

推荐方式：

```http
X-Relay-Secret: channel-secret
```

兼容方式：

```http
POST /webhooks/{channel_id}?secret=channel-secret
```

当 Header 和 Query 同时存在时，以 Header 为准。

### 请求体

Webhook Handler 不解析业务内容，只保留原始请求体的结构。

请求体包装规则：

- JSON：解析后放入 `body`
- 非 JSON 但可按 UTF-8 解码的文本：放入 `body.raw`
- 二进制或不可按 UTF-8 解码的内容：放入 `body_base64`

示例：

```http
POST /webhooks/gewe-main
X-Relay-Secret: channel-secret
Content-Type: application/json

{"type":"text","content":"hello"}
```

对应事件中的 body：

```json
{
  "body": {
    "type": "text",
    "content": "hello"
  }
}
```

纯文本请求体示例：

```text
hello
```

对应事件中的 body：

```json
{
  "body": {
    "raw": "hello"
  }
}
```

二进制或不可按 UTF-8 解码的请求体示例：

```json
{
  "body_base64": "..."
}
```

### 成功响应

Webhook 接收成功后直接返回成功。该响应只表示 Relay 已经接收第三方回调，并已写入 Redis Stream；不代表任何消费应用已经完成业务处理。

建议响应：

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "ok": true,
  "source_id": "src_01J00000000000000000000000",
  "stream_ids": {
    "hermes-prod": "1715330000000-0"
  }
}
```

说明：

- Webhook 响应不携带消费应用业务处理结果
- MVP 中即使当前没有在线 SSE 客户端，只要回调已被 Relay 接收并写入 Redis Stream，也返回成功
- 投递失败、重试、指数退避属于 Relay 内部策略，不影响第三方 Webhook 的本次成功响应

### 失败响应

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{
  "ok": false,
  "error": "channel_not_found"
}
```

建议错误码：

- `channel_not_found`
- `channel_disabled`
- `invalid_channel_secret`
- `body_too_large`
- `invalid_body`
- `internal_error`

## HTTP Callback 投递

MVP 支持将事件通过 HTTP Callback 投递给 App。Callback 是 Relay 到消费应用的出站请求，不影响第三方 Webhook 的本次成功响应。

建议请求：

```http
POST {callback.url}
Content-Type: application/json
X-Relay-Callback-Secret: callback-secret
X-Relay-Source-ID: src_01J00000000000000000000000
X-Relay-Stream-ID: 1715330000000-0
X-Relay-App: hermes-prod
```

请求体与 SSE `data` 一致：

```json
{
  "id": "1715330000000-0",
  "source_id": "src_01J00000000000000000000000",
  "channel": "gewe-main",
  "received_at": "2026-05-10T12:00:00Z",
  "headers": {},
  "body": {}
}
```

MVP Callback 鉴权使用简单 secret header：

```http
X-Relay-Callback-Secret: callback-secret
```

消费应用比较 header 值是否等于配置的 secret。

Callback 响应 `2xx` 视为成功；非 `2xx`、请求超时或网络错误视为失败并进入重试。

重试策略：

- 使用指数退避
- 初始退避、最大退避和最大尝试次数由 App 配置控制
- 超过最大尝试次数后记录失败日志和统计信息
- Callback 投递失败不影响第三方 Webhook 的成功响应

## SSE 事件流

```http
GET /apps/{app_id}/events?token=xxx
```

### 鉴权

MVP 同时支持 query token 和 `Authorization: Bearer`。

query token：

```http
GET /apps/hermes-prod/events?token=app-token
```

Bearer token：

```http
Authorization: Bearer app-token
```

当 query token 和 Bearer token 同时存在时，以 Bearer token 为准。

### 成功响应头

```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

### Webhook 事件格式

```text
id: 1715330000000-0
event: webhook
data: {"id":"1715330000000-0","source_id":"src_01J00000000000000000000000","channel":"gewe-main","received_at":"2026-05-10T12:00:00Z","headers":{},"body":{}}

```

事件 JSON：

```json
{
  "id": "1715330000000-0",
  "source_id": "src_01J00000000000000000000000",
  "channel": "gewe-main",
  "received_at": "2026-05-10T12:00:00Z",
  "headers": {
    "Content-Type": ["application/json"]
  },
  "body": {}
}
```

### Heartbeat

推荐使用 SSE comment 作为 heartbeat：

```text
: heartbeat

```

默认间隔建议：`15s` 或 `30s`，由配置决定。

### 事件补发

MVP 支持标准 SSE `Last-Event-ID` 断线补发。

客户端重连示例：

```http
GET /apps/hermes-prod/events?token=app-token
Last-Event-ID: 1715330000000-0
```

补发语义：

- 首次连接不携带 `Last-Event-ID` 时，从当前最新位置开始接收，默认不补历史
- 重连携带 `Last-Event-ID` 时，从 Redis Stream 中补发该事件之后的事件
- 如果 `Last-Event-ID` 已经不在 Redis Stream 保留窗口内，服务发送 `replay_missed` 事件，然后从当前最新位置继续接收

`replay_missed` 示例：

```text
event: replay_missed
data: {"last_event_id":"1715330000000-0","reason":"event_not_in_retention_window"}

```

### 失败响应

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{
  "ok": false,
  "error": "invalid_app_token"
}
```

建议错误码：

- `app_not_found`
- `app_disabled`
- `invalid_app_token`
- `sse_not_supported`
- `internal_error`

## 健康检查

建议 MVP 提供健康检查接口：

```http
GET /healthz
```

响应：

```json
{
  "ok": true
}
```

## 对接 URL 汇总

假设 Nginx 反代域名为：

```text
https://your-domain.com
```

第三方平台 Webhook 地址：

```text
https://your-domain.com/webhooks/{channel_id}
```

SSE 消费地址：

```text
https://your-domain.com/apps/{app_id}/events?token={app_token}
```

健康检查地址，仅建议内网或受保护访问：

```text
https://your-domain.com/healthz
```

状态地址，仅建议内网或受保护访问：

```text
https://your-domain.com/stats
```

## 运行信息接口

MVP 可选提供只读状态接口，默认可不暴露公网或由反向代理保护。

```http
GET /readyz
GET /stats
```

`/stats` 可返回在线连接数、Channel 数、App 数、Route 数等基础信息。
