package proxy

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/domain/usage"
	entity "modelgate/internal/repository"
)

// ProxyContext 承载单次代理请求的完整工作流状态
// 在 ExecuteCoreWorkflow 入口处创建，贯穿整个请求生命周期
type ProxyContext struct {
	// --- gin 上下文 ---
	GinCtx *gin.Context

	// --- 原始请求信息（纯输入）---
	Request *BackendRequest

	// --- 协议转换器 ---
	Proto Protocol

	// --- 解析后的请求体（只解析一次，所有修改在 map 上完成）---
	Payload map[string]interface{}

	// --- 工作流中派生的状态 ---
	StartTime    time.Time
	BackendID    string
	InputTokens  int
	User         *entity.User
	TraceID      string
	DefaultModel string // 配额策略中的默认模型（用于 LB fallback）

	// --- Proxy 实例引用（调用 service 层方法）---
	proxy *Proxy
}

// SendError 发送协议感知的错误响应，并记录第四阶段 Dump
func (pctx *ProxyContext) SendError(statusCode int, errType, message string) {
	var respBody []byte
	if pctx.Proto != nil {
		respBody = pctx.Proto.BuildErrorResponse(errType, message)
		pctx.GinCtx.Data(statusCode, "application/json", respBody)
	} else {
		respBody, _ = json.Marshal(gin.H{"error": message})
		pctx.GinCtx.Data(statusCode, "application/json", respBody)
	}
	pctx.DumpTraffic(fmt.Sprintf("4_%d_converted_response.txt", statusCode), respBody, false)
}

// MarshalRequestBody 将修改后的 Payload 序列化为 []byte
func (pctx *ProxyContext) MarshalRequestBody() ([]byte, error) {
	return json.Marshal(pctx.Payload)
}

// buildUsageRecord 构建 usage.Record（集中所有字段的组装）
func (pctx *ProxyContext) buildUsageRecord(statusCode, inputTokens, outputTokens int, latencyMs int64, responsePayload string, ttftMs int64) *usage.Record {
	req := pctx.Request
	return &usage.Record{
		UserID:          req.UserID,
		UserName:        pctx.User.Name,
		UserEmail:       pctx.User.Email,
		ModelID:         req.ModelID,
		LatencyMs:       int(latencyMs),
		ClientIP:        req.ClientIP,
		UserAgent:       req.UserAgent,
		BackendID:       pctx.BackendID,
		StatusCode:      statusCode,
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		TraceID:         pctx.TraceID,
		RequestPayload:  pctx.Payload,
		ResponsePayload: responsePayload,
		TTFTMs:          ttftMs,
	}
}

// RecordUsage 记录使用量到 usageService 和 quotaService
func (pctx *ProxyContext) RecordUsage(statusCode, inputTokens, outputTokens int, latencyMs int64, responsePayload string, ttftMs int64) {
	pctx.GinCtx.Set("input_tokens", inputTokens)
	pctx.GinCtx.Set("output_tokens", outputTokens)

	pctx.proxy.usageService.RecordUsageDetailed(
		pctx.buildUsageRecord(statusCode, inputTokens, outputTokens, latencyMs, responsePayload, ttftMs),
	)

	// 记录请求并扣除 Token
	_ = pctx.proxy.quotaService.RecordRequestTokens(
		pctx.Request.UserID,
		pctx.Request.ModelID,
		pctx.Request.APIKeyID,
		inputTokens,
		outputTokens,
		latencyMs,
	)
}

// RecordErrorUsage 记录错误场景的使用量（简化版，只记日志不扣配额）
func (pctx *ProxyContext) RecordErrorUsage(statusCode int, errorMsg string) {
	pctx.proxy.usageService.RecordUsageDetailed(&usage.Record{
		UserID:         pctx.Request.UserID,
		UserName:       pctx.User.Name,
		UserEmail:      pctx.User.Email,
		ModelID:        pctx.Request.ModelID,
		ClientIP:       pctx.Request.ClientIP,
		UserAgent:      pctx.Request.UserAgent,
		BackendID:      pctx.BackendID,
		StatusCode:     statusCode,
		Error:          errorMsg,
		InputTokens:    pctx.InputTokens,
		TraceID:        pctx.TraceID,
		RequestPayload: pctx.Payload,
	})
}

// DumpTraffic 如果启用了 traffic dumper，记录原始流量
func (pctx *ProxyContext) DumpTraffic(filename string, data []byte, appendMode bool) {
	if pctx.proxy.trafficDumper != nil && pctx.proxy.trafficDumper.IsEnabled() {
		pctx.proxy.trafficDumper.Dump(pctx.TraceID, filename, data, appendMode)
	}
}

// Latency 返回从请求开始到当前的延迟（毫秒）
func (pctx *ProxyContext) Latency() int64 {
	return time.Since(pctx.StartTime).Milliseconds()
}

// NewProxyContext 创建 ProxyContext
func (p *Proxy) NewProxyContext(c *gin.Context, req *BackendRequest, proto Protocol) *ProxyContext {
	pctx := &ProxyContext{
		GinCtx:    c,
		Request:   req,
		Proto:     proto,
		StartTime: time.Now(),
		proxy:     p,
	}

	// 解析 TraceID
	pctx.TraceID = c.GetHeader("X-Request-ID")
	if pctx.TraceID == "" {
		pctx.TraceID = c.Writer.Header().Get("X-Request-ID")
	}
	if pctx.TraceID == "" {
		pctx.TraceID = "req-" + uuid.New().String()
	}

	// 解析请求体（只解析一次）
	var payload map[string]interface{}
	_ = json.Unmarshal(req.RequestBody, &payload)
	pctx.Payload = payload

	return pctx
}
