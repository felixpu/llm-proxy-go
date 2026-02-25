# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

LLM Proxy Go — 高性能 LLM 代理服务，提供智能路由、负载均衡、三层缓存和 Web 管理后台。Go 1.24+，纯 Go 编译（CGO_ENABLED=0），无 C 依赖。

## Build & Test Commands

```bash
make build                # 当前平台编译，产物 ./llm-proxy
make build-all            # 跨平台编译（linux/darwin/windows × amd64/arm64）
make test                 # 单元测试（= make test-unit）
make test-integration     # 集成测试（-tags=integration）
make test-e2e             # E2E 测试（-tags=e2e -timeout=120s）
make test-all             # 全部测试（unit + integration + e2e）
make test-coverage        # 覆盖率报告 → coverage.html
make docker               # Docker 镜像构建
make clean                # 清理构建产物

# 运行单个包测试
go test -v ./internal/service/...
go test -v -run TestFunctionName ./internal/repository/...

# 代码检查
go fmt ./...
go vet ./...
```

## Architecture

分层架构，依赖注入通过 `api.ServerDeps` 结构体传递：

```
cmd/llm-proxy/main.go          → 入口：配置加载 → DB → 迁移 → 仓储 → 服务 → HTTP Server → 优雅关闭
internal/api/server.go          → Gin 路由注册、中间件栈配置
internal/api/handler/           → HTTP 处理器（proxy, auth, user, apikey, admin_*, log 等）
internal/api/middleware/         → 认证(auth)、CSRF、速率限制(rate_limit)、日志+安全头(middleware)
internal/config/                → 三层优先级配置：环境变量 > SQLite > 默认值
internal/database/              → SQLite 连接（WAL 模式）+ 迁移（migrations/）
internal/models/                → 领域模型（domain.go, anthropic.go）
internal/repository/            → 数据访问层，接口定义在 interfaces.go
internal/service/               → 业务逻辑核心
internal/pkg/                   → 内部工具（contextutil, httputil, paths）
frontend/                       → Web 管理后台（go:embed 嵌入，Alpine.js + Vue 组件）
tests/                          → 集中式测试（e2e/, integration/, testutil/）
```

## Key Services (internal/service/)

- **ProxyService** — 核心代理：请求转发、流式响应、元数据收集（延迟/成本/Token）
- **AuthService** — API Key 验证 + Session 管理 + 默认管理员创建
- **HealthChecker** — 后台定期检查端点可用性，自动标记不健康端点
- **LoadBalancer** — 四种策略：round_robin / weighted / least_connections / conversation_hash
- **LLMRouter** — 基于嵌入向量的语义路由 + 条件解析
- **EndpointStore** — 从 Model+Provider 构建端点列表，运行时动态更新
- **WorkerCoordinator** — 多进程 Primary 选举、心跳、故障转移
- **CacheService** — L1 内存(bigcache) + L2 SQLite + L3 语义缓存
- **RoutingCache** — 内存缓存路由决策

## Configuration

三层优先级：环境变量 > SQLite 数据库 > 默认值。`.env` 文件可选，由 `config/loader.go` 自行解析（无第三方依赖）。

所有环境变量以 `LLM_PROXY_` 为前缀（日志级别除外用 `LOG_LEVEL`）。

## Database

SQLite（modernc.org/sqlite 纯 Go 驱动），WAL 模式。迁移文件在 `internal/database/migrations/`，启动时自动执行。

## Testing Conventions

- 单元测试与源码同目录（co-located），默认 build tag 排除 integration 和 e2e
- 集成测试用 `//go:build integration`
- E2E 测试用 `//go:build e2e`，位于 `tests/e2e/`
- 共享测试工具在 `tests/testutil/`（db, fixtures, http, assert）
- 测试框架：`stretchr/testify`

## Version Injection

版本信息通过 Makefile LDFLAGS 注入 `internal/version` 包，Makefile 是 LDFLAGS 的唯一定义处。构建时必须通过 `make build` 而非直接 `go build`。

## Code Style

- 代码注释使用英文（与现有代码库保持一致）
- README 和用户文档使用中文
