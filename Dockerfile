# LLM Proxy Go - Docker 镜像构建文件
# 多阶段构建，CGO 编译 sqlite3

# ============ 构建阶段 ============
FROM golang:1.24-bookworm AS builder

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

WORKDIR /build

# 先复制依赖文件利用 Docker 缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源码
COPY . .

# 编译（纯 Go，无需 CGO）
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w \
        -X 'github.com/user/llm-proxy-go/internal/version.Version=${VERSION}' \
        -X 'github.com/user/llm-proxy-go/internal/version.GitCommit=${GIT_COMMIT}' \
        -X 'github.com/user/llm-proxy-go/internal/version.BuildTime=${BUILD_TIME}'" \
    -o llm-proxy ./cmd/llm-proxy

# ============ 运行阶段 ============
FROM debian:bookworm-slim AS runtime

WORKDIR /app

# 安装运行时依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# 创建非 root 用户
RUN useradd --create-home --shell /bin/bash llmproxy

# 从构建阶段复制二进制
COPY --from=builder /build/llm-proxy /app/llm-proxy

# 创建数据和日志目录
RUN mkdir -p /app/data /app/logs && \
    chown -R llmproxy:llmproxy /app

USER llmproxy

# 环境变量
ENV LLM_PROXY_HOST=0.0.0.0
ENV LLM_PROXY_PORT=8000
ENV LLM_PROXY_DB=/app/data/llm-proxy.db
ENV LLM_PROXY_DATA_DIR=/app/data
ENV LLM_PROXY_LOGS_DIR=/app/logs
ENV LLM_PROXY_SECRET_KEY=change-this-secret-key-in-production
ENV LLM_PROXY_DEFAULT_ADMIN_USERNAME=admin
ENV LLM_PROXY_DEFAULT_ADMIN_PASSWORD=admin123

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/api/health || exit 1

CMD ["/app/llm-proxy"]
