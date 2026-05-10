# 信桥

信桥是一个轻量级公网 Webhook 路由平台，用于接收第三方平台的公网 Webhook 回调，并按配置路由给本地、内网或其他消费应用。

当前代码实现目录为：

- `webhook-router/`：Webhook 路由服务的 Go 代码实现目录，MVP 支持 SSE 和 HTTP Callback 两种投递方式
- `docs/`：项目设计文档、接口约定、部署说明和待确认问题

## 项目定位

第一版服务只负责：

- 接收第三方平台 Webhook 回调
- 校验 Channel secret
- 按配置路由到一个或多个 App
- 将事件写入内部队列和缓存
- 通过 SSE 长连接或 HTTP Callback 向 App 投递统一事件
- 维护基础心跳、日志和 Docker 部署能力

第一版服务不负责：

- 解析微信或其他平台的业务消息
- 调用 Gewe 或其他第三方平台 API
- 实现机器人业务逻辑
- 提供完整管理后台、计费或复杂租户系统
- 保证强一致投递

## 文档入口

- [产品需求与边界](./docs/01-requirements.md)
- [系统设计](./docs/02-architecture.md)
- [HTTP API 设计](./docs/03-api.md)
- [配置文件设计](./docs/04-configuration.md)
- [部署与运行](./docs/05-deployment.md)
- [开发路线图](./docs/06-roadmap.md)
- [待确认问题](./docs/99-open-questions.md)

## MVP 范围

MVP 聚焦在稳定、简单、易部署：

1. YAML 配置加载
2. Webhook 接收接口：`POST /webhooks/{channel_id}`
3. App SSE 接口：`GET /apps/{app_id}/events?token=xxx`
4. Channel secret 鉴权
5. App token 鉴权
6. Channel 到 App 的多对多路由
7. 事件队列
8. 最近事件缓存
9. HTTP Callback 投递
10. 一个 App 支持多个 SSE 客户端同时连接
11. SSE heartbeat
12. Callback 失败重试和指数退避
13. 基础结构化日志
14. Docker 部署

后续版本可以继续增加消息队列、Redis Consumer、Webhook fanout 等投递方式。SSE 和 HTTP Callback 是第一版投递通道。

## 推荐实现方向

- 语言：Go
- HTTP：`net/http` 或 `chi`
- 配置：YAML
- 日志：`log/slog`
- 队列与缓存：Redis Stream
- 部署：单二进制 + Docker
