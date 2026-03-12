package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/entity"
	"modelgate/internal/logger"
	"modelgate/internal/quota"
	"modelgate/internal/usage"
	"modelgate/internal/utils"
)

// Proxy LLM 代理
type Proxy struct {
	lb           *RoundRobinBalancer
	quotaService *quota.Service
	usageService *usage.Service
	httpClient   *http.Client
	modelStore   *entity.ModelStore
	backendStore *entity.BackendStore
	userStore    *entity.UserStore
}

func NewProxy(lb *RoundRobinBalancer, quotaService *quota.Service, usageService *usage.Service, modelStore *entity.ModelStore, backendStore *entity.BackendStore, userStore *entity.UserStore) *Proxy {
	return &Proxy{
		lb:           lb,
		quotaService: quotaService,
		usageService: usageService,
		httpClient:   &http.Client{Timeout: 300 * time.Second},
		modelStore:   modelStore,
		backendStore: backendStore,
		userStore:    userStore,
	}
}

// OpenAIRequest OpenAI 兼容的请求格式
type OpenAIRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	Stream   bool                     `json:"stream,omitempty"`
}

// OpenAIResponse OpenAI 兼容的响应格式
type OpenAIResponse struct {
	ID      string                   `json:"id"`
	Object  string                   `json:"object"`
	Created int64                    `json:"created"`
	Model   string                   `json:"model"`
	Choices []map[string]interface{} `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// StreamResponse 流式响应格式
type StreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
}

type StreamChoice struct {
	Index        int                    `json:"index"`
	Delta        map[string]interface{} `json:"delta"`
	FinishReason *string                `json:"finish_reason"`
}

func (p *Proxy) HandleChatCompletions(c *gin.Context, userID uuid.UUID, apiKeyID uuid.UUID) {
	// 读取请求体
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req OpenAIRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	modelID := req.Model
	if modelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model is required"})
		return
	}

	backendReq := &BackendRequest{
		ModelID:     modelID,
		UserID:      userID,
		APIKeyID:    apiKeyID,
		RequestBody: bodyBytes,
		IsStream:    req.Stream,
		ClientIP:    c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
	}

	p.ExecuteCoreWorkflow(
		c,
		backendReq,
		nil, // responseConverter
		nil, // streamLineConverter
		"",  // default ping message
	)
}

// BackendRequest 后端请求参数
type BackendRequest struct {
	ModelID     string
	UserID      uuid.UUID
	APIKeyID    uuid.UUID
	RequestBody []byte
	IsStream    bool
	ClientIP    string
	UserAgent   string
}

// BackendResponse 后端响应
type BackendResponse struct {
	Body       io.ReadCloser
	StatusCode int
	BackendID  string
}

// ExecuteCoreWorkflow 执行核心代理工作流（复用逻辑）
// 支持请求/响应转换，用于实现多协议支持
func (p *Proxy) ExecuteCoreWorkflow(
	c *gin.Context,
	req *BackendRequest,
	responseConverter func([]byte) ([]byte, error),
	streamLineConverter func(string, map[string]interface{}) (string, error),
	pingMessage string,
) {
	startTime := time.Now()

	// 获取用户信息
	user, err := p.userStore.GetByID(req.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user info"})
		return
	}
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	// 检查配额
	quotaResult, err := p.quotaService.CheckQuota(req.UserID, user.QuotaPolicy, req.ModelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "quota check failed"})
		return
	}

	// 当指定的模型不被允许时，降级使用默认模型重试
	if !quotaResult.Allowed && quotaResult.Reason == "model not allowed" && quotaResult.DefaultModel != "" {
		req.ModelID = quotaResult.DefaultModel
		quotaResult, err = p.quotaService.CheckQuota(req.UserID, user.QuotaPolicy, req.ModelID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "quota check failed"})
			return
		}
	}

	if !quotaResult.Allowed {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": quotaResult.Reason,
			"quota": quotaResult,
		})
		return
	}

	// CheckQuota 只是校验限额，这里需要真实累加内存中的 rate limit 计数器，否则速率限制永远为 0
	_ = p.quotaService.IncrementRate(req.UserID, quotaResult.RateLimitWindow)

	// 选择后端
	backend, ok := p.lb.Next(req.ModelID, quotaResult.DefaultModel)
	if !ok {
		p.usageService.RecordUsageDetailed(&usage.Record{
			UserID:     req.UserID,
			ModelID:    req.ModelID,
			ClientIP:   req.ClientIP,
			UserAgent:  req.UserAgent,
			StatusCode: http.StatusServiceUnavailable,
			Error:      "no backend available",
		})
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no backend available for model: " + req.ModelID})
		return
	}

	// 获取模型配置并注入参数
	modelConfig, _ := p.modelStore.GetByID(req.ModelID)
	requestBody := req.RequestBody

	if modelConfig != nil {
		if len(modelConfig.ModelParams) > 0 {
			requestBody = injectModelParams(requestBody, modelConfig.ModelParams)
		}
		if modelConfig.ContextWindow > 0 {
			requestBody = adjustMaxTokens(requestBody, modelConfig.ContextWindow)
		}
	}

	// 修改请求体以替换 model 名称
	if backend.ModelName != "" {
		requestBody = modifyRequestModel(requestBody, backend.ModelName)
	}

	// 转发请求
	url := strings.TrimSuffix(backend.URL, "/") + "/v1/chat/completions"
	proxyReq, err := http.NewRequest(c.Request.Method, url, bytes.NewReader(requestBody))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create proxy request"})
		return
	}

	// 复制请求头（排除 Accept-Encoding，避免后端返回 gzip 压缩响应）
	for key, values := range c.Request.Header {
		if strings.ToLower(key) == "accept-encoding" {
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

	// 注入自定义 header
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
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "backend request timeout"})
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backend unavailable: " + err.Error()})
		}
		return
	}
	// 注意：这里不能使用 defer resp.Body.Close()，因为如果是流式响应，
	// 需要在 handleConvertedStreamResponse 中异步或同步读取完后再关闭。
	// 对于非流式响应，我们在处理完后关闭。

	p.lb.MarkSuccess(backend.ID)

	// 计算请求的 InputTokens
	inputTokens := utils.EstimateTokens(string(req.RequestBody))

	// 透传非 200 状态码
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		
		outputTokens := utils.EstimateTokens(string(respBody))
		c.Set("input_tokens", inputTokens)
		c.Set("output_tokens", outputTokens)
		p.usageService.RecordUsageDetailed(&usage.Record{
			UserID:       req.UserID,
			ModelID:      req.ModelID,
			LatencyMs:    int(time.Since(startTime).Milliseconds()),
			ClientIP:     req.ClientIP,
			UserAgent:    req.UserAgent,
			BackendID:    backend.ID,
			StatusCode:   resp.StatusCode,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		})
		
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
		return
	}

	// 检查后端实际响应的内容类型
	contentType := resp.Header.Get("Content-Type")
	isStreamResponse := req.IsStream && (strings.Contains(contentType, "text/event-stream") || strings.Contains(contentType, "application/x-ndjson"))

	// 根据是否流式响应选择处理方式
	if isStreamResponse {
		// handleConvertedStreamResponse 负责在结束后调用 resp.Body.Close()
		p.handleConvertedStreamResponse(c, resp, req, backend.ID, startTime, streamLineConverter, pingMessage, inputTokens)
	} else {
		defer resp.Body.Close()
		p.handleConvertedNormalResponse(c, resp, req, backend.ID, startTime, responseConverter, inputTokens)
	}
}

// handleConvertedNormalResponse 处理非流式响应（带转换）
func (p *Proxy) handleConvertedNormalResponse(
	c *gin.Context,
	resp *http.Response,
	req *BackendRequest,
	backendID string,
	startTime time.Time,
	converter func([]byte) ([]byte, error),
	inputTokens int,
) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		p.usageService.RecordUsageDetailed(&usage.Record{
			UserID:      req.UserID,
			ModelID:     req.ModelID,
			ClientIP:    req.ClientIP,
			UserAgent:   req.UserAgent,
			BackendID:   backendID,
			StatusCode:  http.StatusBadGateway,
			Error:       "failed to read backend response",
			InputTokens: inputTokens,
		})
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read backend response"})
		return
	}

	// 检查是否需要解压 gzip 响应
	decompressed := false
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(bytes.NewReader(respBody))
		if err != nil {
			logger.Warnw("failed to create gzip reader", "error", err)
		} else {
			defer gzipReader.Close()
			decompressedBody, err := io.ReadAll(gzipReader)
			if err != nil {
				logger.Warnw("failed to decompress gzip response", "error", err)
			} else {
				respBody = decompressedBody
				decompressed = true
			}
		}
	}

	// 计算 OutputTokens
	outputTokens := 0
	var normalResp OpenAIResponse
	if err := json.Unmarshal(respBody, &normalResp); err == nil && normalResp.Usage.CompletionTokens > 0 {
		outputTokens = normalResp.Usage.CompletionTokens
		// 覆盖 inputTokens 以便使用大模型提供的更精准值（如果有的话）
		if normalResp.Usage.PromptTokens > 0 {
			inputTokens = normalResp.Usage.PromptTokens
		}
	} else {
		outputTokens = utils.EstimateTokens(string(respBody))
	}

	latency := int(time.Since(startTime).Milliseconds())
	c.Set("input_tokens", inputTokens)
	c.Set("output_tokens", outputTokens)

	// 记录使用日志
	p.usageService.RecordUsageDetailed(&usage.Record{
		UserID:       req.UserID,
		ModelID:      req.ModelID,
		LatencyMs:    latency,
		ClientIP:     req.ClientIP,
		UserAgent:    req.UserAgent,
		BackendID:    backendID,
		StatusCode:   resp.StatusCode,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	})

	// 记录请求并扣除 Token
	_ = p.quotaService.RecordRequestTokens(req.UserID, req.ModelID, req.APIKeyID, inputTokens, outputTokens)

	// 只有在成功解压后才删除 Content-Encoding header
	if decompressed {
		resp.Header.Del("Content-Encoding")
	}

	// 设置 Content-Type（确保中间件能正确捕获）
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json" // 默认 Content-Type
	}
	c.Header("Content-Type", contentType)

	// 转换响应
	if converter != nil {
		converted, err := converter(respBody)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert response: " + err.Error()})
			return
		}
		c.Data(resp.StatusCode, contentType, converted)
		return
	}

	c.Data(resp.StatusCode, contentType, respBody)
}

// handleConvertedStreamResponse 处理流式响应（带转换）
func (p *Proxy) handleConvertedStreamResponse(
	c *gin.Context,
	resp *http.Response,
	req *BackendRequest,
	backendID string,
	startTime time.Time,
	lineConverter func(string, map[string]interface{}) (string, error),
	pingMessage string,
	inputTokens int,
) {
	defer resp.Body.Close()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 告知 Nginx 不要缓存响应
	c.Status(resp.StatusCode)

	// 立即发送一个 SSE 注释并 Flush，确保客户端收到 Header，防止首字节超时
	if pingMessage != "" {
		c.Writer.WriteString(pingMessage)
	} else {
		c.Writer.WriteString(": ping\n\n")
	}
	c.Writer.Flush()

	// 使用带 timeout 的 context 处理流式响应，设置为 1 小时以支持超长生成
	ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Hour)
	defer cancel()

	// 使用 mutex 保护并发写入 c.Writer (主循环和心跳协程)
	var writeMu sync.Mutex

	// 设置心跳计时器，每 30 秒发送一个 SSE 注释，防止中间代理因闲置断开连接
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// 启动心跳协程
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				writeMu.Lock()
				// 发送 SSE 注释作为心跳
				if pingMessage != "" {
					_, _ = c.Writer.WriteString(pingMessage)
				} else {
					_, _ = c.Writer.WriteString(": keep-alive\n\n")
				}
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

	// 创建该流的状态跟踪器
	streamState := make(map[string]interface{})
	var fullCollectedText strings.Builder
	outputTokens := 0

	for {
		select {
		case <-ctx.Done():
			logger.Warn("Stream processing timeout or cancelled")
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Errorw("Failed to read stream", "error", err)
			break
		}

		// 转换每一行
		if lineConverter != nil {
			converted, err := lineConverter(line, streamState)
			writeMu.Lock()
			if err != nil {
				// 转换失败时透传原始行
				_, _ = c.Writer.WriteString(line)
				fullCollectedText.WriteString(line)
			} else {
				_, _ = c.Writer.WriteString(converted)
				// 解析转换后的行获取内容以估算 Token
				contentDelta := extractContentFromSSE(converted)
				fullCollectedText.WriteString(contentDelta)
			}
			c.Writer.Flush()
			writeMu.Unlock()
		} else {
			writeMu.Lock()
			_, _ = c.Writer.WriteString(line)
			c.Writer.Flush()
			writeMu.Unlock()
			contentDelta := extractContentFromSSE(line)
			fullCollectedText.WriteString(contentDelta)
		}
	}

	outputTokens = utils.EstimateTokens(fullCollectedText.String())
	latency := int(time.Since(startTime).Milliseconds())

	c.Set("input_tokens", inputTokens)
	c.Set("output_tokens", outputTokens)

	p.usageService.RecordUsageDetailed(&usage.Record{
		UserID:       req.UserID,
		ModelID:      req.ModelID,
		LatencyMs:    latency,
		ClientIP:     req.ClientIP,
		UserAgent:    req.UserAgent,
		BackendID:    backendID,
		StatusCode:   resp.StatusCode,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	})

	_ = p.quotaService.RecordRequestTokens(req.UserID, req.ModelID, req.APIKeyID, inputTokens, outputTokens)
}

// extractContentFromSSE 从 SSE 格式的 data 行中粗略提取文本以估算 Token
func extractContentFromSSE(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
		return ""
	}
	
	jsonStr := strings.TrimPrefix(line, "data: ")
	var streamResp StreamResponse
	if err := json.Unmarshal([]byte(jsonStr), &streamResp); err == nil {
		if len(streamResp.Choices) > 0 {
			if content, ok := streamResp.Choices[0].Delta["content"].(string); ok {
				return content
			}
		}
	}
	return ""
}

func (p *Proxy) HandleListModels(c *gin.Context) {
	models, err := p.modelStore.ListEnabled()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list models"})
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

// injectModelParams 将模型参数注入请求体
// 注意：不覆盖用户已经传入的参数
func injectModelParams(reqBody []byte, params map[string]interface{}) []byte {
	var req map[string]interface{}
	if err := json.Unmarshal(reqBody, &req); err != nil {
		// 如果解析失败，返回原始请求体
		return reqBody
	}

	// 注入参数（不覆盖用户传入的）
	for key, value := range params {
		if _, exists := req[key]; !exists {
			req[key] = value
		}
	}

	// 重新序列化
	modifiedBody, err := json.Marshal(req)
	if err != nil {
		return reqBody
	}

	return modifiedBody
}

// convertHeaderName 将 __user_agent__ 转换为 User-Agent
// 规则：去掉前后的 __，将下划线替换为连字符，每个单词首字母大写
func convertHeaderName(key string) string {
	// 去掉前后的 __
	name := strings.TrimPrefix(key, "__")
	name = strings.TrimSuffix(name, "__")

	// 如果是 "header_xxx" 格式，提取后半部分
	if strings.HasPrefix(name, "header_") {
		name = strings.TrimPrefix(name, "header_")
	}

	// 将下划线分割的单词转换为首字母大写，然后用连字符连接
	// user_agent -> User-Agent
	parts := strings.Split(name, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		}
	}
	return strings.Join(parts, "-")
}

// modifyRequestModel modifies the request body to replace the model name
func modifyRequestModel(reqBody []byte, modelName string) []byte {
	var req map[string]interface{}
	if err := json.Unmarshal(reqBody, &req); err != nil {
		// 如果解析失败，返回原始请求体
		return reqBody
	}

	// 替换 model 名称
	req["model"] = modelName

	// 重新序列化
	modifiedBody, err := json.Marshal(req)
	if err != nil {
		return reqBody
	}

	return modifiedBody
}

// adjustMaxTokens intercepts the request body, counts the tokens roughly,
// and clamps max_tokens or max_completion_tokens if they would exceed the context window.
func adjustMaxTokens(body []byte, contextWindow int) []byte {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}

	// 抽出文本以估算 Tokens
	var contentBuilder strings.Builder

	// 1. 尝试追加 System (OpenAI/Anthropic compatible map access)
	if system, ok := payload["system"]; ok {
		switch v := system.(type) {
		case string:
			contentBuilder.WriteString(v)
		case []interface{}: // Anthropic system format array
			for _, blockObj := range v {
				if blockMap, ok := blockObj.(map[string]interface{}); ok {
					if bText, _ := blockMap["text"].(string); bText != "" {
						contentBuilder.WriteString(bText)
					}
				}
			}
		}
	}

	// 2. 尝试追加 Messages
	if messages, ok := payload["messages"].([]interface{}); ok {
		for _, msgObj := range messages {
			if msgMap, ok := msgObj.(map[string]interface{}); ok {
				// OpenAI system prompts live in messages
				if role, _ := msgMap["role"].(string); role == "system" {
					if content, _ := msgMap["content"].(string); content != "" {
						contentBuilder.WriteString(content)
						continue
					}
				}

				if content, ok := msgMap["content"]; ok {
					switch v := content.(type) {
					case string:
						contentBuilder.WriteString(v)
					case []interface{}:
						// Anthropic/复杂体格式，抽出文本块
						for _, blockObj := range v {
							if blockMap, ok := blockObj.(map[string]interface{}); ok {
								if bType, _ := blockMap["type"].(string); bType == "text" {
									if bText, _ := blockMap["text"].(string); bText != "" {
										contentBuilder.WriteString(bText)
									}
								} else if bType == "tool_result" {
									if cObj, ok := blockMap["content"].(string); ok {
										contentBuilder.WriteString(cObj)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 3. 尝试追加 Tools / Functions
	if tools, ok := payload["tools"].([]interface{}); ok {
		for _, toolObj := range tools {
			// 直接将 tool 的整个 JSON 序列化加进评估，因为结构和描述都算 token
			if b, err := json.Marshal(toolObj); err == nil {
				contentBuilder.Write(b)
			}
		}
	}

	inputTokens := utils.EstimateTokens(contentBuilder.String())

	// 获取客户端请求的最大 Token 数
	var maxTokens int
	var tokenKey string
	if mt, ok := payload["max_tokens"]; ok {
		if val, ok := mt.(float64); ok {
			maxTokens = int(val)
			tokenKey = "max_tokens"
		}
	} else if mct, ok := payload["max_completion_tokens"]; ok {
		// qwen or new openai format
		if val, ok := mct.(float64); ok {
			maxTokens = int(val)
			tokenKey = "max_completion_tokens"
		}
	}

	if maxTokens > 0 && tokenKey != "" {
		if inputTokens+maxTokens > contextWindow {
			newMax := contextWindow - inputTokens
			// Minimum safeguard
			if newMax < 1000 {
				newMax = 1000
			}
			payload[tokenKey] = newMax

			if newBody, err := json.Marshal(payload); err == nil {
				return newBody
			}
		}
	}

	return body
}
