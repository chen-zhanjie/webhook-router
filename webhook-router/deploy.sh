#!/bin/bash
# webhook-router 生产部署脚本
#
# 用法:
#   ./deploy.sh deploy            本地构建镜像并部署到服务器
#   ./deploy.sh init              仅初始化服务器部署目录和配置模板
#   ./deploy.sh restart           仅重启服务器容器
#   ./deploy.sh logs              查看容器日志
#   ./deploy.sh status            查看容器状态
#
# 说明:
#   - 本地构建 linux/amd64 Docker 镜像
#   - 通过 docker save | gzip | ssh docker load 传输镜像
#   - 服务器上的 config.yaml 不会被覆盖
#   - Redis 默认使用 1Panel/宿主机已有 Redis，或你在 config.yaml 中自行配置

set -euo pipefail

SERVER="root@120.79.241.27"
DEPLOY_DIR="/opt/webhook-router"
IMAGE="webhook-router:latest"
CONTAINER="webhook-router"
COMPOSE_FILE="docker-compose.yml"
TARGET="${1:-deploy}"

echo "================================================"
echo " webhook-router 部署 | 目标: $TARGET"
echo " 服务器: $SERVER"
echo " 目录:   $DEPLOY_DIR"
echo "================================================"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "✗ 缺少命令: $1"
    exit 1
  }
}

ensure_local_tools() {
  require_cmd docker
  require_cmd ssh
  require_cmd gzip
}

remote() {
  ssh "$SERVER" "$@"
}

init_remote() {
  echo "▶ 初始化服务器目录..."
  remote "mkdir -p '$DEPLOY_DIR'"

  echo "▶ 写入 docker compose 文件..."
  remote "cat > '$DEPLOY_DIR/$COMPOSE_FILE'" <<'EOF'
services:
  webhook-router:
    image: webhook-router:latest
    container_name: webhook-router
    restart: unless-stopped
    ports:
      - "18080:18080"
    volumes:
      - ./config.yaml:/app/config.yaml:ro
    command: ["--config", "/app/config.yaml"]
    extra_hosts:
      - "host.docker.internal:host-gateway"
    healthcheck:
      test: ["CMD", "/app/webhook-router", "--version"]
      interval: 30s
      timeout: 5s
      retries: 3
EOF

  echo "▶ 准备 config.yaml（如已存在则不覆盖）..."
  if remote "test -f '$DEPLOY_DIR/config.yaml'"; then
    echo "  config.yaml 已存在，保持不变"
  else
    scp config.example.yaml "$SERVER:$DEPLOY_DIR/config.yaml"
    echo "  已上传 config.example.yaml 为服务器 config.yaml"
    echo "  ⚠ 请登录服务器修改 $DEPLOY_DIR/config.yaml 中的 secret/token/redis 配置"
  fi
}

build_image() {
  echo "▶ 本地构建 Docker 镜像 linux/amd64..."
  docker build --pull=false --platform linux/amd64 -t "$IMAGE" .
}

push_image() {
  echo "▶ 传输镜像到服务器..."
  IMAGE_SIZE=$(docker image inspect "$IMAGE" --format='{{.Size}}' 2>/dev/null || echo 0)
  if command -v pv >/dev/null 2>&1; then
    docker save "$IMAGE" | pv -s "$IMAGE_SIZE" -i 0.5 | gzip | ssh "$SERVER" "docker load"
  else
    echo "  (提示: brew install pv 可显示传输进度)"
    docker save "$IMAGE" | gzip | ssh "$SERVER" "docker load"
  fi
}

restart_remote() {
  echo "▶ 重启远端容器..."
  remote "cd '$DEPLOY_DIR' && docker compose -f '$COMPOSE_FILE' up -d --no-deps '$CONTAINER'"
  echo "▶ 远端容器状态:"
  remote "cd '$DEPLOY_DIR' && docker compose -f '$COMPOSE_FILE' ps"
}

show_logs() {
  remote "cd '$DEPLOY_DIR' && docker compose -f '$COMPOSE_FILE' logs -f --tail=200 '$CONTAINER'"
}

show_status() {
  remote "cd '$DEPLOY_DIR' && docker compose -f '$COMPOSE_FILE' ps && echo && docker logs --tail=80 '$CONTAINER'"
}

case "$TARGET" in
  init)
    ensure_local_tools
    init_remote
    ;;
  deploy)
    ensure_local_tools
    init_remote
    build_image
    push_image
    restart_remote
    ;;
  restart)
    ensure_local_tools
    restart_remote
    ;;
  logs)
    ensure_local_tools
    show_logs
    ;;
  status)
    ensure_local_tools
    show_status
    ;;
  *)
    echo "用法: ./deploy.sh <目标>"
    echo ""
    echo "目标:"
    echo "  deploy    本地构建镜像并部署到服务器"
    echo "  init      初始化服务器部署目录和配置模板"
    echo "  restart   重启服务器容器"
    echo "  logs      查看容器日志"
    echo "  status    查看容器状态"
    exit 1
    ;;
esac

echo ""
echo "================================================"
echo " 完成！"
echo "================================================"

