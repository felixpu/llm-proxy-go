# Go 测试文件重组指南

## 概述

Go 项目的测试文件已按照业务功能层级重新组织，使用 Go 的 build tag 机制来分类不同类型的测试。

## 测试结构

```
go/
├── internal/                    # 源码 + 单元测试（co-located）
│   ├── api/handler/
│   │   ├── user.go
│   │   └── user_test.go         # //go:build !integration && !e2e
│   ├── service/
│   │   ├── proxy.go
│   │   ├── proxy_test.go        # //go:build !integration && !e2e
│   │   └── routing_integration_test.go  # //go:build integration
│   └── repository/
│       ├── user_repo.go
│       └── user_repo_test.go    # //go:build !integration && !e2e
│
├── tests/                       # 集中管理非单元测试
│   ├── integration/             # 集成测试（跨层、跨模块）
│   │   └── [未来扩展]
│   ├── e2e/                     # 端到端测试
│   │   ├── e2e_test.go          # //go:build e2e
│   │   ├── benchmark.go
│   │   ├── benchmark_runner.go
│   │   ├── consistency.go
│   │   └── exporter.go
│   └── testutil/                # 共享测试工具
│       ├── db.go                # 数据库测试工具
│       ├── fixtures.go          # 测试数据
│       ├── http.go              # HTTP 测试工具
│       └── assert.go            # 断言工具
│
└── Makefile                     # 测试运行入口
```

## 测试分类

### 1. 单元测试（Unit Tests）
- **位置**：与源码同目录（co-located）
- **Build Tag**：`//go:build !integration && !e2e`
- **特点**：
  - 测试单个函数/方法
  - 可访问未导出符号（白盒测试）
  - 快速执行
  - 无外部依赖

**运行命令**：
```bash
make test              # 默认运行单元测试
make test-unit        # 显式运行单元测试
go test ./internal/...
```

### 2. 集成测试（Integration Tests）
- **位置**：`internal/service/routing_integration_test.go`（需要访问未导出符号）
- **Build Tag**：`//go:build integration`
- **特点**：
  - 测试多个组件的交互
  - 跨层级测试（如 service + repository）
  - 需要数据库
  - 执行时间较长

**运行命令**：
```bash
make test-integration
go test ./internal/... -tags=integration
```

### 3. E2E 测试（End-to-End Tests）
- **位置**：`tests/e2e/`
- **Build Tag**：`//go:build e2e`
- **Package**：`e2e_test`（黑盒测试）
- **特点**：
  - 测试完整的用户流程
  - 通过公开 API 测试
  - 需要完整的应用环境
  - 执行时间最长

**运行命令**：
```bash
make test-e2e
go test ./tests/e2e/... -tags=e2e -timeout=120s
```

## 使用 Makefile

### 可用命令

```bash
# 单元测试（默认）
make test

# 显式运行各类测试
make test-unit          # 仅单元测试
make test-integration   # 仅集成测试
make test-e2e          # 仅 E2E 测试
make test-all          # 所有测试

# 覆盖率报告
make test-coverage     # 生成 coverage.html

# 帮助
make help
```

## Build Tag 说明

### 单元测试
```go
//go:build !integration && !e2e
// +build !integration,!e2e

package handler
```
- 默认运行（不需要特殊 tag）
- 排除集成和 E2E 测试

### 集成测试
```go
//go:build integration
// +build integration

package service
```
- 需要 `-tags=integration` 才能运行
- 可访问未导出符号（白盒）

### E2E 测试
```go
//go:build e2e
// +build e2e

package e2e_test
```
- 需要 `-tags=e2e` 才能运行
- 黑盒测试，仅使用公开 API

## 迁移说明

### 已完成的迁移

1. **testutil 迁移**
   - 从 `internal/testutil/` → `tests/testutil/`
   - 所有 import 路径已更新
   - 所有源码文件已更新

2. **Build Tag 添加**
   - 所有单元测试：`//go:build !integration && !e2e`
   - 集成测试：`//go:build integration`
   - E2E 测试：`//go:build e2e`

3. **E2E 测试重组**
   - 从 `internal/test/` → `tests/e2e/`
   - Package 改为 `e2e_test`
   - 添加 `//go:build e2e` tag

### 为什么单元测试保持 co-located？

Go 的白盒测试（`package xxx`）必须与源码同目录，因为：
- 需要访问未导出的函数、字段、类型
- 这是 Go 的标准约定
- 提高代码内聚性

使用 build tag 而不是物理迁移，既保持了 Go 的最佳实践，又实现了逻辑分类。

## 最佳实践

### 编写单元测试
```go
//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import "testing"

func TestUserHandler_GetUser(t *testing.T) {
    // 测试代码
}
```

### 编写集成测试
```go
//go:build integration
// +build integration

package service

import "testing"

func TestIntegration_UserFlow(t *testing.T) {
    // 跨层测试代码
}
```

### 编写 E2E 测试
```go
//go:build e2e
// +build e2e

package e2e_test

import "testing"

func TestE2E_CompleteUserJourney(t *testing.T) {
    // 完整流程测试
}
```

## 常见问题

### Q: 为什么单元测试没有被迁移到 tests/unit/?
A: Go 的白盒测试必须与源码同目录才能访问未导出符号。这是 Go 的设计约定。使用 build tag 实现了逻辑分类，同时保持了 Go 的最佳实践。

### Q: 如何在 IDE 中运行特定类型的测试？
A: 在 IDE 的 Go 配置中添加 build flags：
- GoLand/IntelliJ: Settings → Go → Build Tags & Vendoring
- VS Code: 在 settings.json 中配置 `go.buildFlags`

### Q: 集成测试为什么还在 internal/service/?
A: 因为它需要访问 service 包的未导出符号（如 `GetCacheKey`、`routingCache`）。这是白盒集成测试的特点。

### Q: 如何在 CI/CD 中运行不同的测试？
A: 使用 Makefile 或直接使用 go test 命令：
```bash
# CI 流程示例
make test-unit          # 快速反馈
make test-integration   # 深度验证
make test-e2e          # 完整验证
```

## 性能指标

根据最近的测试运行：
- **单元测试**：~0.6s（models 包）
- **集成测试**：~0.5s（service 包）
- **E2E 测试**：取决于应用启动时间

## 相关文件

- `Makefile` - 测试运行脚本
- `tests/testutil/` - 共享测试工具库
- `tests/e2e/` - E2E 测试框架
