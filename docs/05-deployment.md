# 部署与运行

## 部署形态

MVP 推荐两种部署形态：

1. 单二进制运行
2. Docker 容器运行

MVP 依赖 Redis Stream 作为事件队列和缓存。服务配置文件挂载到运行环境中。

## 端口与反向代理

服务默认监听：

```text
:18080
```

公网部署时建议放在 Nginx、Caddy、Traefik 或云厂商负载均衡之后，由反向代理负责：

- HTTPS 证书
- 域名绑定
- 请求体大小限制
- 访问日志
- 可选 IP allowlist

SSE 场景需要关闭代理缓冲或确认代理不会缓存事件流。

Nginx 示例要点：

```nginx
proxy_http_version 1.1;
proxy_set_header Connection "";
proxy_buffering off;
proxy_cache off;
```

## Docker 建议

Docker 镜像应包含：

- 编译后的 `webhook-router` 二进制
- 默认非 root 用户
- 暴露 `18080` 端口
- 通过 volume 挂载配置文件
- 通过配置文件或环境变量连接 Redis

Docker 镜像不应内置生产配置、secret 或运行时状态。升级时应只替换镜像，继续挂载原有配置文件并复用原有 Redis。

示例运行方式：

```bash
docker run --rm \
  -p 18080:18080 \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  webhook-router:latest \
  --config /app/config.yaml
```

当前项目提供本地构建部署脚本：

```bash
cd webhook-router
./deploy.sh deploy
```

脚本会在本地构建 `linux/amd64` 镜像，传输到 `root@120.79.241.27`，并在服务器 `/opt/webhook-router` 下通过 Docker Compose 重启容器。服务器上的 `config.yaml` 只在不存在时初始化，后续部署不会覆盖。

当前 1Panel 服务器上的 Redis 位于 Docker 网络 `1panel-network`，并带有网络别名 `redis`。部署脚本会把 `webhook-router` 加入该网络，因此 Redis 地址应配置为：

```yaml
redis:
  addr: "redis:6379"
```

Redis 密码在服务器 `/opt/webhook-router/config.yaml` 中配置，不提交到仓库。

如果 Redis 运行在服务器宿主机，容器内访问地址通常应配置为：

```yaml
redis:
  addr: "host.docker.internal:6379"
```

本地联调可使用 Redis 容器：

```bash
docker run --rm -p 6379:6379 redis:7-alpine
```

## 升级策略

生产环境预期采用 Docker 镜像升级和重新部署模式。

升级原则：

- 配置文件与镜像分离，`config.yaml` 通过 volume 挂载或外部配置管理提供
- Redis 数据与应用容器分离，Redis Stream 保留窗口内的事件不随应用容器替换丢失
- 新版本启动时必须兼容旧版本配置，新增配置项应提供默认值
- 不允许在镜像升级时覆盖用户已有配置
- 不允许把 secret 写入镜像或默认示例配置之外的代码常量
- 应提供版本号和启动日志，方便确认当前运行镜像版本

推荐升级流程：

1. 拉取或构建新镜像
2. 保留原 `config.yaml` 和 Redis
3. 使用新镜像启动新容器
4. 通过 `/healthz` 和 `/stats` 验证服务状态
5. 确认 SSE 连接可重连，Callback 投递正常
6. 删除旧容器

如果未来引入配置结构变化，应遵守兼容策略：

- 新增字段必须可选，并有安全默认值
- 字段重命名时至少保留一个版本的兼容读取
- 删除字段前先在文档和启动日志中标记 deprecated
- 配置校验错误必须明确指出字段路径和原因

Redis Stream 数据结构升级应尽量向后兼容。事件 JSON 新增字段可以接受，既有字段的语义不应破坏；如果必须变更事件结构，应引入事件 `schema_version`。

## 健康检查

建议容器健康检查调用：

```http
GET /healthz
```

## 优雅退出

服务收到 `SIGINT` 或 `SIGTERM` 后应：

1. 停止接收新请求
2. 尽量完成正在处理的 Webhook 请求
3. 停止启动新的 Callback 投递任务
4. 尽量完成正在执行的 Callback 请求
5. 关闭 SSE 连接，让客户端自动重连到新容器
6. 在 `shutdown_timeout` 内退出

## 日志

生产环境建议使用 JSON 日志输出到 stdout/stderr，由容器平台或 systemd 收集。

关键字段建议：

- `time`
- `level`
- `msg`
- `request_id`
- `source_id`
- `stream_id`
- `channel`
- `app`
- `route`
- `delivery`
- `status`
- `error`

## 运维注意事项

- MVP 使用 Redis Stream，因此服务重启后 Redis 保留窗口内的事件仍可补发
- MVP 按单实例部署设计，不使用 Redis Consumer Group
- 如果要正式多实例部署，需要进一步明确 Consumer Group、App dispatcher 和连接归属策略
- 第三方 Webhook 响应应尽快返回成功，消费应用投递失败、重试、指数退避由 Relay 内部策略处理
- Docker 升级时应保持配置文件和 Redis 不变，替换应用容器即可

## Nginx 反向代理

Nginx / 1Panel OpenResty 反代目标：

```text
http://127.0.0.1:18080
```

示例：

```nginx
location / {
    proxy_pass http://127.0.0.1:18080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}

location /apps/ {
    proxy_pass http://127.0.0.1:18080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
    add_header X-Accel-Buffering no;
}
```

`/apps/` 是 SSE 长连接路径，必须关闭代理缓冲并设置较长超时。
