# auth-pro 应用镜像。构建上下文必须是仓库根目录：
#   docker build -f deploy/docker/Dockerfile -t auth-pro:1.9.1 .
# 或使用 compose（见 docker-compose.yml）。
#
# 两个目标：
#   runtime（默认）— 从本仓库多阶段编译前端与 linux/amd64 后端
#   from-package  — 解压已有 auth_pro-full-vX.Y.Z.tar.gz（构建参数 PACKAGE_PATH）

# ---------- 前端 ----------
FROM node:20-bookworm AS frontend
WORKDIR /src
ENV HUSKY=0
RUN corepack enable
COPY frontend/package.json frontend/pnpm-lock.yaml ./frontend/
COPY scripts/check-unused-exports.mjs ./scripts/check-unused-exports.mjs
WORKDIR /src/frontend
RUN pnpm install --frozen-lockfile
WORKDIR /src
COPY frontend ./frontend
COPY VERSION ./VERSION
ARG AUTH_PRO_VERSION=
RUN VERSION="${AUTH_PRO_VERSION:-$(tr -d '[:space:]' < VERSION)}" \
  && VITE_VERSION="$VERSION" pnpm -C frontend run build \
  && test -f frontend/dist/index.html \
  && test -f frontend/dist/version.json

# ---------- 后端 ----------
FROM golang:1.22-bookworm AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /src/backend
RUN go mod download
WORKDIR /src
COPY backend ./backend
COPY sdk ./sdk
COPY VERSION ./VERSION
COPY --from=frontend /src/frontend/dist/ ./frontend-dist/
ARG AUTH_PRO_VERSION=
RUN set -euo pipefail \
  && VERSION="${AUTH_PRO_VERSION:-$(tr -d '[:space:]' < VERSION)}" \
  && rm -rf backend/static \
  && mkdir -p backend/static \
  && cp -R frontend-dist/. backend/static/ \
  && mkdir -p backend/handler/sdk_assets/_meta backend/handler/sdk_assets/go \
  && cp -a sdk/php sdk/node sdk/python sdk/browser backend/handler/sdk_assets/ \
  && cp -a sdk/go/authpro backend/handler/sdk_assets/go/ \
  && cp sdk/go/go.mod backend/handler/sdk_assets/_meta/go.mod.txt \
  && rm -rf backend/handler/sdk_assets/python/authpro/__pycache__ \
  && rm -f backend/handler/sdk_assets/go/authpro/*_test.go \
  && BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
  && LDFLAGS="-s -w -X auto_pro/config.AppVersion=${VERSION} -X auto_pro/config.BuildTime=${BUILD_TIME}" \
  && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go -C backend build -trimpath -ldflags "$LDFLAGS" -o /tmp/auth_pro . \
  && mkdir -p /out/site/backend \
  && cp -R frontend-dist/. /out/site/ \
  && cp /tmp/auth_pro /out/site/backend/auth_pro \
  && chmod 755 /out/site/backend/auth_pro

# ---------- 运行镜像（源码构建，默认）----------
FROM debian:bookworm-slim AS runtime
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl tzdata \
  && rm -rf /var/lib/apt/lists/* \
  && groupadd --system --gid 10001 authpro \
  && useradd --system --uid 10001 --gid authpro --home-dir /data --shell /usr/sbin/nologin authpro \
  && mkdir -p /app /data \
  && chown -R authpro:authpro /app /data

COPY --from=backend /out/site/ /app/
ENV HOST=0.0.0.0 \
    PORT=19127 \
    AUTO_PRO_DATA_DIR=/data \
    AUTO_PRO_FRONTEND_DIR=/app \
    TZ=Asia/Shanghai
WORKDIR /data
USER authpro
EXPOSE 19127
HEALTHCHECK --interval=30s --timeout=5s --start-period=40s --retries=3 \
  CMD curl -fsS "http://127.0.0.1:${PORT}/api/install/status" >/dev/null || exit 1
CMD ["/app/backend/auth_pro"]

# ---------- 运行镜像（官网发布包）----------
FROM debian:bookworm-slim AS from-package
ARG PACKAGE_PATH=release/packages/auth_pro-full-v1.9.1.tar.gz
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl tzdata tar gzip \
  && rm -rf /var/lib/apt/lists/* \
  && groupadd --system --gid 10001 authpro \
  && useradd --system --uid 10001 --gid authpro --home-dir /data --shell /usr/sbin/nologin authpro \
  && mkdir -p /app /data
COPY ${PACKAGE_PATH} /tmp/auth_pro_package.tar.gz
RUN tar -xzf /tmp/auth_pro_package.tar.gz -C /app \
  && rm -f /tmp/auth_pro_package.tar.gz \
  && test -x /app/backend/auth_pro \
  && test -f /app/index.html \
  && chown -R authpro:authpro /app /data
ENV HOST=0.0.0.0 \
    PORT=19127 \
    AUTO_PRO_DATA_DIR=/data \
    AUTO_PRO_FRONTEND_DIR=/app \
    TZ=Asia/Shanghai
WORKDIR /data
USER authpro
EXPOSE 19127
HEALTHCHECK --interval=30s --timeout=5s --start-period=40s --retries=3 \
  CMD curl -fsS "http://127.0.0.1:${PORT}/api/install/status" >/dev/null || exit 1
CMD ["/app/backend/auth_pro"]
