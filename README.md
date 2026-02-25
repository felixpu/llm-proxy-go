# LLM Proxy - Go 版本

高性能 LLM 代理服务的 Go 语言实现，提供智能路由、负载均衡、缓存和管理功能。

## 特性

- 🚀 **高性能**：4-5x 吞吐量提升，60% 内存降低
- 🎯 **智能路由**：基于嵌入向量的语义路由
- ⚖️ **负载均衡**：支持轮询、加权、最少连接、会话哈希
- 💾 **三层缓存**：内存 + SQLite + 语义缓存
- 🔒 **安全认证**：API Key + Session Token + CSRF 保护
- 📊 **Web 管理**：完整的管理后台（Alpine.js + Go templates）
- 🔄 **多 Worker**：支持多进程协调和故障转移
- 📝 **请求日志**：完整的请求追踪和统计

## 快速开始

### 前置要求

- Go 1.21+
- SQLite 3

### 安装

```bash
# 克隆仓库
git clone <repository-url>
cd llm-proxy/go

# 安装依赖
go mod download

# 编译
go build -o llm-proxy cmd/llm-proxy/main.go
```

### 运行

```bash
# 使用默认配置运行
./llm-proxy

# 或使用 go run
go run cmd/llm-proxy/main.go

# 使用 .env 文件配置
cp ../.env.example ../.env
# 编辑 .env 文件
source ../.env
go run cmd/llm-proxy/main.go
```

服务将在 `http://localhost:8000` 启动（可通过 `LLM_PROXY_PORT` 环境变量修改）。

默认管理员账号：
- 用户名：`admin`
- 密码：`admin123`

## 配置

### 环境变量

配置优先级：环境变量 > SQLite 数据库 > 默认值

**服务配置**：
```bash
LLM_PROXY_HOST=0.0.0.0              # 监听地址
LLM_PROXY_PORT=8000                 # 监听端口
LLM_PROXY_WORKERS=1                 # Worker 数量
LLM_PROXY_LOG_LEVEL=INFO            # 日志级别 (DEBUG/INFO/WARN/ERROR)
```

**数据库配置**：
```bash
LLM_PROXY_DATABASE_PATH=data/llm-proxy.db  # SQLite 数据库路径
```

**安全配置**：
```bash
LLM_PROXY_SECRET_KEY=your-secret-key       # Session 密钥
LLM_PROXY_SESSION_EXPIRE_HOURS=24          # Session 过期时间
LLM_PROXY_DEFAULT_ADMIN_USERNAME=admin     # 默认管理员用户名
LLM_PROXY_DEFAULT_ADMIN_PASSWORD=admin123  # 默认管理员密码
```

**健康检查配置**：
```bash
LLM_PROXY_HEALTH_CHECK_ENABLED=true        # 启用健康检查
LLM_PROXY_HEALTH_CHECK_INTERVAL=60         # 检查间隔（秒）
LLM_PROXY_HEALTH_CHECK_TIMEOUT=10          # 超时时间（秒）
```

**负载均衡配置**：
```bash
LLM_PROXY_LOAD_BALANCE_STRATEGY=weighted   # 策略：round_robin/weighted/least_connections/conversation_hash
```

### 配置文件

完整的配置示例请参考 `../.env.example`。

## 项目结构

```
go/
├── cmd/
│   └── llm-proxy/
│       └── main.go              # 主入口
├── internal/
│   ├── api/
│   │   ├── handler/             # HTTP 处理器
│   │   │   ├── templates/       # Go html/template 模板
│   │   │   ├── static/          # 静态资源（CSS/JS）
│   │   │   ├── admin_*.go       # 管理后台 API
│   │   │   ├── apikey.go        # API Key 管理
│   │   │   ├── logs.go          # 日志查询
│   │   │   ├── proxy.go         # 代理处理
│   │   │   ├── ui.go            # Web UI
│   │   │   └── user.go          # 用户管理
│   │   ├── middleware/          # 中间件
│   │   │   ├── auth.go          # 认证中间件
│   │   │   ├── csrf.go          # CSRF 保护
│   │   │   ├── middleware.go    # 日志中间件
│   │   │   └── rate_limit.go    # 速率限制
│   │   └── server.go            # HTTP 服务器
│   ├── config/                  # 配置管理
│   │   ├── config.go
│   │   └── loader.go
│   ├── database/                # 数据库
│   │   ├── db.go                # 连接管理
│   │   └── migrations.go        # 迁移管理
│   ├── models/                  # 数据模型
│   ├── repository/              # 数据访问层（16 个 Repository）
│   ├── service/                 # 业务逻辑层
│   │   ├── auth.go              # 认证服务
│   │   ├── cache_service.go     # 三层缓存
│   │   ├── embedding_service.go # 嵌入向量服务
│   │   ├── health_checker.go    # 健康检查
│   │   ├── llm_router.go        # 智能路由
│   │   ├── load_balancer.go     # 负载均衡
│   │   ├── log_service.go       # 日志服务
│   │   ├── proxy.go             # 代理服务（含 SSE 流式）
│   │   └── worker_coordinator.go # Worker 协调
│   └── pkg/                     # 工具包
│       ├── contextutil/         # Context 工具
│       ├── httputil/            # HTTP 工具
│       └── paths/               # 路径管理
├── sql/
│   └── migrations/
│       └── 001_initial_schema.sql  # 初始数据库 schema
├── go.mod
├── go.sum
└── README.md                    # 本文件
```

## 开发工作流

### 1. 准备环境

```bash
# 安装依赖
go mod download

# 检查 Go 版本
go version  # 需要 1.21+
```

### 2. 开发前检查

```bash
# 代码格式化
go fmt ./...

# 代码检查
go vet ./...

# 运行测试
go test ./...

# 编译检查
go build ./...
```

### 3. 运行开发服务器

```bash
# 使用 .env 配置
source ../.env
go run cmd/llm-proxy/main.go

# 或使用 DEBUG 日志级别
LLM_PROXY_LOG_LEVEL=DEBUG go run cmd/llm-proxy/main.go
```

### 4. 查看日志

```bash
# 实时查看日志
tail -f logs/llm-proxy.log

# 查看错误日志
tail -f logs/llm-proxy-error.log
```

### 5. 热重载（可选）

```bash
# 安装 air（热重载工具）
go install github.com/cosmtrek/air@latest

# 运行
air
```

## 测试

### 单元测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./internal/service/...

# 带覆盖率
go test -cover ./...

# 详细输出
go test -v ./...
```

### 基准测试

```bash
# 运行基准测试
go test -bench=. ./internal/test/

# 带内存分析
go test -bench=. -benchmem ./internal/test/
```

### E2E 测试

```bash
# 运行端到端测试
go test -v ./internal/test/ -run TestE2E
```

## 构建

### 本地构建

```bash
# 基本构建
go build -o llm-proxy cmd/llm-proxy/main.go

# 优化构建（减小体积）
go build -ldflags="-s -w" -o llm-proxy cmd/llm-proxy/main.go

# 静态链接（无外部依赖）
CGO_ENABLED=0 go build -ldflags="-s -w" -o llm-proxy cmd/llm-proxy/main.go
```

### 多平台构建

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -o llm-proxy-linux-amd64 cmd/llm-proxy/main.go

# Linux arm64
GOOS=linux GOARCH=arm64 go build -o llm-proxy-linux-arm64 cmd/llm-proxy/main.go

# macOS amd64
GOOS=darwin GOARCH=amd64 go build -o llm-proxy-darwin-amd64 cmd/llm-proxy/main.go

# macOS arm64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o llm-proxy-darwin-arm64 cmd/llm-proxy/main.go

# Windows amd64
GOOS=windows GOARCH=amd64 go build -o llm-proxy-windows-amd64.exe cmd/llm-proxy/main.go
```

### 使用 GoReleaser

```bash
# 安装 goreleaser
go install github.com/goreleaser/goreleaser@latest

# 本地构建（不发布）
goreleaser build --snapshot --clean

# 发布（需要 Git tag）
git tag -a v1.0.0 -m "Release v1.0.0"
goreleaser release --clean
```

## 部署

### Docker 部署

```bash
# 构建镜像
docker build -t llm-proxy:latest -f ../Dockerfile .

# 运行容器
docker run -d \
  -p 8000:8000 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/logs:/app/logs \
  -e LLM_PROXY_PORT=8000 \
  --name llm-proxy \
  llm-proxy:latest
```

### Docker Compose 部署

```bash
# 启动服务
docker-compose -f ../docker-compose.yml up -d

# 查看日志
docker-compose -f ../docker-compose.yml logs -f

# 停止服务
docker-compose -f ../docker-compose.yml down
```

### 系统服务部署

创建 systemd 服务文件 `/etc/systemd/system/llm-proxy.service`：

```ini
[Unit]
Description=LLM Proxy Service
After=network.target

[Service]
Type=simple
User=llm-proxy
WorkingDirectory=/opt/llm-proxy
ExecStart=/opt/llm-proxy/llm-proxy
Restart=on-failure
RestartSec=5s

Environment="LLM_PROXY_PORT=8000"
Environment="LLM_PROXY_LOG_LEVEL=INFO"

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable llm-proxy
sudo systemctl start llm-proxy
sudo systemctl status llm-proxy
```

## API 文档

### 健康检查

```bash
GET /api/health
```

### 代理请求

```bash
POST /v1/chat/completions
Headers:
  Authorization: Bearer <api-key>
  Content-Type: application/json
Body:
  {
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": false
  }
```

### 管理 API

所有管理 API 需要登录认证。

**用户管理**：
- `GET /api/users` - 获取用户列表
- `POST /api/users` - 创建用户
- `PUT /api/users/:id` - 更新用户
- `DELETE /api/users/:id` - 删除用户

**API Key 管理**：
- `GET /api/apikeys` - 获取 API Key 列表
- `POST /api/apikeys` - 创建 API Key
- `DELETE /api/apikeys/:id` - 删除 API Key

**日志查询**：
- `GET /api/logs` - 查询请求日志
- `DELETE /api/logs` - 清除日志

更多 API 文档请参考 Web 管理界面的帮助页面。

## 性能优化

### 缓存配置

三层缓存架构：

1. **L1 内存缓存**（bigcache）：
   - 最快，但容量有限
   - 适合热点数据

2. **L2 SQLite 缓存**：
   - 持久化，容量大
   - 适合常用数据

3. **L3 语义缓存**：
   - 基于嵌入向量相似度
   - 适合相似查询

### Worker 配置

```bash
# 单 Worker（默认）
LLM_PROXY_WORKERS=1

# 多 Worker（需要 Primary 选举）
LLM_PROXY_WORKERS=4
```

多 Worker 模式下：
- 自动选举 Primary Worker
- 心跳检测（10 秒间隔）
- 故障转移（30 秒超时）

### 数据库优化

```bash
# 增加连接池大小
LLM_PROXY_DATABASE_MAX_OPEN_CONNS=50
LLM_PROXY_DATABASE_MAX_IDLE_CONNS=10
```

## 故障排查

### 常见问题

**1. 端口被占用**

```bash
# 查看占用端口的进程
lsof -i :8000

# 杀掉进程
kill -9 <PID>
```

**2. 数据库锁定**

```bash
# 检查数据库文件权限
ls -la data/llm-proxy.db

# 删除锁文件
rm -f data/llm-proxy.db-shm data/llm-proxy.db-wal
```

**3. 日志文件过大**

```bash
# 清理日志
> logs/llm-proxy.log
> logs/llm-proxy-error.log

# 或使用 logrotate
```

**4. 静态资源 404**

确保 `internal/api/handler/static/` 目录存在且包含所有静态文件。

### 调试模式

```bash
# 启用 DEBUG 日志
LLM_PROXY_LOG_LEVEL=DEBUG go run cmd/llm-proxy/main.go

# 使用 delve 调试器
dlv debug cmd/llm-proxy/main.go
```

## 迁移指南

从 Python 版本迁移到 Go 版本：

1. **数据库兼容**：Go 版本使用相同的 SQLite schema，数据可直接迁移
2. **配置兼容**：环境变量名称保持一致
3. **API 兼容**：所有 API 端点保持兼容
4. **性能提升**：预期 4-5x 吞吐量提升，60% 内存降低

## 贡献

欢迎贡献代码！请遵循以下步骤：

1. Fork 仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

## 许可证

[许可证信息]

## 相关链接

- [Python 版本](../)
- [可行性分析文档](../docs/plans/go-refactoring-feasibility-analysis.md)
- [API 文档](http://localhost:8000/help)
