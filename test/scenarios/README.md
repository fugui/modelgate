# LLMgate 场景测试

基于应用场景的测试框架，验证核心业务逻辑而非函数实现。

## 测试理念

- **场景驱动**：每个测试描述一个完整的用户场景
- **行为验证**：关注"系统应该做什么"而非"代码怎么写"
- **需求可追溯**：测试名称和内容直接对应 SPEC.md 中的需求

## 测试场景覆盖

### 1. 用户完整流程 (`user_flow_test.go`)
```
用户注册 → 登录 → 创建 API Key → 调用模型 → 配额扣减 → 查看统计
```
验证：系统核心流程是否贯通

### 2. 配额限制 (`quota_test.go`)
- ✅ 日配额耗尽后请求被拒绝
- ✅ 多模型配额共享逻辑

### 3. API Key 管理 (`apikey_test.go`)
- ✅ Key 过期后无法使用
- ✅ Key 被禁用后失效
- ✅ 非法格式 Key 被拒绝
- ⚠️ 用户被禁用后 Key 应失效（发现潜在问题）

### 4. 速率限制与权限 (`rate_limit_test.go`)
- ✅ 速率限制超限后被拒绝
- ✅ 模型访问权限控制
- ✅ 并发请求下配额计算正确
- ⚠️ 次日配额重置（需要额外机制）

### 5. 协议转换与 Claude Code (`claude_code_test.go`)
- ✅ Anthropic 到 OpenAI 的请求体正确转换
- ✅ 包含系统提示词、工具定义（Tool Schemas）
- ✅ 模型 Tool Call 准确映射到 `tool_use`，并自动追加 `toolu_` 前缀
- ✅ Gemini 流式响应（SSE）在提前 `stop` 时的正确捕获与 `stop_reason` 修正
- ✅ Claude Code 执行结果（`tool_result`）在附带 `is_error` 时的错误消息强化

## 运行测试

```bash
# 安装依赖
go get github.com/mattn/go-sqlite3
go get github.com/stretchr/testify

# 运行所有场景测试
go test ./test/scenarios/... -v

# 运行特定场景
go test ./test/scenarios/... -v -run TestScenario_UserEndToEndFlow

# 生成覆盖率报告
go test ./test/scenarios/... -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## 添加新场景

参考模板：

```go
// TestScenario_XXX
// 场景：一句话描述
// Given: 前置条件
// When: 用户行为
// Then: 期望结果
func TestScenario_XXX(t *testing.T) {
    scenario := SetupTestDB(t)
    defer scenario.Cleanup()
    scenario.InitServices()
    
    // 测试步骤...
}
```

## 发现的问题

通过编写场景测试，发现以下潜在问题：

### ✅ 已修复

1. **APIKey.ValidateKey 不检查用户状态**
   - 问题：即使 User.Enabled = false，Key 仍然有效
   - 修复：在 `ValidateKey` 中添加用户状态检查，返回 `user disabled` 错误
   - 影响文件：`internal/apikey/service.go`, `cmd/server/main.go`

### ⚠️ 待解决

2. **次日配额重置**：需要定时任务或更复杂的逻辑

## 下一步建议

1. 将场景测试集成到 CI/CD 流程
2. 补充更多边界场景（如数据库连接失败、后端 LLM 服务不可用等）
3. 添加性能场景测试（模拟 1000 并发用户）
