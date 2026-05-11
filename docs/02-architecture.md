# 系统设计

## 总体架构

```text
Third Party Platform
        |
        | POST /webhooks/{channel_id}
        v
+------------------------+
| webhook-router         |
|                        |
|  Webhook Handler       |
|  Config Registry       |
|  Route Matcher         |
|  Event Builder         |
|  Event Queue           |
|  Event Cache           |
|  In-Memory Broker      |
|  Delivery Adapter      |
|  Temporary File Store  |
|  SSE Handler           |
|  Callback Worker       |
+------------------------+
        |
        | text/event-stream / HTTP callback
        v
Consumer Apps / Clients
```

## 模块划分

### Config Registry

负责加载和校验 YAML 配置，提供运行时查询能力。

职责：

- 加载 server、channels、apps、routes
- 校验 ID 唯一性
- 校验 Route 引用的 Channel 和 App 是否存在
- 过滤 disabled 配置项

MVP 可在启动时加载一次配置。后续可以扩展热加载或管理 API。

### Webhook Handler

负责处理第三方平台回调。

职责：

- 解析 `channel_id`
- 校验 Channel 状态
- 校验 Channel secret
- 读取请求 headers 和 body
- 构造统一事件
- 调用 Route Matcher 获取目标 App
- 将事件写入 Redis Stream

### Route Matcher

负责根据 Channel 找到启用的目标 App。

MVP 只支持静态配置路由：

```text
channel_id -> []app_id
```

后续可以扩展条件路由，例如按消息类型、群聊 ID、请求 header 或 JSON 字段过滤。

### Event Builder

负责把原始 Webhook 请求包装成统一事件结构。

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

MVP 不理解 body 业务含义。请求体包装规则为：

- JSON：解析后放入 `body`
- 非 JSON 但可按 UTF-8 解码的文本：放入 `body.raw`
- 二进制或不可按 UTF-8 解码的内容：放入 `body_base64`

### Event Queue

Event Queue 负责把 Webhook 接收和消费应用投递解耦。

职责：

- 接收 Webhook Handler 生成的统一事件
- 根据 Route Matcher 生成面向目标 App 的投递任务
- 将投递任务交给 Delivery Adapter
- 为投递失败、重试、指数退避预留扩展点

MVP 队列使用 Redis Stream 实现。Webhook Handler 写入 Redis Stream 成功后即可返回第三方 Webhook 成功响应，后续由投递流程异步读取 Stream 并推送给目标 App。

### Event Cache

Event Cache 负责保存最近事件，用于断线重连和临时无在线连接时补发。

职责：

- 保存最近 N 条事件或最近一段时间内的事件
- 支持按 App 查询可补发事件
- 支持根据 SSE `Last-Event-ID` 找到重连后需要补发的事件范围

MVP 缓存使用 Redis Stream 实现。Redis Stream 同时承担事件队列和最近事件缓存的职责，通过最大长度、保留时间或清理任务控制保留窗口。

### Delivery Adapter

Delivery Adapter 表示 Relay 向消费应用投递事件的通道抽象。

MVP 实现 SSE 和 HTTP Callback：

```text
App -> SSE connection(s)
App -> HTTP Callback
```

后续可以扩展：

```text
App -> Message Queue
App -> Redis Stream consumer
```

Webhook 的响应不等待消费应用完成业务处理。第三方 Webhook 接收成功并写入 Redis Stream 后，Relay 立即返回成功；消费应用侧投递失败、重试、指数退避等属于 Relay 内部投递策略。

### In-Memory Broker

负责管理 App 的 SSE 连接，并向连接广播事件。

职责：

- 维护 `app_id -> connections` 映射
- 支持同一个 App 多连接
- 对目标 App 广播事件
- 处理连接断开清理
- 发送 heartbeat

SSE 在线连接仍由当前进程内存管理，意味着：

- 服务重启后在线连接会断开，但 Redis 保留窗口内的事件仍可补发
- 多实例部署时不能仅依赖本地连接表，需要额外设计实例间投递协调
- MVP 不保证消费应用业务处理成功，只保证事件写入 Redis 后可进入投递流程

### SSE Handler

负责建立和维护 `text/event-stream` 长连接。

职责：

- 校验 App token
- 设置 SSE 必要响应头
- 注册客户端连接
- 写入事件和 heartbeat
- 根据请求上下文感知客户端断开

### Callback Worker

负责从 App Stream 中读取待投递事件，并向配置了 HTTP Callback 的 App 发起 `POST` 请求。

职责：

- 按 App 读取 Redis Stream 事件
- 使用统一事件结构作为 Callback 请求体
- 附带 Callback 鉴权信息
- 记录投递成功、失败和重试日志
- 对失败请求执行重试和指数退避
- 避免阻塞第三方 Webhook 响应

## 请求流

### Webhook 到 SSE 转发

```text
1. 第三方平台 POST /webhooks/gewe-main
2. Webhook Handler 查询 channel gewe-main
3. 校验 secret
4. 读取 headers/body 并生成 event
5. Route Matcher 找到目标 apps
6. Webhook Handler 将 event 写入 Redis Stream
7. Webhook Handler 立即返回成功
8. Event Queue 异步生成 App 投递任务
9. Delivery Adapter 根据 App 配置执行 SSE 推送或 HTTP Callback
```

### SSE 连接建立

```text
1. App GET /apps/hermes-prod/events?token=xxx
2. SSE Handler 查询 app hermes-prod
3. 校验 token
4. 设置 text/event-stream 响应头
5. Broker 注册连接
6. 如果请求包含 `Last-Event-ID`，从 Redis Stream 补发该事件之后的事件
7. 周期性写 heartbeat
8. 客户端断开后 Broker 清理连接
```

## 并发模型建议

MVP 可使用 Redis Stream 加每个 SSE 连接一个发送队列：

```text
Redis Stream event
    -> App dispatcher 查找目标 app connections
    -> 非阻塞写入每个 connection 的 channel
    -> connection writer goroutine 顺序写 HTTP ResponseWriter
```

建议设置每个连接的发送队列长度，避免慢客户端拖垮 Webhook 请求处理。

慢客户端策略需要在实现前确认，候选方案：

- 队列满时丢弃新事件并记录日志
- 队列满时断开慢客户端
- 队列满时阻塞短时间后失败

## Redis Stream 设计

MVP 使用 Redis Stream 作为可靠队列和事件缓存基线。

MVP 确认按 App 建 Stream：

```text
relay:app:{app_id}:events
```

Webhook 接收后，Relay 根据 Route Matcher 找到目标 App，并为每个目标 App 写入对应的 App Stream。这样每个 App 的重连补发、保留窗口和消费游标都可以独立管理。

写入流程：

```text
Webhook -> Event Builder -> Route Matcher -> XADD relay:app:{app_id}:events
```

MVP 使用 Redis Stream ID 作为 SSE `id` 和事件 JSON 中的 `id`。写入 App Stream 后得到的 Redis Stream ID，例如 `1715330000000-0`，会直接用于 SSE：

```text
id: 1715330000000-0
event: webhook
data: {"id":"1715330000000-0","source_id":"src_01J00000000000000000000000","channel":"gewe-main","body":{}}
```

这样 `Last-Event-ID` 可以直接作为 Redis Stream 游标使用，不需要维护额外的事件 ID 到 Stream ID 的索引。

同一个 Webhook 事件路由到多个 App 时，会分别写入多个 App Stream。由于 Redis Stream ID 由每个 App Stream 写入时生成，同一原始 Webhook 在不同 App Stream 中的 SSE `id` 可以不同，但 `source_id` 相同，用于日志、排查和跨 App 关联。

## HTTP Callback 设计

MVP 支持 HTTP Callback 投递。配置了 Callback 的 App 会收到 Relay 发出的 `POST` 请求，请求体与 SSE `data` 使用相同事件 JSON。

建议请求：

```http
POST {callback.url}
Content-Type: application/json
X-Relay-Callback-Secret: callback-secret
X-Relay-Source-ID: src_01J00000000000000000000000
X-Relay-Stream-ID: 1715330000000-0
X-Relay-App: app-a
```

请求体：

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

MVP Callback 鉴权使用简单 secret header：`X-Relay-Callback-Secret: {secret}`。消费应用比较 header 值是否等于配置的 secret。

Callback 响应 `2xx` 视为成功；非 `2xx`、请求超时或网络错误视为失败并进入重试。重试使用指数退避，初始退避、最大退避和最大尝试次数由 App 配置控制。超过最大尝试次数后记录失败日志和统计信息。

SSE 断线补发使用标准 `Last-Event-ID`：

```text
GET /apps/{app_id}/events?token=xxx
Last-Event-ID: 1715330000000-0
```

补发语义：

- 首次连接不携带 `Last-Event-ID` 时，从当前最新位置开始接收，默认不补历史
- 重连携带 `Last-Event-ID` 时，从该事件之后开始补发
- 如果 `Last-Event-ID` 已经不在 Redis Stream 保留窗口内，发送 `replay_missed` 事件，然后从当前最新位置继续接收

MVP 不使用 Redis Consumer Group，按单实例部署设计。Relay 进程内的 App dispatcher 读取对应 App Stream，并向本实例在线 SSE 连接推送或执行 HTTP Callback。正式多实例部署时，再引入 Consumer Group、实例归属和投递协调策略。

## 安全边界

MVP 至少需要：

- Channel secret 用于保护 Webhook 入口
- App token 用于保护 SSE 消费入口
- 请求 body 大小限制
- 基础访问日志和失败日志

后续可以扩展：

- IP allowlist
- HMAC 签名校验
- HTTPS 终止建议
- Token 轮换
- Callback HMAC 签名校验
- 管理 API 鉴权

## 可扩展点

- Replay：基于 Redis Stream 的最近事件回放与 `Last-Event-ID`
- Route Conditions：条件路由
- HTTP Callback Delivery：向消费应用发起 HTTP 回调
- Delivery Retry：投递失败重试和指数退避
- Delivery ACK：消费端确认和投递状态
- Admin API：配置查看、事件查询、连接状态查询
- Metrics：Prometheus 指标与告警
