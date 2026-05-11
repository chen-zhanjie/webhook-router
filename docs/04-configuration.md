# 配置文件设计

## 配置文件格式

MVP 使用 YAML 配置文件管理服务、Channel、App 和 Route。

配置文件属于运行时数据，不应打包进生产镜像。Docker 部署时应通过 volume、ConfigMap、Secret 或宿主机文件挂载提供。

建议默认文件路径：

```text
config.yaml
```

服务启动时可通过参数指定：

```bash
webhook-router --config ./config.yaml
```

## 示例配置

```yaml
server:
  listen: ":18080"
  read_timeout: "10s"
  write_timeout: "0s"
  shutdown_timeout: "10s"
  max_body_bytes: 1048576

sse:
  heartbeat_interval: "15s"
  connection_buffer_size: 64
  slow_client_policy: "disconnect"

redis:
  addr: "redis:6379"
  username: ""
  password: ""
  db: 0
  key_prefix: "relay"
  stream_max_len: 10000

queue:
  type: "redis_stream"

cache:
  type: "redis_stream"
  max_events_per_app: 10000
  ttl: "24h"

files:
  storage_dir: "/tmp/webhook-router-files"
  ttl: "10m"
  max_bytes: 52428800

log:
  level: "info"
  format: "json"

channels:
  - id: gewe-main
    secret: "channel-secret"
    enabled: true

apps:
  - id: hermes-prod
    token: "app-token"
    enabled: true
    delivery:
      sse:
        enabled: true
      callback:
        enabled: false
        url: ""
        secret: ""
        timeout: "10s"
        max_attempts: 5
        initial_backoff: "1s"
        max_backoff: "60s"

routes:
  - id: gewe-to-hermes
    channel: gewe-main
    app: hermes-prod
    enabled: true
```

## 字段说明

### server

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `listen` | string | `":18080"` | HTTP 监听地址 |
| `read_timeout` | duration | `10s` | HTTP 读取超时 |
| `write_timeout` | duration | `0s` | HTTP 写入超时，SSE 场景建议不设置固定写超时 |
| `shutdown_timeout` | duration | `10s` | 优雅退出等待时间 |
| `max_body_bytes` | int64 | `1048576` | Webhook 请求体最大字节数 |

### sse

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `heartbeat_interval` | duration | `15s` | SSE heartbeat 间隔 |
| `connection_buffer_size` | int | `64` | 每个 SSE 连接的事件缓冲队列长度 |
| `slow_client_policy` | string | `disconnect` | 慢客户端策略，候选值：`disconnect`、`drop_newest`、`block` |

### redis

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `addr` | string | `127.0.0.1:6379` | Redis 地址 |
| `username` | string | `""` | Redis 用户名 |
| `password` | string | `""` | Redis 密码 |
| `db` | int | `0` | Redis DB |
| `key_prefix` | string | `relay` | Redis key 前缀 |
| `stream_max_len` | int64 | `10000` | 每个 App Stream 的近似最大保留条数 |

MVP 按 App 建 Stream，key 格式为：

```text
{key_prefix}:app:{app_id}:events
```

当前 1Panel 部署中，`webhook-router` 容器加入 `1panel-network`，Redis 使用该网络中的别名 `redis`，因此线上配置应为：

```yaml
redis:
  addr: "redis:6379"
  password: "在服务器配置文件中填写"
```

Redis 密码只写入服务器 `/opt/webhook-router/config.yaml`，不要提交到仓库。

### queue

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `type` | string | `redis_stream` | 队列类型，MVP 使用 `redis_stream` |

### cache

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `type` | string | `redis_stream` | 缓存类型，MVP 使用 `redis_stream` |
| `max_events_per_app` | int | `10000` | 每个 App 最多保留的最近事件数 |
| `ttl` | duration | `24h` | 事件最长保留时间 |

### files

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `storage_dir` | string | `/tmp/webhook-router-files` | 临时文件存储目录 |
| `ttl` | duration | `10m` | 文件上传后保留时间 |
| `max_bytes` | int64 | `52428800` | 单次文件上传最大字节数 |

应用使用自己的 App token 上传文件。成功后返回 `/files/{file_id}/{filename}` 相对路径，可拼接当前域名和端口直接访问。下载地址不要求鉴权，过期或不存在返回 `404`。

### log

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `level` | string | `info` | 日志级别：`debug`、`info`、`warn`、`error` |
| `format` | string | `json` | 日志格式：`json`、`text` |

### channels

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `id` | string | 是 | Channel ID，用于 URL path |
| `secret` | string | 是 | Webhook secret |
| `enabled` | bool | 否 | 是否启用，默认 `true` |

### apps

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `id` | string | 是 | App ID，用于 URL path |
| `token` | string | 否 | SSE 连接 token。启用 SSE 时必填 |
| `enabled` | bool | 否 | 是否启用，默认 `true` |
| `delivery.sse.enabled` | bool | 否 | 是否启用 SSE 投递 |
| `delivery.callback.enabled` | bool | 否 | 是否启用 HTTP Callback 投递 |
| `delivery.callback.url` | string | 否 | Callback URL，启用 Callback 时必填 |
| `delivery.callback.secret` | string | 否 | Callback secret header 鉴权密钥 |
| `delivery.callback.timeout` | duration | 否 | Callback 请求超时 |
| `delivery.callback.max_attempts` | int | 否 | Callback 最大尝试次数 |
| `delivery.callback.initial_backoff` | duration | 否 | Callback 初始退避时间 |
| `delivery.callback.max_backoff` | duration | 否 | Callback 最大退避时间 |

一个 App 可以同时启用 SSE 和 HTTP Callback。此时同一个 App Stream 中的事件会同时进入 SSE 在线推送流程和 Callback 投递流程。

### routes

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `id` | string | 是 | Route ID |
| `channel` | string | 是 | 来源 Channel ID |
| `app` | string | 是 | 目标 App ID |
| `enabled` | bool | 否 | 是否启用，默认 `true` |

## 校验规则

启动时应执行配置校验：

1. `channels[].id` 必须唯一
2. `apps[].id` 必须唯一
3. `routes[].id` 必须唯一
4. Route 引用的 Channel 必须存在
5. Route 引用的 App 必须存在
6. `channels[].secret` 不能为空
7. `max_body_bytes` 必须大于 0
8. `heartbeat_interval` 必须大于 0
9. `connection_buffer_size` 必须大于 0
10. `redis.addr` 不能为空
11. `redis.stream_max_len` 必须大于 0
12. `queue.type` 必须为 `redis_stream`
13. `cache.type` 必须为 `redis_stream`
14. `cache.max_events_per_app` 必须大于 0
15. `cache.ttl` 必须大于 0
16. `files.storage_dir` 不能为空
17. `files.ttl` 必须大于 0
18. `files.max_bytes` 必须大于 0
19. App 至少启用一种投递方式：SSE 或 HTTP Callback
20. 启用 SSE 的 App 必须配置 `token`
21. 启用 HTTP Callback 的 App 必须配置 `delivery.callback.url`
22. Callback `timeout`、`max_attempts`、`initial_backoff`、`max_backoff` 必须大于 0

## 配置兼容策略

为了支持 Docker 镜像直接升级并复用现有配置，配置解析需要遵守以下规则：

- 新增配置项必须有默认值
- 未识别字段默认不应导致启动失败，除非该字段位于安全敏感配置区域并可能造成误判
- 必填字段缺失时，错误信息必须包含字段路径
- 字段废弃时先保留兼容读取，并在启动日志中输出 deprecated 提示
- 示例配置可以更新，但不能要求用户每次升级都手动迁移已有配置

配置中的 secret/token 可以继续保存在外部 `config.yaml` 中，也可以后续扩展环境变量展开。无论哪种方式，生产 secret 都不应写入镜像。

## ID 规则建议

建议 Channel、App、Route ID 采用小写字母、数字和短横线：

```text
^[a-z0-9][a-z0-9-]{0,62}$
```

这样可以直接安全用于 URL path、日志和指标标签。

## 密钥管理

MVP 可以先在 YAML 中写明文 secret/token，部署时通过文件权限和服务器访问控制保护。

后续建议支持环境变量展开：

```yaml
channels:
  - id: gewe-main
    secret: "${GEWE_MAIN_SECRET}"
    enabled: true
```

## 常见配置变更

### 添加 Channel

```yaml
channels:
  - id: gewe-test
    secret: "replace-with-channel-secret"
    enabled: true
```

第三方平台回调地址为：

```text
https://your-domain.com/webhooks/gewe-test
```

### 添加 SSE App

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

SSE 连接地址为：

```text
https://your-domain.com/apps/debug-client/events?token=replace-with-app-token
```

### 添加 Callback App

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

### 添加 Route

```yaml
routes:
  - id: gewe-main-to-debug-client
    channel: gewe-main
    app: debug-client
    enabled: true
```

修改配置后重启容器：

```bash
cd /opt/webhook-router
docker compose restart webhook-router
```
