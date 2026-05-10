# 开发路线图

## 第一阶段：MVP

目标：完成可部署、可联调的 Webhook 路由平台，MVP 支持 SSE 和 HTTP Callback。

范围：

- Go 项目初始化
- 配置文件加载与校验
- Webhook 接收接口
- Channel secret 鉴权
- 统一事件结构
- 静态路由匹配
- Redis Stream 事件队列
- Redis Stream 最近事件缓存
- 内存 Broker
- App token 鉴权
- SSE 推送
- HTTP Callback 投递
- Callback 失败重试与指数退避
- `Last-Event-ID` 断线补发
- heartbeat
- 支持一个 App 多个连接
- 基础结构化日志
- 基础 `/stats`
- 健康检查接口
- Dockerfile
- Docker 升级兼容：镜像与配置分离，复用 Redis 和旧配置
- 示例配置
- README 使用说明

验收建议：

- 使用 `curl` 可以建立 SSE 连接
- 使用 `curl` 可以向 Webhook 发送 JSON 请求
- 一个 Channel 可以路由到多个 App
- 无在线 SSE 客户端时，事件可以进入缓存等待后续补发
- 客户端携带 `Last-Event-ID` 重连时，可以补发断线期间事件
- 一个 App 的多个 SSE 连接都能收到消息
- 配置了 Callback 的 App 可以收到 HTTP 回调
- `/stats` 可以查看在线连接数、配置对象数量和基础投递统计
- 替换 Docker 镜像并复用原配置后，服务可以正常启动
- Webhook 接收成功后立即返回成功，不等待消费应用完成处理
- 鉴权失败返回明确错误
- 客户端断开后连接能被清理

## 第二阶段：可靠性增强

目标：提升断线恢复和可观测性。

候选功能：

- 更完整的补发窗口、过期告警和回放观测能力
- Redis Consumer Group 和多实例投递协调
- 客户端重连补发优化
- Callback 投递状态查询
- Callback 重试队列观测
- 事件查看 API
- 连接状态 API
- 基础 Prometheus metrics
- 更完善的请求 ID 和链路日志

## 第三阶段：平台化

目标：支持更复杂的路由、管理和运营能力。

候选功能：

- 管理后台
- 管理 API
- 条件路由
- 事件搜索
- 投递状态
- 投递 ACK
- 告警和监控
- Token 轮换
- 多实例部署方案

## 暂缓事项

以下事项在真实需求明确前不进入 MVP：

- 业务消息解析
- Gewe API 调用
- 机器人逻辑
- 完整用户系统
- 多租户计费
- 强一致投递
