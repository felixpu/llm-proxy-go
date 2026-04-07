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

- Go 1.24+
- Make

### 安装

```bash
# 克隆仓库
git clone <repository-url>
cd llm-proxy-go

# 编译
make build
```

### 运行

```bash
# 初始化配置并启动（推荐）
./scripts/start.sh

# 或前台运行
./scripts/start.sh -f

# 或直接运行编译后的二进制
cp .env.example .env
# 编辑 .env 文件
./llm-proxy
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
LOG_LEVEL=INFO                      # 日志级别 (DEBUG/INFO/WARN/ERROR)
```

**数据库与目录配置**：
```bash
LLM_PROXY_DB=data/llm-proxy.db     # SQLite 数据库路径
LLM_PROXY_DATA_DIR=data             # 数据目录
LLM_PROXY_LOGS_DIR=logs             # 日志目录
```

**安全配置**：
```bash
LLM_PROXY_SECRET_KEY=your-secret-key       # Session 密钥
LLM_PROXY_SESSION_EXPIRE_HOURS=24          # Session 过期时间
LLM_PROXY_COOKIE_SECURE=false              # Cookie Secure 标志（HTTPS 下设为 true）
LLM_PROXY_DEFAULT_ADMIN_USERNAME=admin     # 默认管理员用户名
LLM_PROXY_DEFAULT_ADMIN_PASSWORD=admin123  # 默认管理员密码
```

**速率限制配置**：
```bash
LLM_PROXY_RATE_LIMIT_ENABLED=true          # 启用速率限制
LLM_PROXY_RATE_LIMIT_MAX_REQUESTS=100      # 每个时间窗口最大请求数
LLM_PROXY_RATE_LIMIT_WINDOW_SECONDS=60     # 时间窗口（秒）
LLM_PROXY_RATE_LIMIT_MAX_CLIENTS=10000     # 最大跟踪客户端 IP 数
```

**日志轮转配置**：
```bash
LLM_PROXY_LOG_MAX_SIZE_MB=10               # 单个日志文件最大体积（MB）
LLM_PROXY_LOG_MAX_BACKUPS=5                # 保留的旧日志文件数量
LLM_PROXY_LOG_MAX_AGE_DAYS=30              # 旧日志最大保留天数
LLM_PROXY_LOG_COMPRESS=true                # 是否 gzip 压缩旧日志
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

完整的配置示例请参考 `.env.example`。

## 项目结构

```
├── cmd/llm-proxy/               # 主入口
├── internal/
│   ├── api/
│   │   ├── handler/             # HTTP 处理器
│   │   └── middleware/          # 中间件（认证、CSRF、限流）
│   ├── config/                  # 配置管理
│   ├── database/                # 数据库连接与迁移
│   │   ├── migrations/          # SQL 迁移文件
│   │   └── sqlc/                # sqlc 生成代码
│   ├── models/                  # 数据模型
│   ├── repository/              # 数据访问层
│   ├── service/                 # 业务逻辑层
│   ├── version/                 # 版本信息
│   ├── pkg/                     # 内部工具包
│   ├── test/                    # 测试辅助
│   └── testutil/                # 测试工具
├── frontend/                    # 前端资源（go:embed 嵌入）
│   ├── css/                     # 样式文件
│   ├── js/vue/                  # Vue 组件、页面、Store
│   └── vendor/                  # 前端第三方库
├── scripts/
│   ├── build.sh                 # 发布包打包脚本
│   ├── start.sh                 # 启动/管理脚本（Linux/macOS）
│   └── start.bat                # 启动脚本（Windows）
├── sql/
│   ├── migrations/              # 数据库 schema 迁移
│   └── queries/                 # sqlc 查询定义
├── tests/
│   ├── e2e/                     # 端到端测试
│   ├── integration/             # 集成测试
│   └── testutil/                # 测试工具
├── configs/                     # 配置文件
├── bin/                         # 辅助工具
├── Makefile                     # 构建入口（LDFLAGS 唯一定义处）
├── Dockerfile                   # Docker 镜像构建
├── docker-compose.yml           # Docker Compose 编排
├── .env.example                 # 环境变量模板
├── go.mod
└── go.sum
```

## 开发工作流

### 1. 准备环境

```bash
# 安装依赖
go mod download

# 检查 Go 版本
go version  # 需要 1.24+
```

### 2. 开发前检查

```bash
# 代码格式化
go fmt ./...

# 代码检查
go vet ./...

# 运行测试
make test

# 编译检查
make build
```

### 3. 运行开发服务器

```bash
# 使用启动脚本（自动初始化 .env）
./scripts/start.sh -f

# 或使用 DEBUG 日志级别
LLM_PROXY_LOG_LEVEL=DEBUG ./llm-proxy
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
# 运行单元测试
make test

# 带覆盖率
make test-coverage

# 运行特定包的测试
go test -v ./internal/service/...
```

### 集成测试

```bash
make test-integration
```

### E2E 测试

```bash
make test-e2e
```

### 全部测试

```bash
make test-all
```

### 基准测试

```bash
# 运行基准测试
go test -bench=. ./internal/test/

# 带内存分析
go test -bench=. -benchmem ./internal/test/
```

## 构建

### 本地构建

```bash
# 当前平台编译
make build

# 查看版本信息
./llm-proxy --version
```

### 多平台构建

```bash
# 编译所有平台（linux/darwin/windows × amd64/arm64）
make build-all

# 编译指定平台
make build-linux-amd64
make build-linux-arm64
make build-darwin-amd64
make build-darwin-arm64
make build-windows-amd64
```

编译产物输出到 `dist/` 目录。

### 发布包

```bash
# 当前平台：编译 + 打包
make release

# 指定平台：编译 + 打包
make release-linux-amd64

# 所有平台：编译 + 打包
make release-all

# 清理构建产物
make clean
```

发布包结构：
```
llm-proxy-<ver>-<os>-<arch>/
├── llm-proxy (或 .exe)
├── start.sh (或 start.bat)
├── .env.example
├── README.txt
├── data/
└── logs/
```

## 部署

### Docker 部署

```bash
# 构建镜像
make docker

# 运行容器
docker run -d \
  -p 8000:8000 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/logs:/app/logs \
  -e LLM_PROXY_SECRET_KEY=your-secret-key \
  --name llm-proxy \
  llm-proxy:latest
```

### 发布包部署

```bash
# 解压发布包
tar -xzf llm-proxy-<ver>-<os>-<arch>.tar.gz
cd llm-proxy-<ver>-<os>-<arch>

# 初始化配置
cp .env.example .env
# 编辑 .env 文件

# 启动服务
./start.sh
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

日志轮转已内置（基于 lumberjack），默认配置：
- 单文件最大 10MB，超出自动轮转
- 保留最近 5 个旧日志文件
- 旧日志保留 30 天
- 自动 gzip 压缩

可通过环境变量调整：
```bash
LLM_PROXY_LOG_MAX_SIZE_MB=10
LLM_PROXY_LOG_MAX_BACKUPS=5
LLM_PROXY_LOG_MAX_AGE_DAYS=30
LLM_PROXY_LOG_COMPRESS=true
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

- [API 文档](http://localhost:8000/help)
- [Makefile 帮助](Makefile) — `make help` 查看所有可用命令
