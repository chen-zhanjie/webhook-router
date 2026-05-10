# Deployment Guide

## Current Server

```text
120.79.241.27
```

Deployment directory:

```text
/opt/webhook-router
```

Container port:

```text
18080
```

Local reverse proxy target:

```text
http://127.0.0.1:18080
```

## Deploy From Local Machine

```bash
cd webhook-router
./deploy.sh deploy
```

The script:

1. Builds `webhook-router:latest` locally for `linux/amd64`
2. Streams the image to the server using `docker save | gzip | ssh docker load`
3. Writes Docker Compose file to `/opt/webhook-router/docker-compose.yml`
4. Creates `/opt/webhook-router/config.yaml` only if missing
5. Restarts the `webhook-router` container

The script never overwrites an existing remote `config.yaml`.

## 1Panel Redis

Current server Redis is managed by 1Panel.

Network:

```text
1panel-network
```

Redis alias in that network:

```text
redis
```

App config should use:

```yaml
redis:
  addr: "redis:6379"
  password: "configured-only-on-server"
```

Do not commit the Redis password.

## Nginx / OpenResty Reverse Proxy

Use this upstream target:

```text
http://127.0.0.1:18080
```

Suggested config:

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

`/apps/` is the SSE long-connection path. Proxy buffering must be disabled.

## Server Commands

Status:

```bash
cd /opt/webhook-router
docker compose ps
curl http://127.0.0.1:18080/healthz
curl http://127.0.0.1:18080/stats
```

Logs:

```bash
docker logs -f --tail=200 webhook-router
```

Restart after config changes:

```bash
cd /opt/webhook-router
docker compose restart webhook-router
```

## Upgrade

Run from local machine:

```bash
./deploy.sh deploy
```

Upgrade behavior:

- Replaces the image/container
- Reuses existing `/opt/webhook-router/config.yaml`
- Reuses existing 1Panel Redis
- Does not overwrite secrets

