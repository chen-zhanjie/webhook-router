# 应用接入指南

本文面向后续接入方，说明如何新增第三方 Webhook 入口、消费应用和路由关系。

## 核心对象

### Channel

Channel 表示一个第三方 Webhook 入口。

新增 Channel 后，会得到一个公网回调地址：

```text
POST /webhooks/{channel_id}
```

例如：

```text
POST /webhooks/gewe-main
```

### App

App 表示一个消费应用。

MVP 支持两种投递方式：

- SSE：消费应用主动建立长连接接收事件
- HTTP Callback：Relay 主动向消费应用发起 HTTP 回调

一个 App 可以同时启用 SSE 和 HTTP Callback。

### Route

Route 表示 Channel 到 App 的转发关系。

一个 Channel 可以路由到多个 App，一个 App 也可以接收多个 Channel。

## 新增一个 SSE 应用

目标：让 `debug-client` 通过 SSE 接收 `gewe-main` 的消息。

编辑服务器配置：

```bash
ssh root@120.79.241.27
cd /opt/webhook-router
vim config.yaml
```

新增 App：

```yaml
apps:
  - id: debug-client
    token: "replace-with-debug-token"
    enabled: true
    delivery:
      sse:
        enabled: true
      callback:
        enabled: false
```

新增 Route：

```yaml
routes:
  - id: gewe-main-to-debug-client
    channel: gewe-main
    app: debug-client
    enabled: true
```

重启服务：

```bash
docker compose restart webhook-router
```

客户端连接 SSE：

```bash
curl -N 'https://your-domain.com/apps/debug-client/events?token=replace-with-debug-token'
```

也可以使用 Bearer token：

```bash
curl -N 'https://your-domain.com/apps/debug-client/events' \
  -H 'Authorization: Bearer replace-with-debug-token'
```

## 新增一个 HTTP Callback 应用

目标：让 `crm-prod` 通过 HTTP Callback 接收 `gewe-main` 的消息。

新增 App：

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

新增 Route：

```yaml
routes:
  - id: gewe-main-to-crm-prod
    channel: gewe-main
    app: crm-prod
    enabled: true
```

重启服务：

```bash
docker compose restart webhook-router
```

Relay 会向 Callback URL 发送请求：

```http
POST /webhook/gewe
Content-Type: application/json
X-Relay-Callback-Secret: replace-with-callback-secret
X-Relay-Source-ID: src_01J00000000000000000000000
X-Relay-Stream-ID: 1715330000000-0
X-Relay-App: crm-prod
```

消费应用只需要校验 `X-Relay-Callback-Secret` 是否等于配置值。

Callback 返回 `2xx` 视为成功。非 `2xx`、请求超时或网络错误会按指数退避重试。

## 新增一个 Webhook 入口

目标：新增 `gewe-test` 入口，并路由给 `debug-client`。

新增 Channel：

```yaml
channels:
  - id: gewe-test
    secret: "replace-with-channel-secret"
    enabled: true
```

新增 Route：

```yaml
routes:
  - id: gewe-test-to-debug-client
    channel: gewe-test
    app: debug-client
    enabled: true
```

第三方平台配置 Webhook 地址：

```text
https://your-domain.com/webhooks/gewe-test
```

推荐使用 Header 传 secret：

```http
X-Relay-Secret: replace-with-channel-secret
```

如果第三方平台不支持自定义 Header，也可以使用 query：

```text
https://your-domain.com/webhooks/gewe-test?secret=replace-with-channel-secret
```

## 事件格式

SSE 和 HTTP Callback 使用同一份事件 JSON：

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

字段说明：

- `id`：当前 App Stream 的 Redis Stream ID，也是 SSE `id`
- `source_id`：同一个原始 Webhook 的关联 ID，跨 App 一致
- `channel`：来源 Channel ID
- `received_at`：Relay 接收第三方 Webhook 的时间
- `headers`：第三方 Webhook 原始请求 headers
- `body`：请求体。JSON 原样保留，非 JSON 文本放入 `body.raw`
- `body_base64`：二进制或不可 UTF-8 解码内容

## SSE 断线重连

SSE 客户端应保存最后收到的 `id`，重连时放入标准 Header：

```http
Last-Event-ID: 1715330000000-0
```

Relay 会从 Redis Stream 补发该 ID 之后的事件。

如果事件已经超过保留窗口，Relay 会发送：

```text
event: replay_missed
data: {"last_event_id":"1715330000000-0","reason":"event_not_in_retention_window"}
```

## 配置生效流程

修改 `/opt/webhook-router/config.yaml` 后，需要重启容器：

```bash
cd /opt/webhook-router
docker compose restart webhook-router
```

查看状态：

```bash
docker compose ps
curl http://127.0.0.1:18080/healthz
curl http://127.0.0.1:18080/stats
```

查看日志：

```bash
docker logs -f --tail=200 webhook-router
```

## 当前线上部署约定

服务器：`120.79.241.27`

部署目录：

```text
/opt/webhook-router
```

服务监听：

```text
127.0.0.1:18080
```

容器加入 1Panel 网络：

```text
1panel-network
```

Redis 地址：

```yaml
redis:
  addr: "redis:6379"
```

Redis 密码在服务器 `/opt/webhook-router/config.yaml` 中配置，不写入仓库。

Nginx / OpenResty 反代目标：

```text
http://127.0.0.1:18080
```

SSE 路径 `/apps/{app_id}/events` 需要关闭代理缓冲并设置较长超时。

