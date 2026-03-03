# 健康检查改进方案

## 问题描述

当前健康检查只检测 Provider 的 BaseURL 连通性，无法检测具体模型是否可用。例如：
- Provider 不再支持 opus 模型
- 但 BaseURL 仍然返回 200 OK
- 健康检查通过，但实际请求会失败

## 改进方案：动态熔断机制

### 1. 配置项扩展

```go
// HealthCheckConfig 扩展
type HealthCheckConfig struct {
    Enabled         bool
    IntervalSeconds int
    TimeoutSeconds  int

    // 新增：熔断配置
    CircuitBreaker CircuitBreakerConfig
}

type CircuitBreakerConfig struct {
    Enabled              bool    // 是否启用熔断
    ConsecutiveFailures  int     // 连续失败次数阈值（默认 3）
    FailureRateThreshold float64 // 失败率阈值（默认 0.8）
    MinSampleSize        int     // 最小样本量（默认 5）
    CooldownSeconds      int     // 冷却期（默认 300）
    HalfOpenRequests     int     // 半开状态允许的请求数（默认 3）
}
```

### 2. 端点状态扩展

```go
type EndpointState struct {
    Name              string
    Status            models.EndpointStatus
    CurrentConnections int
    TotalRequests     int
    TotalErrors       int
    LastCheckTime     *time.Time
    LastError         string
    AvgResponseTimeMs float64

    // 新增：熔断相关
    ConsecutiveFailures int       // 连续失败次数
    CircuitOpenedAt     *time.Time // 熔断开启时间
    LastErrorType       ErrorType  // 最后一次错误类型
}

type ErrorType int

const (
    ErrorTypeUnknown ErrorType = iota
    ErrorTypeNetwork           // 网络错误（临时）
    ErrorTypeTimeout           // 超时（临时）
    ErrorTypeServerError       // 5xx（临时）
    ErrorTypeModelNotFound     // 404 模型不存在（永久）
    ErrorTypeInvalidModel      // 400 无效模型（永久）
    ErrorTypeUnauthorized      // 401/403（永久）
)
```

### 3. 核心逻辑

#### 3.1 请求失败时的处理

```go
func (hc *HealthChecker) UpdateRequestStats(
    name string,
    success bool,
    latencyMs float64,
    errorType ErrorType, // 新增参数
) {
    hc.mu.Lock()
    defer hc.mu.Unlock()

    state, ok := hc.states[name]
    if !ok {
        return
    }

    state.mu.Lock()
    defer state.mu.Unlock()

    state.TotalRequests++

    if !success {
        state.TotalErrors++
        state.ConsecutiveFailures++
        state.LastErrorType = errorType

        // 检查是否需要熔断
        if hc.shouldOpenCircuit(state, errorType) {
            now := time.Now()
            state.Status = models.EndpointUnhealthy
            state.CircuitOpenedAt = &now
            hc.logger.Warn("circuit breaker opened",
                zap.String("endpoint", name),
                zap.Int("consecutive_failures", state.ConsecutiveFailures),
                zap.String("error_type", errorType.String()),
            )
        }
    } else {
        // 成功请求重置连续失败计数
        state.ConsecutiveFailures = 0

        // 如果处于熔断状态，检查是否可以恢复
        if state.Status == models.EndpointUnhealthy && state.CircuitOpenedAt != nil {
            if hc.shouldCloseCircuit(state) {
                state.Status = models.EndpointHealthy
                state.CircuitOpenedAt = nil
                hc.logger.Info("circuit breaker closed",
                    zap.String("endpoint", name),
                )
            }
        }
    }

    state.totalResponseMs += latencyMs
    if state.TotalRequests > 0 {
        state.AvgResponseTimeMs = state.totalResponseMs / float64(state.TotalRequests)
    }
}
```

#### 3.2 熔断判断逻辑

```go
func (hc *HealthChecker) shouldOpenCircuit(state *EndpointState, errorType ErrorType) bool {
    if !hc.cfg.CircuitBreaker.Enabled {
        return false
    }

    // 永久性错误立即熔断
    if errorType == ErrorTypeModelNotFound ||
       errorType == ErrorTypeInvalidModel ||
       errorType == ErrorTypeUnauthorized {
        return true
    }

    // 连续失败次数超过阈值
    if state.ConsecutiveFailures >= hc.cfg.CircuitBreaker.ConsecutiveFailures {
        return true
    }

    // 失败率超过阈值（需要足够的样本量）
    if state.TotalRequests >= hc.cfg.CircuitBreaker.MinSampleSize {
        failureRate := float64(state.TotalErrors) / float64(state.TotalRequests)
        if failureRate >= hc.cfg.CircuitBreaker.FailureRateThreshold {
            return true
        }
    }

    return false
}
```

#### 3.3 恢复判断逻辑

```go
func (hc *HealthChecker) shouldCloseCircuit(state *EndpointState) bool {
    if state.CircuitOpenedAt == nil {
        return false
    }

    // 检查是否过了冷却期
    cooldownDuration := time.Duration(hc.cfg.CircuitBreaker.CooldownSeconds) * time.Second
    if time.Since(*state.CircuitOpenedAt) < cooldownDuration {
        return false
    }

    // 半开状态：需要连续成功一定次数
    // 这里简化处理：只要有成功请求就尝试恢复
    // 实际可以要求连续成功 N 次
    return true
}
```

#### 3.4 半开状态的请求控制

```go
func (hc *HealthChecker) IsHealthy(name string) bool {
    hc.mu.RLock()
    defer hc.mu.RUnlock()

    state, ok := hc.states[name]
    if !ok {
        return false
    }

    // 完全健康
    if state.Status == models.EndpointHealthy {
        return true
    }

    // 完全不健康
    if state.Status == models.EndpointUnhealthy {
        // 检查是否可以进入半开状态
        if state.CircuitOpenedAt != nil {
            cooldownDuration := time.Duration(hc.cfg.CircuitBreaker.CooldownSeconds) * time.Second
            if time.Since(*state.CircuitOpenedAt) >= cooldownDuration {
                // 半开状态：允许少量请求通过
                // 使用概率控制（如 10% 的请求可以尝试）
                return rand.Float64() < 0.1
            }
        }
        return false
    }

    return state.Status == models.EndpointHealthy
}
```

### 4. ProxyService 中的错误类型识别

```go
func (s *ProxyService) proxyToEndpoint(...) (*models.AnthropicResponse, *ProxyMetadata, error) {
    // ... 现有代码 ...

    resp, err := s.client.Do(upReq)
    if err != nil {
        errorType := classifyError(err)
        s.healthChecker.UpdateRequestStats(epName, false, msSince(start), errorType)
        return nil, nil, fmt.Errorf("upstream request failed: %w", err)
    }
    defer resp.Body.Close()

    latencyMs := msSince(start)
    success := resp.StatusCode < 400

    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        s.healthChecker.UpdateRequestStats(epName, false, latencyMs, ErrorTypeNetwork)
        return nil, nil, fmt.Errorf("read upstream response: %w", err)
    }

    if resp.StatusCode >= 400 {
        errorType := classifyHTTPError(resp.StatusCode, respBody)
        s.healthChecker.UpdateRequestStats(epName, false, latencyMs, errorType)
        return nil, nil, &UpstreamError{StatusCode: resp.StatusCode, Body: respBody}
    }

    s.healthChecker.UpdateRequestStats(epName, true, latencyMs, ErrorTypeUnknown)

    // ... 现有代码 ...
}

func classifyError(err error) ErrorType {
    if errors.Is(err, context.DeadlineExceeded) {
        return ErrorTypeTimeout
    }
    // 其他网络错误
    return ErrorTypeNetwork
}

func classifyHTTPError(statusCode int, body []byte) ErrorType {
    switch statusCode {
    case 401, 403:
        return ErrorTypeUnauthorized
    case 404:
        // 检查响应体是否包含 "model not found" 等关键词
        if strings.Contains(string(body), "model") &&
           strings.Contains(string(body), "not found") {
            return ErrorTypeModelNotFound
        }
        return ErrorTypeUnknown
    case 400:
        // 检查是否是无效模型错误
        if strings.Contains(string(body), "invalid model") ||
           strings.Contains(string(body), "model") {
            return ErrorTypeInvalidModel
        }
        return ErrorTypeUnknown
    case 500, 502, 503, 504:
        return ErrorTypeServerError
    default:
        return ErrorTypeUnknown
    }
}
```

### 5. 配置示例

```env
# 熔断配置
LLM_PROXY_CIRCUIT_BREAKER_ENABLED=true
LLM_PROXY_CIRCUIT_BREAKER_CONSECUTIVE_FAILURES=3
LLM_PROXY_CIRCUIT_BREAKER_FAILURE_RATE_THRESHOLD=0.8
LLM_PROXY_CIRCUIT_BREAKER_MIN_SAMPLE_SIZE=5
LLM_PROXY_CIRCUIT_BREAKER_COOLDOWN_SECONDS=300
LLM_PROXY_CIRCUIT_BREAKER_HALF_OPEN_REQUESTS=3
```

## 优势

1. **零额外 Token 消耗**：完全基于实际请求的结果
2. **智能识别**：区分永久性错误和临时性错误
3. **自动恢复**：冷却期后自动尝试恢复
4. **可配置**：所有阈值都可以通过配置调整
5. **向后兼容**：默认关闭，不影响现有行为

## 实施步骤

1. 扩展配置结构和环境变量解析
2. 修改 `EndpointState` 结构
3. 实现错误类型分类函数
4. 修改 `UpdateRequestStats` 方法
5. 修改 `IsHealthy` 方法支持半开状态
6. 更新 ProxyService 中的错误处理
7. 添加单元测试
8. 更新文档

## 测试场景

1. 连续失败 3 次后自动熔断
2. 失败率超过 80% 后自动熔断
3. 404 模型不存在立即熔断
4. 冷却期后自动尝试恢复
5. 恢复成功后正常工作
6. 恢复失败后重新熔断
