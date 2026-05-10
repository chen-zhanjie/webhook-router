# 产品需求与边界

## 背景

Gewe 等第三方平台只支持通过公网回调地址推送消息。消费应用可能运行在本地、内网或不适合暴露公网地址的环境中，因此需要一个公网中转服务接收 Webhook，并通过 SSE 长连接把消息转发给消费应用。

## 目标

实现一个轻量级公网 Webhook 路由平台。第一版支持 SSE 和 HTTP Callback 两种消费应用投递方式，后续可扩展消息队列或其他投递方式。

```text
Gewe / 第三方平台
    -> 公网 Webhook 回调
    -> Relay 中转服务
    -> SSE 长连接 / HTTP Callback（MVP）
    -> 消费应用 / 客户端
```

核心目标：

- 避免消费应用必须暴露公网地址
- 支持多个第三方消息入口
- 支持多个消费应用
- 支持频道到应用的多对多转发
- 第一版支持 SSE 和 HTTP Callback 投递，后续预留其他投递方式
- 保持服务简单、稳定、易部署

## 核心概念

### Channel

Channel 表示一个第三方消息入口。每个 Channel 对应一个公网 Webhook 地址。

示例，Hermes 只是其中一种消费应用：

- `gewe-main`
- `gewe-test`
- `feishu-bot`
- `github-webhook`

对应接口：

```http
POST /webhooks/{channel_id}
```

### App

App 表示一个消费应用。MVP 中，App 可以通过 SSE 长连接接收事件，也可以由 Relay 通过 HTTP Callback 主动回调，后续 App 可以扩展为消息队列等其他投递方式。

示例：

- `hermes-prod`
- `hermes-dev`
- `debug-client`

对应接口：

```http
GET /apps/{app_id}/events?token=xxx
```

### Route

Route 表示 Channel 到 App 的转发关系。

关系是多对多：

- 一个 Channel 可以转发给多个 App
- 一个 App 可以接收多个 Channel 的消息

示例：

- `gewe-main -> hermes-prod`
- `gewe-main -> debug-client`
- `gewe-test -> hermes-dev`

## MVP 功能范围

### Webhook 接收

服务提供公网 Webhook 地址，接收 Gewe 或其他第三方平台的 `POST` 请求。

接收到请求后：

1. 校验 Channel 是否存在且启用
2. 校验 Channel secret
3. 生成 `source_id` 标识同一个原始 Webhook
5. 记录请求时间、headers、body
6. 查找该 Channel 绑定的 App
7. 将事件写入 Redis Stream 队列和最近事件缓存
8. 立即返回 Webhook 接收成功
9. 由内部投递流程异步推送或回调目标 App

### SSE 长连接

MVP 中，应用通过 SSE 接收事件：

```http
GET /apps/{app_id}/events?token=xxx
```

要求：

- 校验 App 是否存在且启用
- 校验 App token
- 建立 SSE 长连接
- 定期发送 heartbeat
- 客户端断线后可以自动重连
- 支持一个 App 多个客户端同时连接
- 支持从事件缓存中补发最近事件
- 支持标准 SSE `Last-Event-ID` 断线补发

### HTTP Callback

MVP 中，App 也可以配置 HTTP Callback 投递方式，由 Relay 主动向 App 的 callback URL 发起 `POST` 请求。

要求：

- Callback URL 通过配置文件管理
- Callback 请求体使用统一事件结构
- 支持 Callback secret header 鉴权，后续可增强为 HMAC 签名
- 支持请求超时配置
- 支持失败重试和指数退避
- Callback 投递结果不影响第三方 Webhook 的接收成功响应

### 事件队列

MVP 需要包含内部事件队列，用于把 Webhook 接收和消费应用投递解耦。

要求：

- Webhook 接收成功后将事件提交到队列
- 队列按路由将事件分发给目标 App，并根据 App 投递方式执行 SSE 推送或 HTTP Callback
- 投递流程不阻塞第三方 Webhook 响应
- 投递失败、重试和指数退避预留实现空间

### 事件缓存

MVP 需要包含最近事件缓存，用于客户端断线重连或暂时无在线连接时补发。

要求：

- 按 App 或按 Route 保存最近事件
- 缓存大小可配置
- 服务重启后可在 Redis Stream 保留窗口内补发事件
- 与标准 SSE `Last-Event-ID` 配合补发断线期间事件

### 统一事件结构

事件中的 `id` 使用当前 App Stream 的 Redis Stream ID，用于 SSE `Last-Event-ID` 和按 App 补发。

事件中的 `source_id` 标识同一个原始 Webhook。一个 Webhook 路由到多个 App 时，不同 App Stream 中的 `id` 可以不同，但 `source_id` 相同。

### 配置文件管理

第一版不做管理后台，通过配置文件完成 Channel、App 和 Route 管理。

### 日志

MVP 应记录关键生命周期日志：

- 服务启动和配置加载结果
- Webhook 请求接收结果
- 鉴权失败原因
- 路由匹配结果
- SSE 客户端连接、断开、发送失败
- 运行时错误

## 非目标

第一版不做：

- 不解析微信业务消息
- 不直接调用 Gewe API
- 不做机器人逻辑
- 不做复杂用户系统
- 不做多租户计费
- 不做完整管理后台
- 不保证强一致投递
