# Windows 服务卡死问题 — 诊断与重构计划

**日期**: 2026-04-07
**现象**: Windows 构建物部署在 118.195.206.150:18000，运行一段时间后访问 Web 页面卡死，必须重启服务才能恢复。
**状态**: 诊断完成，待验证

---

## 一、问题定位

### P0-1: 异步日志写入 goroutine 无限堆积（最可能的根因）

**位置**: `internal/service/proxy.go:463-471`

```go
go func() {
    saveCtx, cancel := context.WithTimeout(context.Background(), DefaultAsyncRepoTimeout)  // 5s
    defer cancel()
    if _, err := s.logRepo.Insert(saveCtx, entry); err != nil {
        s.logger.Error("failed to save request log", ...)
    }
}()
```

**问题**:
- **每个代理请求都会 spawn 一个 goroutine** 来写日志
- 写连接池只有 1 个连接（`database/db.go:41`），所有 goroutine 排队竞争
- SQLite busy_timeout 5s + context timeout 5s = 每个 goroutine 最多存活 5s
- **但如果请求持续涌入**，goroutine 堆积速度 > 消化速度，内存持续增长
- Windows 上 SQLite 文件锁竞争比 Linux 更激烈（NTFS vs POSIX 锁），加剧此问题
- **没有任何反压机制**：不管当前排队多少，新请求仍然创建新 goroutine

**同类问题还出现在**（全部使用相同的无限制 `go func()` 模式）:
- `internal/service/auth.go:81-87` — UpdateLastUsed（每次 API Key 认证）
- `internal/service/llm_router.go:198-206` — IncrementHitCount（每次路由规则匹配）
- `internal/service/routing_decision_cache.go:61-67` — UpdateHitCountByHash（每次 L2 缓存命中）

**影响链**:
```
持续请求 → goroutine 堆积（每个等 5s 获取写连接）
→ 内存增长 + OS 线程增长 → Go 运行时调度延迟 → HTTP handler 也变慢
→ 更多 goroutine 堆积 → 恶性循环 → 服务无响应
```

### P0-2: RateLimit 清理 goroutine 无法停止 + IP map 无上限

**位置**: `internal/api/middleware/rate_limit.go:101-119`

```go
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    for {
        if cfg.StopCh != nil {   // ← 永远是 nil！
            ...
        } else {
            select {
            case <-ticker.C:
                limiter.cleanup()
            }
        }
    }
}()
```

**位置**: `cmd/llm-proxy/main.go:276-281`

```go
RateLimit: &middleware.RateLimitConfig{
    Enabled:       cfg.RateLimit.Enabled,
    MaxRequests:   cfg.RateLimit.MaxRequests,
    WindowSeconds: cfg.RateLimit.WindowSeconds,
    ExemptPaths:   middleware.DefaultRateLimitConfig().ExemptPaths,
    // StopCh 未设置 → nil
},
```

**问题**:
1. `StopCh` 始终为 nil，后台 goroutine **永远无法退出**（应用生命周期内持续运行）
2. `rl.requests` 是 `map[string][]time.Time`，**无容量上限**
3. cleanup 只删除过期条目，不限制 map 总大小
4. 如果有大量不同 IP 访问（爬虫、扫描器），map 会持续膨胀

### P0-3: 写数据库连接池 MaxOpenConns=1 的瓶颈

**位置**: `internal/database/db.go:41-42`

```go
conn.SetMaxOpenConns(1)
conn.SetMaxIdleConns(1)
```

**问题**:
- SQLite WAL 模式本身只允许一个写入者，`MaxOpenConns(1)` 看似合理
- 但 Go `database/sql` 的连接池在 `MaxOpenConns(1)` 时会**串行化所有写操作**
- 当多个 goroutine 等待连接时，它们都阻塞在 `sql.DB.conn()` 上
- 与 P0-1 的 goroutine 堆积叠加，形成致命的资源争夺

**另外**: `config.go:136-138` 定义了 `ConnMaxLifetime: 5 * time.Minute`，但 **db.go 中从未应用**。长连接可能导致 SQLite WAL checkpoint 延迟。

### P1-1: 流式 channel 客户端断开后 goroutine 残留

**位置**: `internal/service/proxy.go:566-571` + `internal/api/handler/proxy.go:348-354`

**service 侧**:
```go
chunkChan := make(chan StreamChunk, 100)
go s.readSSEStream(ctx, resp, ep, epName, attemptStart, meta, chunkChan)
```

**handler 侧**:
```go
case <-clientGone:
    h.logger.Debug("client disconnected during stream", ...)
    return  // ← 直接 return，不 drain chunkChan
```

**问题**:
- 客户端断开 → handler return → 没人消费 chunkChan
- `readSSEStream` 尝试 `chunkChan <- chunk` → 缓冲区满(100)后阻塞
- 虽然 `readSSEStream` 有 `select { case <-ctx.Done() }` 检查，但只在循环头检查
- **如果 `reader.ReadBytes('\n')` 阻塞在 I/O 上**，ctx.Done 不会被检查
- 该 goroutine 会一直活着，直到上游关闭连接或 `resp.Body` 超时
- `streamClient` 的 `Timeout: 0`（无超时），意味着这个 goroutine **可能永远不会结束**

### P1-2: Shutdown 时无法回收异步 goroutine

**位置**: `cmd/llm-proxy/main.go:192-197`

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
if err := httpServer.Shutdown(ctx); err != nil {
    return fmt.Errorf("server shutdown: %w", err)
}
```

**问题**:
- `httpServer.Shutdown()` 只等待活跃 HTTP 请求完成
- 所有 `go func()` 启动的后台任务（日志写入、命中计数更新等）**不受管理**
- 没有 `sync.WaitGroup` 或类似机制来等待它们
- shutdown 时可能有正在写入的日志被截断，或数据库连接已关闭但 goroutine 仍在写入

### P2-1: streamClient 无超时

**位置**: `internal/service/proxy.go:90-97`

```go
streamClient: &http.Client{
    Timeout: 0, // No timeout for streaming
    ...
},
```

**问题**:
- 流式请求对上游无任何超时限制
- 如果上游变慢（throttling、网络问题），连接会永远保持
- 结合 P1-1，造成 goroutine + TCP 连接双重泄漏

### P2-2: ConnMaxLifetime 配置未生效

**位置**: `internal/config/config.go:138` vs `internal/database/db.go`

```go
// config.go:138
ConnMaxLifetime: 5 * time.Minute,

// db.go — 没有调用 conn.SetConnMaxLifetime()
```

**问题**:
- 长期运行的连接不会被回收
- 在 Windows 上可能导致 SQLite WAL 文件持续增长（checkpoint 延迟）

---

## 二、根因总结

```
                    ┌──────────────────────────┐
                    │  持续的 API 请求流        │
                    └──────────┬───────────────┘
                               │
              ┌────────────────┼────────────────┐
              ▼                ▼                 ▼
    SaveRequestLog()    UpdateLastUsed()   IncrementHitCount()
    (go func)           (go func)          (go func)
              │                │                 │
              └────────────────┼────────────────┘
                               ▼
                  ┌────────────────────────┐
                  │  SQLite 写连接池 (1个)  │
                  │  busy_timeout=5s       │
                  └────────────────────────┘
                               │
                    Windows NTFS 文件锁 ← 更慢
                               │
                  goroutine 排队 → 堆积 → 内存↑
                               │
                  Go 运行时调度变慢 → HTTP 响应变慢
                               │
                  Web UI 请求也排在后面 → 页面卡死
```

**辅助因素**:
- RateLimit IP map 无上限 → 内存缓慢增长
- streamClient 无超时 → 僵尸 goroutine + 连接
- ConnMaxLifetime 未生效 → WAL 文件膨胀
- Shutdown 无 goroutine 回收 → 重启时可能数据丢失

---

## 三、修复方案（待确认后实施）

### 方案 A: 有界 Worker Pool（推荐）

替换所有无限制的 `go func()` 为统一的 `AsyncWorkerPool`：

```go
type AsyncWorkerPool struct {
    ch     chan func()
    wg     sync.WaitGroup
    logger *zap.Logger
}

func NewAsyncWorkerPool(size int, logger *zap.Logger) *AsyncWorkerPool {
    pool := &AsyncWorkerPool{
        ch:     make(chan func(), size*2), // 缓冲队列
        logger: logger,
    }
    for i := 0; i < size; i++ {
        pool.wg.Add(1)
        go pool.worker()
    }
    return pool
}

func (p *AsyncWorkerPool) Submit(fn func()) {
    select {
    case p.ch <- fn:
    default:
        p.logger.Warn("async worker pool full, dropping task")
    }
}

func (p *AsyncWorkerPool) Shutdown() {
    close(p.ch)
    p.wg.Wait()
}
```

**改造点**:
- `proxy.go:463` — SaveRequestLog 改为 `pool.Submit(...)`
- `auth.go:81` — UpdateLastUsed 改为 `pool.Submit(...)`
- `llm_router.go:198` — IncrementHitCount 改为 `pool.Submit(...)`
- `routing_decision_cache.go:61` — UpdateHitCountByHash 改为 `pool.Submit(...)`
- `main.go` — 初始化时创建 pool，shutdown 时 `pool.Shutdown()`

### 方案 B: RateLimit 修复

1. 传入 `StopCh` 以支持优雅关闭
2. 给 IP map 加上限（如 10000 个条目）
3. cleanup 时如果超过上限，LRU 淘汰最早的条目

### 方案 C: 流式请求修复

1. `streamClient` 增加 `ResponseHeaderTimeout`（如 120s）
2. 客户端断开时取消上下文（ctx cancel）确保 readSSEStream 退出
3. readSSEStream 中使用 `context.Reader` 包装 resp.Body

### 方案 D: 数据库连接修复

1. 应用 `ConnMaxLifetime` 配置
2. 考虑写连接池增加到 2-3（仍然是串行写入但减少等待）

### 方案 E: Shutdown 增强

1. main.go 中 shutdown 时先 `pool.Shutdown()` 再关闭数据库
2. 给 RateLimit 的 StopCh 发信号

---

## 四、验证方法

### 部署前验证

1. 添加 pprof 端点，可远程检查 goroutine 数量
2. 添加 DB 连接池状态日志（每 30s 打印 OpenConnections, InUse, WaitCount）
3. 在本地长时间压测模拟

### 部署后验证

1. 定期检查 `http://host:18000/debug/pprof/goroutine?debug=1`
2. 观察日志中的 DB 连接池状态
3. 确认 goroutine 数量稳定，不会随时间线性增长

---

## 五、优先级排序

| 优先级 | 问题 | 修复方案 | 预期效果 |
|--------|------|---------|---------|
| **P0** | goroutine 无限堆积 | 方案 A: Worker Pool | 消除卡死根因 |
| **P0** | RateLimit 不可停止 + 无上限 | 方案 B | 消除内存泄漏 |
| **P0** | ConnMaxLifetime 未生效 | 方案 D | 防止 WAL 膨胀 |
| **P1** | 流式 goroutine 残留 | 方案 C | 减少僵尸资源 |
| **P1** | Shutdown 无回收 | 方案 E | 安全关闭 |
| **P2** | streamClient 无超时 | 方案 C | 防止连接泄漏 |

---

## 六、二次验证报告（精准比对）

### 验证 1: Web UI 请求路径是否直接被 DB 阻塞？

**结论: 否。静态文件路径不涉及任何 DB 操作。**

浏览器访问 `http://host:18000/` 的完整链路：

```
请求 "/"
  ↓ gin.Recovery()         — 无 I/O
  ↓ Logger(zap)            — 内存日志
  ↓ SecurityHeaders()      — 设置 HTTP 头
  ↓ RateLimit()            — 内存 map 检查（/ 不在豁免列表但操作极快）
  ↓ CSRF()                 — 密码学操作，无 I/O
  ↓ EndpointStore.Get()    — RWMutex 读锁 + 内存 copy（<1μs）
  ↓ NoRoute handler
  ↓ isAPIPath("/") = false
  ↓ ServeFrontend()        — go:embed 内存文件系统
  ↓ 返回 index.html
```

每一步都是纯内存操作。**没有 DB、没有磁盘 I/O、没有网络调用。**

`EndpointStore.GetEndpoints()` 验证（`endpoint_store.go:93-104`）：
```go
func (s *EndpointStore) GetEndpoints() []*models.Endpoint {
    s.mu.RLock()         // 读锁，不阻塞其他读
    defer s.mu.RUnlock()
    snapshot := make([]*models.Endpoint, len(s.endpoints))
    copy(snapshot, s.endpoints)
    return snapshot
}
```
- 使用 `sync.RWMutex` 读锁，多个请求可并发访问
- 仅复制指针切片，纳秒级操作

### 验证 2: 那 Web UI 为什么卡死？

**结论: 间接阻塞 — goroutine 堆积导致 Go 运行时调度延迟。**

虽然静态文件路径本身无阻塞，但 Go 运行时是**协作式调度**：
1. 数千个 goroutine 阻塞在 SQLite 写连接等待上
2. Go 调度器需要在更多 goroutine 间切换 → 调度延迟增加
3. OS 线程数增长（每个阻塞的 goroutine 可能占用一个 OS 线程）
4. 内存压力 → GC 频率增加 → STW（Stop The World）暂停更长
5. 最终：即使是纯内存操作的 HTTP handler，也因调度排队而响应缓慢

**关键证据**: 每个 API 代理请求会触发 **至少 1-3 个异步 goroutine**：
- `SaveRequestLog()` — proxy.go:463（每次请求）
- `UpdateLastUsed()` — auth.go:81（每次 API Key 认证）
- `IncrementHitCount()` — llm_router.go:198（每次规则匹配）
- `UpdateHitCountByHash()` — routing_decision_cache.go:61（每次 L2 缓存命中）

每个 goroutine 最多阻塞 5s（DefaultAsyncRepoTimeout），全部竞争唯一的写连接。

### 验证 3: SQLite WAL 在 Windows 上的行为

**结论: WAL 文件可能在高负载下膨胀，但不是直接根因。**

| 配置项 | 值 | 问题 |
|--------|-----|------|
| journal_mode | WAL | 正确 |
| busy_timeout | 5000ms | 合理但叠加 goroutine 堆积后加剧问题 |
| wal_autocheckpoint | 未设置（默认 1000 pages ≈ 4MB） | 中等风险 |
| ConnMaxLifetime | 配置了 5min 但**未应用** | 连接永不回收 |
| 迁移文件中 | 无任何 WAL PRAGMA | 完全依赖默认 |

**WAL checkpoint 条件**:
- 自动: WAL 达到 ~4MB 时（默认 1000 pages）
- 连接关闭时: 但 `MaxOpenConns=1` + 无 `ConnMaxLifetime` = 连接永不关闭
- 读取连接 (`mode=ro`): 不会触发 checkpoint

**Windows 风险链**:
```
goroutine 高并发写入 → WAL 文件快速增长 → NTFS 文件锁竞争更慢
→ checkpoint 延迟 → WAL 继续增长 → 恶性循环
```

但这个循环的**起点**仍是 goroutine 堆积（P0-1），WAL 膨胀是加速器而非根因。

### 验证 4: 重启为什么能恢复？

进一步印证诊断正确：
1. **进程退出** → 所有 goroutine 被强制终止，内存释放
2. **SQLite WAL checkpoint** → 进程退出时自动执行，WAL 文件被合并回主库
3. **重新启动** → goroutine 数为 0，连接池空闲，一切正常
4. 随着时间推移 → goroutine 再次堆积 → 再次卡死

---

## 七、最终定位

```
确定性: ★★★★☆ (高确信)

根因: 无限制的异步 goroutine 写入 SQLite（proxy.go:463, auth.go:81,
      llm_router.go:198, routing_decision_cache.go:61）
      在 Windows 上因 NTFS 文件锁竞争加剧，导致 goroutine 堆积速度
      超过消化速度。

加速因素:
  1. 写连接池 MaxOpenConns=1 — 所有写入串行化
  2. ConnMaxLifetime 未应用 — 连接永不回收
  3. WAL checkpoint 无显式控制 — 文件可能膨胀
  4. RateLimit IP map 无上限 — 内存缓慢泄漏
  5. streamClient Timeout=0 — 僵尸连接可能积累

表现: Go 运行时调度延迟 → 所有 HTTP 响应变慢 → Web UI 卡死
恢复: 重启清空所有堆积的 goroutine + WAL checkpoint
```
