package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/sjson"
	"modelgate/internal/domain/quota"
	"modelgate/internal/domain/usage"
	"modelgate/internal/infra/logger"
	"modelgate/internal/repository"
)

// Proxy LLM 代理
type Proxy struct {
	lb            *RoundRobinBalancer
	quotaService  *quota.Service
	usageService  *usage.Service
	httpClient    *http.Client
	modelStore    *entity.ModelStore
	backendStore  *entity.BackendStore
	userStore     *entity.UserStore
	trafficDumper *logger.TrafficDumper
}

func NewProxy(lb *RoundRobinBalancer, quotaService *quota.Service, usageService *usage.Service, modelStore *entity.ModelStore, backendStore *entity.BackendStore, userStore *entity.UserStore) *Proxy {
	return &Proxy{
		lb:           lb,
		quotaService: quotaService,
		usageService: usageService,
		httpClient:   &http.Client{Timeout: 30 * time.Minute},
		modelStore:   modelStore,
		backendStore: backendStore,
		userStore:    userStore,
	}
}

// SetTrafficDumper 设置原始流量调试日志组件
func (p *Proxy) SetTrafficDumper(dumper *logger.TrafficDumper) {
	p.trafficDumper = dumper
}

// OpenAIRequestHeader 优化后的轻量级请求解析器，用于 OpenAI Chat Completions
// 避免对 messages 进行全量 unmarshal，仅解析需要操作的顶层字段
type OpenAIRequestHeader struct {
	Model               string
	Stream              bool
	MaxTokens           *int
	MaxCompletionTokens *int
	Messages            []ChatMessage
	Tools               []json.RawMessage
	RawFields           map[string]json.RawMessage
}

type ChatMessage struct {
	Role             string                   `json:"role"`
	Content          json.RawMessage          `json:"content"`
	Name             *string                  `json:"name,omitempty"`
	ToolCallID       *string                  `json:"tool_call_id,omitempty"`
	ToolCalls        []map[string]interface{} `json:"tool_calls,omitempty"`
	ReasoningContent *string                  `json:"reasoning_content,omitempty"`
}

func (r *OpenAIRequestHeader) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &r.RawFields); err != nil {
		return err
	}

	if modelRaw, exists := r.RawFields["model"]; exists {
		_ = json.Unmarshal(modelRaw, &r.Model)
	}
	if streamRaw, exists := r.RawFields["stream"]; exists {
		_ = json.Unmarshal(streamRaw, &r.Stream)
	}
	if maxTokensRaw, exists := r.RawFields["max_tokens"]; exists {
		_ = json.Unmarshal(maxTokensRaw, &r.MaxTokens)
	}
	if maxCompletionTokensRaw, exists := r.RawFields["max_completion_tokens"]; exists {
		_ = json.Unmarshal(maxCompletionTokensRaw, &r.MaxCompletionTokens)
	}
	if messagesRaw, exists := r.RawFields["messages"]; exists {
		_ = json.Unmarshal(messagesRaw, &r.Messages)
	}
	if toolsRaw, exists := r.RawFields["tools"]; exists {
		_ = json.Unmarshal(toolsRaw, &r.Tools)
	}
	return nil
}

func (r *OpenAIRequestHeader) MarshalJSON() ([]byte, error) {
	if r.RawFields == nil {
		r.RawFields = make(map[string]json.RawMessage)
	}

	if r.Model != "" {
		b, _ := json.Marshal(r.Model)
		r.RawFields["model"] = b
	} else {
		delete(r.RawFields, "model")
	}

	bStream, _ := json.Marshal(r.Stream)
	r.RawFields["stream"] = bStream

	if r.MaxTokens != nil {
		b, _ := json.Marshal(r.MaxTokens)
		r.RawFields["max_tokens"] = b
	} else {
		delete(r.RawFields, "max_tokens")
	}

	if r.MaxCompletionTokens != nil {
		b, _ := json.Marshal(r.MaxCompletionTokens)
		r.RawFields["max_completion_tokens"] = b
	} else {
		delete(r.RawFields, "max_completion_tokens")
	}

	if len(r.Messages) > 0 {
		b, _ := json.Marshal(r.Messages)
		r.RawFields["messages"] = b
	} else {
		delete(r.RawFields, "messages")
	}

	if len(r.Tools) > 0 {
		b, _ := json.Marshal(r.Tools)
		r.RawFields["tools"] = b
	} else {
		delete(r.RawFields, "tools")
	}

	return json.Marshal(r.RawFields)
}


func (r *OpenAIRequestHeader) InjectParams(params map[string]interface{}) {
	if len(params) == 0 {
		return
	}
	if r.RawFields == nil {
		r.RawFields = make(map[string]json.RawMessage)
	}
	for k, v := range params {
		switch k {
		case "model":
			if r.Model == "" {
				if strVal, ok := v.(string); ok {
					r.Model = strVal
				}
			}
		case "stream":
			// skip stream injection
		case "max_tokens":
			if r.MaxTokens == nil && r.MaxCompletionTokens == nil {
				if floatVal, ok := v.(float64); ok {
					intVal := int(floatVal)
					r.MaxTokens = &intVal
				}
			}
		case "max_completion_tokens":
			if r.MaxTokens == nil && r.MaxCompletionTokens == nil {
				if floatVal, ok := v.(float64); ok {
					intVal := int(floatVal)
					r.MaxCompletionTokens = &intVal
				}
			}
		default:
			if _, exists := r.RawFields[k]; !exists {
				if b, err := json.Marshal(v); err == nil {
					r.RawFields[k] = b
				}
			}
		}
	}
}

// OpenAIRequest OpenAI 兼容的请求格式（仅用于基础解析）
type OpenAIRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	Stream   bool                     `json:"stream,omitempty"`
}

// BackendRequest 后端请求参数（纯输入，由协议 Handler 构造）
type BackendRequest struct {
	ModelID     string
	UserID      uuid.UUID
	APIKeyID    uuid.UUID
	RequestBody []byte
	IsStream    bool
	ClientIP    string
	UserAgent   string
	Passthrough bool // true = 不解析请求体，直通代理模式
	SessionKey  string
}

// BackendResponse 后端响应
type BackendResponse struct {
	Body       io.ReadCloser
	StatusCode int
	BackendID  string
}

// ExecuteCoreWorkflow 执行核心代理工作流
func (p *Proxy) ExecuteCoreWorkflow(c *gin.Context, req *BackendRequest, proto Protocol) {
	pctx := p.NewProxyContext(c, req, proto, req.Passthrough)

	// 1. 认证用户并检查配额
	if !p.authenticateAndCheckQuota(pctx) {
		return
	}

	// 2. 选择后端（内部已获取并发许可）
	backend := p.selectBackend(pctx)
	if backend == nil {
		return
	}

	// 3. 确保请求完成后释放并发许可
	defer p.lb.ReleaseBackend(pctx.BackendID)

	// 4. 准备并发送请求
	resp := p.prepareAndSendRequest(pctx, backend)
	if resp == nil {
		return
	}

	// 5. 处理响应
	p.dispatchResponse(pctx, resp)
}

// authenticateAndCheckQuota 获取用户信息并检查配额
func (p *Proxy) authenticateAndCheckQuota(pctx *ProxyContext) bool {
	req := pctx.Request

	user, err := p.userStore.GetByID(req.UserID)
	if err != nil {
		pctx.SendError(http.StatusInternalServerError, "api_error", "failed to get user info")
		return false
	}
	if user == nil {
		pctx.SendError(http.StatusUnauthorized, "invalid_request_error", "user not found")
		return false
	}

	pctx.User = user

	// 检查配额
	quotaResult, err := p.quotaService.CheckQuota(req.UserID, user.QuotaPolicy, req.ModelID)
	if err != nil {
		pctx.SendError(http.StatusInternalServerError, "api_error", "quota check failed")
		return false
	}

	// 当指定的模型不被允许时，降级使用默认模型重试
	if !quotaResult.Allowed && quotaResult.Reason == "model not allowed" && quotaResult.DefaultModel != "" {
		req.ModelID = quotaResult.DefaultModel
		quotaResult, err = p.quotaService.CheckQuota(req.UserID, user.QuotaPolicy, req.ModelID)
		if err != nil {
			pctx.SendError(http.StatusInternalServerError, "api_error", "quota check failed")
			return false
		}
	}

	if !quotaResult.Allowed {
		pctx.SendError(http.StatusTooManyRequests, "rate_limit_error", quotaResult.Reason)
		return false
	}

	_ = p.quotaService.IncrementRate(req.UserID, quotaResult.RateLimitWindow)
	pctx.DefaultModel = quotaResult.DefaultModel
	return true
}

// selectBackend 通过负载均衡选择后端
func (p *Proxy) selectBackend(pctx *ProxyContext) *Backend {
	req := pctx.Request

	backend, actualModelID, ok := p.lb.Next(req.ModelID, pctx.DefaultModel, req.SessionKey)
	if !ok {
		pctx.RecordErrorUsage(http.StatusTooManyRequests, "all backends at concurrency capacity")
		pctx.SendError(http.StatusTooManyRequests, "rate_limit_error", "all backends for model "+req.ModelID+" are at concurrency capacity, please retry later")
		return nil
	}

	req.ModelID = actualModelID
	pctx.GinCtx.Set("model_id", actualModelID)
	pctx.BackendID = backend.ID

	return backend
}

// prepareAndSendRequest 准备请求体、构造 HTTP 请求并发送到后端
func (p *Proxy) prepareAndSendRequest(pctx *ProxyContext, backend *Backend) *http.Response {
	req := pctx.Request

	// 确定后端使用的模型名称
	backendModelName := req.ModelID
	if backend.ModelName != "" {
		backendModelName = backend.ModelName
	}

	var requestBody []byte
	var err error

	if pctx.Payload != nil {
		// OpenAI Chat 模式：解析后的请求体，可注入参数
		modelConfig, _ := p.modelStore.GetByID(req.ModelID)
		if modelConfig != nil && len(modelConfig.ModelParams) > 0 {
			pctx.Payload.InjectParams(modelConfig.ModelParams)
		}

		pctx.Payload.Model = backendModelName
		requestBody, err = pctx.MarshalRequestBody()
		if err != nil {
			pctx.SendError(http.StatusInternalServerError, "api_error", "failed to marshal request body")
			return nil
		}
	} else {
		// Passthrough 模式：仅替换 model 字段
		requestBody, err = replaceModelInJSON(req.RequestBody, backendModelName)
		if err != nil {
			pctx.SendError(http.StatusInternalServerError, "api_error", "failed to prepare request body")
			return nil
		}
	}

	// Dump 阶段 2（实际发送给后端的请求体）
	pctx.DumpTraffic(logger.Stage2ConvertedRequest, requestBody, false)

	// 发送 HTTP 请求到后端
	return p.sendHTTPRequest(pctx, backend, requestBody)
}

// sendHTTPRequest 发送 HTTP 请求到后端并返回响应
func (p *Proxy) sendHTTPRequest(pctx *ProxyContext, backend *Backend, requestBody []byte) *http.Response {
	c := pctx.GinCtx

	// 构造目标 URL
	baseURL := strings.TrimSuffix(backend.URL, "/")
	backendPath := pctx.Proto.BackendPath()
	var url string
	if strings.HasSuffix(baseURL, "/openai") {
		url = baseURL + strings.TrimPrefix(backendPath, "/v1")
	} else {
		url = baseURL + backendPath
	}

	proxyReq, err := http.NewRequest(c.Request.Method, url, bytes.NewReader(requestBody))
	if err != nil {
		pctx.SendError(http.StatusInternalServerError, "api_error", "failed to create proxy request")
		return nil
	}

	// 复制请求头（排除特定的请求头，避免冲突）
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		// 排除 Accept-Encoding 避免后端返回 gzip 导致无法精准统计 token
		// 排除 Content-Length 因为修改了 requestBody 长度变了
		// 排除 Host 让 http.Client 自己推导正确的后端 host
		if lowerKey == "accept-encoding" || lowerKey == "content-length" || lowerKey == "host" {
			continue
		}
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// 添加后端认证
	if backend.APIKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+backend.APIKey)
	}

	// 注入自定义 header（从模型参数中提取 __xxx__ 格式的 header）
	modelConfig, _ := p.modelStore.GetByID(pctx.Request.ModelID)
	if modelConfig != nil && len(modelConfig.ModelParams) > 0 {
		for key, value := range modelConfig.ModelParams {
			if strings.HasPrefix(key, "__") && strings.HasSuffix(key, "__") {
				headerName := convertHeaderName(key)
				if strValue, ok := value.(string); ok {
					proxyReq.Header.Set(headerName, strValue)
				}
			}
		}
	}

	proxyReq.ContentLength = int64(len(requestBody))

	// 发送请求
	resp, err := p.httpClient.Do(proxyReq)
	if err != nil {
		p.lb.MarkFailed(backend.ID)
		if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
			pctx.SendError(http.StatusGatewayTimeout, "api_error", "backend request timeout")
		} else {
			pctx.SendError(http.StatusServiceUnavailable, "api_error", "backend unavailable: "+err.Error())
		}
		return nil
	}

	p.lb.MarkSuccess(backend.ID)
	return resp
}

// dispatchResponse 根据响应状态码和内容类型分派到对应的处理器
func (p *Proxy) dispatchResponse(pctx *ProxyContext, resp *http.Response) {
	// 透传非 200 状态码
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		p.handleErrorResponse(pctx, resp)
		return
	}

	// 检查后端实际响应的内容类型
	contentType := resp.Header.Get("Content-Type")
	isStreamResponse := pctx.Request.IsStream && (strings.Contains(contentType, "text/event-stream") || strings.Contains(contentType, "application/x-ndjson"))

	if isStreamResponse {
		p.handleStreamResponse(pctx, resp)
	} else {
		defer resp.Body.Close()
		p.handleNormalResponse(pctx, resp)
	}
}

// handleErrorResponse 处理后端返回的非 200 状态码
func (p *Proxy) handleErrorResponse(pctx *ProxyContext, resp *http.Response) {
	respBody, _ := io.ReadAll(resp.Body)
	latency := pctx.Latency()

	// 错误场景记录日志，Token 记为 0（不计费）
	pctx.GinCtx.Set("input_tokens", 0)
	pctx.GinCtx.Set("output_tokens", 0)
	p.usageService.RecordUsageDetailed(pctx.buildUsageRecord(resp.StatusCode, 0, 0, latency, string(respBody), latency))

	// Dump 阶段 3 & 4
	pctx.DumpTraffic(fmt.Sprintf("3_%d_backend_response.txt", resp.StatusCode), respBody, false)
	pctx.DumpTraffic(fmt.Sprintf("4_%d_converted_response.txt", resp.StatusCode), respBody, false)

	// 直接透传后端错误响应
	pctx.GinCtx.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
}

// handleNormalResponse 处理非流式响应（直通 + Usage 提取）
func (p *Proxy) handleNormalResponse(pctx *ProxyContext, resp *http.Response) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		pctx.RecordErrorUsage(http.StatusBadGateway, "failed to read backend response")
		pctx.GinCtx.JSON(http.StatusBadGateway, gin.H{"error": "failed to read backend response"})
		return
	}

	// 检查是否需要解压 gzip 响应
	if resp.Header.Get("Content-Encoding") == "gzip" {
		if gzipReader, err := gzip.NewReader(bytes.NewReader(respBody)); err == nil {
			defer gzipReader.Close()
			if decompressedBody, err := io.ReadAll(gzipReader); err == nil {
				respBody = decompressedBody
				resp.Header.Del("Content-Encoding")
			}
		}
	}

	// Dump 阶段 3 & 4（直通模式下两者相同）
	pctx.DumpTraffic(fmt.Sprintf("3_%d_backend_response.txt", resp.StatusCode), respBody, false)
	pctx.DumpTraffic(fmt.Sprintf("4_%d_converted_response.txt", resp.StatusCode), respBody, false)

	// 通过 Protocol 从响应体中提取精确 Token（完全依赖后端返回的 usage）
	inputTokens, outputTokens := pctx.Proto.ExtractUsage(respBody)

	latency := pctx.Latency()
	pctx.RecordUsage(resp.StatusCode, inputTokens, outputTokens, latency, string(respBody), latency)

	// 设置 Content-Type 并返回响应（直通，不做任何转换）
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	pctx.GinCtx.Header("Content-Type", contentType)
	pctx.GinCtx.Data(resp.StatusCode, contentType, respBody)
}

// handleStreamResponse 处理流式响应（直通 + Usage 提取）
func (p *Proxy) handleStreamResponse(pctx *ProxyContext, resp *http.Response) {
	c := pctx.GinCtx

	defer resp.Body.Close()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(resp.StatusCode)

	pingMessage := pctx.Proto.PingMessage()
	if pingMessage == "" {
		pingMessage = ": ping\n\n"
	}

	// 发送首字节，防止客户端超时
	c.Writer.WriteString(pingMessage)
	c.Writer.Flush()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Hour)
	defer cancel()

	var writeMu sync.Mutex

	// 心跳协程
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				writeMu.Lock()
				_, _ = c.Writer.WriteString(pingMessage)
				c.Writer.Flush()
				writeMu.Unlock()
			}
		}
	}()

	// 处理 gzip 压缩的流式响应
	var reader *bufio.Reader
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			logger.Errorw("Failed to create gzip reader", "error", err)
			return
		}
		defer gzipReader.Close()
		reader = bufio.NewReader(gzipReader)
	} else {
		reader = bufio.NewReader(resp.Body)
	}

	// 流式状态：累积 Usage
	var preciseInputTokens, preciseOutputTokens int
	var firstTokenOnce sync.Once
	var ttftMs int64

	// defer 确保无论流式循环如何退出，都记录 Token
	defer func() {
		latency := pctx.Latency()
		pctx.RecordUsage(resp.StatusCode, preciseInputTokens, preciseOutputTokens, latency, "", ttftMs)
	}()

	dumpFilename3 := fmt.Sprintf("3_%d_backend_response.txt", resp.StatusCode)
	dumpFilename4 := fmt.Sprintf("4_%d_converted_response.txt", resp.StatusCode)

	for {
		select {
		case <-ctx.Done():
			logger.Warn("Stream processing timeout or cancelled")
			return
		default:
		}

		line, err := reader.ReadString('\n')
		firstTokenOnce.Do(func() {
			ttftMs = pctx.Latency()
		})

		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Errorw("Failed to read stream", "error", err)
			break
		}

		// 从 SSE 行中提取 Usage（Protocol 负责解析不同格式）
		inToks, outToks := pctx.Proto.ExtractStreamUsage(line)
		if inToks > 0 {
			preciseInputTokens = inToks
		}
		if outToks > 0 {
			preciseOutputTokens = outToks
		}

		// Dump 阶段 3 & 4（直通模式下相同）
		pctx.DumpTraffic(dumpFilename3, []byte(line), true)
		pctx.DumpTraffic(dumpFilename4, []byte(line), true)

		// 直通：原样转发给客户端
		writeMu.Lock()
		_, _ = c.Writer.WriteString(line)
		c.Writer.Flush()
		writeMu.Unlock()
	}
}

// replaceModelInJSON 在 JSON 中替换 model 字段（用于 passthrough 模式）
func replaceModelInJSON(body []byte, newModel string) ([]byte, error) {
	return sjson.SetBytes(body, "model", newModel)
}

func (p *Proxy) HandleListModels(c *gin.Context) {
	models, err := p.modelStore.ListEnabled()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "api_error",
				"message": "failed to list models",
			},
		})
		return
	}

	var data []map[string]interface{}
	for _, m := range models {
		data = append(data, map[string]interface{}{
			"id":       m.ID,
			"object":   "model",
			"created":  m.CreatedAt.Unix(),
			"owned_by": "modelgate",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   data,
	})
}

// convertHeaderName 将 __user_agent__ 转换为 User-Agent
func convertHeaderName(key string) string {
	name := strings.TrimPrefix(key, "__")
	name = strings.TrimSuffix(name, "__")
	name = strings.TrimPrefix(name, "header_")

	parts := strings.Split(name, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		}
	}
	return strings.Join(parts, "-")
}
