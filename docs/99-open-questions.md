# 待确认问题

这个文档用于记录实现前需要和你一问一答确认的事项。默认我们会优先选择简单、稳定、方便部署的方案。

## P0：MVP 实现前建议确认

1. Channel secret 放在哪里？

   已确认：MVP 同时支持请求头 `X-Relay-Secret: xxx` 和 query 参数 `?secret=xxx`。

   当两者同时存在时，以请求头为准。

2. Webhook body 非 JSON 时如何包装？

   已确认：

   - JSON：解析后放入 `body`
   - 非 JSON 但可按 UTF-8 解码的文本：放入 `body.raw`
   - 二进制或不可按 UTF-8 解码的内容：放入 `body_base64`

3. Webhook 成功响应状态码使用什么？

   已确认：Webhook 接收成功后直接返回成功。

   MVP 使用 `200 OK`，响应只表示 Relay 已经接收第三方回调并提交给内部投递流程，不代表任何消费应用已经完成业务处理。

4. 没有在线 SSE 客户端时，Webhook 是否仍返回成功？

   已确认：仍返回成功。第三方 Webhook 接收和消费应用投递解耦。

   已确认：MVP 需要队列和缓存能力，使用 Redis Stream 实现。无在线 SSE 客户端时事件应进入 Redis Stream，后续客户端可补发。

   已确认：MVP 同时支持 SSE 和 HTTP Callback。消费应用投递失败不影响第三方 Webhook 成功响应。

5. 慢客户端如何处理？

   建议队列满时断开该 SSE 客户端并记录日志，避免慢连接拖垮服务。

6. 是否需要限制 Webhook 请求体大小？

   建议默认 `1 MiB`，可通过配置调整。

7. 是否需要保留所有请求 headers？

   建议 MVP 全量保留，但后续可配置脱敏或过滤，例如 `Authorization`、`Cookie`。

8. SSE heartbeat 间隔用多少？

   建议默认 `15s`。

9. 是否需要从第一版开始支持 `Last-Event-ID`？

   已确认：MVP 支持标准 SSE `Last-Event-ID` 断线补发。

   语义：首次连接不补历史；重连携带 `Last-Event-ID` 时补发该事件之后的事件；如果事件已不在 Redis Stream 保留窗口内，发送 `replay_missed` 事件后从当前最新位置继续。

10. 是否需要热加载配置？

    建议第一版不做，修改配置后重启服务。

## P1：实现过程中可继续确认

1. 是否需要支持多个配置文件或配置目录？
2. App token 是否只允许 query 参数，还是也支持 `Authorization: Bearer`？

   已确认：MVP 同时支持 query token 和 `Authorization: Bearer`。两者同时存在时，以 Bearer token 为准。

3. Channel secret 是否要支持 HMAC 签名校验？
4. 是否需要 Webhook 请求 IP allowlist？
5. 事件 ID 使用 ULID、UUIDv7 还是简单随机 ID？

   已确认：MVP 使用 Redis Stream ID 作为 SSE `id` 和事件 JSON 中的 `id`。

   已确认：MVP 增加 `source_id` 字段，用于标识同一个原始 Webhook。一个原始 Webhook 路由到多个 App 时，各 App Stream 的 `id` 可以不同，但 `source_id` 相同。

6. `/stats` 这类状态接口是否要在 MVP 暴露？

   已确认：MVP 提供基础 `/stats`，用于查看在线连接数、配置对象数量和基础投递统计；完整事件查看和投递状态查询后置。

7. Docker 镜像是否需要多架构构建？
8. 是否有固定部署域名或反向代理环境？
9. Redis Stream 是否按 App 建 Stream，还是按 Channel 建 Stream 再派发？

   已确认：MVP 按 App 建 Stream，key 格式为 `{key_prefix}:app:{app_id}:events`。

10. 是否使用 Redis Consumer Group 管理投递 worker？

   已确认：MVP 不使用 Redis Consumer Group，按单实例部署设计。

11. HTTP Callback 的鉴权方式使用固定 secret header，还是 HMAC 签名？

   已确认：MVP 使用简单 secret header：`X-Relay-Callback-Secret: {secret}`。HMAC 签名作为后续安全增强。

12. 一个 App 是否允许同时启用 SSE 和 Callback？

   已确认：允许。一个 App 可以同时启用 SSE 和 HTTP Callback。

13. 是否需要对保存的 headers 做脱敏？

   已确认：MVP 默认不脱敏，原样保存 headers；后续预留可配置脱敏。

## 当前建议默认值

| 问题 | 建议默认值 |
| --- | --- |
| Channel secret | 同时支持 `X-Relay-Secret` header 和 `?secret=xxx`，Header 优先 |
| App token | query 参数 `token` |
| App token header | 同时支持 `Authorization: Bearer`，Bearer 优先 |
| Webhook 成功响应 | `200 OK`，只确认 Relay 接收成功 |
| 无在线客户端 | 仍返回成功，事件进入缓存等待后续补发 |
| 请求体大小 | `1 MiB` |
| heartbeat | `15s` |
| 慢客户端策略 | 队列满则断开 |
| 队列 | Redis Stream |
| 缓存 | Redis Stream |
| Stream 建模 | 按 App 建 Stream：`{key_prefix}:app:{app_id}:events` |
| SSE 事件 ID | Redis Stream ID，例如 `1715330000000-0` |
| 原始 Webhook ID | `source_id`，用于跨 App 关联同一个原始 Webhook |
| 投递方式 | MVP 支持 SSE 和 HTTP Callback |
| 同一 App 多投递方式 | 允许 SSE 和 HTTP Callback 同时启用 |
| Callback 成功判定 | `2xx` 成功；非 `2xx`、超时、网络错误重试 |
| Callback 鉴权 | 简单 secret header：`X-Relay-Callback-Secret` |
| Callback 重试 | 指数退避，超过最大次数后记录失败日志和统计 |
| Redis Consumer Group | MVP 不使用，按单实例部署 |
| `/stats` | MVP 提供基础统计 |
| Header 脱敏 | MVP 默认不脱敏，后续预留可配置脱敏 |
| Docker 升级 | 替换镜像重新部署，配置文件和 Redis 与镜像分离 |
| 配置热加载 | MVP 不支持 |
| 事件回放 | 支持标准 SSE `Last-Event-ID` 补发 |
| 存储 | Redis |
