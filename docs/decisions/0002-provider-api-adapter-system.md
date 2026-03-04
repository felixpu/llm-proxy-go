# ADR-0002: Provider API 适配器系统架构

日期：2026-03-04
状态：已实施

## 背景
LLM 代理服务需要支持多个 LLM Provider（Anthropic、OpenAI、Google 等），每个 Provider 有不同的：
1. API 端点格式和认证方式
2. 请求/响应结构
3. 流式响应格式（SSE）
4. 错误处理机制
5. 特殊功能（如 Anthropic 的 Prompt Caching）

原有实现将所有逻辑耦合在 ProxyService 中，导致：
- 代码难以维护和扩展
- 添加新 Provider 需要修改核心代理逻辑
- 测试困难（无法独立测试各 Provider 的逻辑）

## 决策
实现基于接口的 Provider API 适配器系统：

### 核心架构
```
ProxyService
    ↓
APIAdapter (interface)
    ↓
├── AnthropicAdapter
├── OpenAIAdapter
└── GoogleAdapter (future)
```

### 关键接口
```go
type APIAdapter interface {
    BuildRequest(ctx context.Context, req *models.Request) (*http.Request, error)
    ParseResponse(resp *http.Response) (*models.Response, error)
    ParseStreamEvent(line []byte) (*models.StreamEvent, error)
    ExtractMetadata(resp *http.Response) *models.Metadata
}
```

### 实现细节
1. **适配器工厂**：根据 Provider 类型创建对应的适配器
2. **共享逻辑提取**：将通用的 HTTP 调用、流式处理逻辑提取到 `llm_caller.go`
3. **Provider 特定逻辑**：每个适配器实现自己的请求构建、响应解析、元数据提取
4. **类型安全**：使用强类型的 Provider 枚举，避免字符串比较

### 集成点
- `internal/service/api_adapter.go`：适配器接口定义
- `internal/service/anthropic_adapter.go`：Anthropic 实现
- `internal/service/llm_caller.go`：共享 LLM 调用逻辑
- `internal/service/proxy.go`：使用适配器的代理服务
- `internal/models/domain.go`：Provider 类型定义

## 理由
1. **关注点分离**：每个适配器只关注自己的 Provider 逻辑
2. **易于扩展**：添加新 Provider 只需实现接口，无需修改核心代码
3. **可测试性**：可以独立测试每个适配器
4. **代码复用**：共享逻辑（HTTP 调用、流式处理）只实现一次
5. **类型安全**：编译时检查 Provider 类型，减少运行时错误

## 考虑的替代方案

### 方案 1：继续在 ProxyService 中使用 if-else
- 优点：实现简单，无需额外抽象
- 缺点：代码耦合严重，难以维护和测试

### 方案 2：使用策略模式（Strategy Pattern）
- 优点：与适配器模式类似，关注点分离
- 缺点：策略模式更适合算法替换，适配器模式更适合接口转换

### 方案 3：使用插件系统（Plugin System）
- 优点：最大灵活性，可动态加载 Provider
- 缺点：过度设计，增加复杂度，Go 的插件系统不成熟

## 后果

### 正面影响
- 代码结构清晰，易于理解和维护
- 添加新 Provider 成本低（只需实现接口）
- 测试覆盖率提升（可独立测试每个适配器）
- 减少代码重复（共享逻辑提取）
- 支持 Provider 特定功能（如 Prompt Caching）

### 负面影响
- 增加抽象层，初次理解成本略高
- 需要维护接口定义和多个实现
- 可能出现接口不够通用的情况（需要重构接口）

### 风险
- 接口设计不当可能导致频繁修改
- 过度抽象可能导致性能损失（实际影响很小）
- 需要确保所有适配器行为一致（通过集成测试保证）

## 相关 Commit
- 5be8e73 - feat: 添加 Provider API 类型支持和适配器系统
- be0e667 - refactor(service): extract shared LLM call logic and improve API adapter
- 62bc99f - test(service): update proxy tests to use new API adapter structure
- f4bbfa6 - feat: 添加 Anthropic Prompt Caching 完整支持
