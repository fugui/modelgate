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
	"modelgate/internal/domain/quota"
	"modelgate/internal/domain/usage"
	"modelgate/internal/infra/logger"
	"modelgate/internal/infra/utils"
	"modelgate/internal/repository"
)

// ConcurrencyTracker 并发追踪接口
type ConcurrencyTracker interface {
	Acquire(userID string) bool
	Release(userID string)
}

// Proxy LLM 代理
type Proxy struct {
	lb                 *RoundRobinBalancer
	quotaService       *quota.Service
	usageService       *usage.Service
	httpClient         *http.Client
	modelStore         *entity.ModelStore
	backendStore       *entity.BackendStore
	userStore          *entity.UserStore
	concurrencyTracker ConcurrencyTracker
	trafficDumper      *logger.TrafficDumper
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

// SetConcurrencyTracker 设置并发追踪器（用于统计并发数）
func (p *Proxy) SetConcurrencyTracker(tracker ConcurrencyTracker) {
	p.concurrencyTracker = tracker
}

// SetTrafficDumper 设置原始流量调试日志组件
func (p *Proxy) SetTrafficDumper(dumper *logger.TrafficDumper) {
	p.trafficDumper = dumper
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
	Usage   *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// StreamResponse 流式响应格式
type StreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
	Usage   *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

type StreamChoice struct {
	Index        int                    `json:"index"`
	Delta        map[string]interface{} `json:"delta"`
	FinishReason *string                `json:"finish_reason"`
}

// BackendRequest 后端请求参数
type BackendRequest struct {
	ModelID        string
	UserID         uuid.UUID
	UserName       string
	UserEmail      string
	APIKeyID       uuid.UUID
	RequestBody    []byte
	IsStream       bool
	ClientIP       string
	UserAgent      string
	TraceID        string
	RequestPayload map[string]interface{}
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
	proto Protocol,
) {
	startTime := time.Now()

	req.TraceID = c.GetHeader("X-Request-ID")
	if req.TraceID == "" {
		req.TraceID = c.Writer.Header().Get("X-Request-ID")
	}
	if req.TraceID == "" {
		req.TraceID = "req-" + uuid.New().String()
	}

	var requestPayload map[string]interface{}
	_ = json.Unmarshal(req.RequestBody, &requestPayload)
	req.RequestPayload = requestPayload

	// 统一发送错误的工具函数闭包，捕获 proto 实例
	sendErr := func(statusCode int, errType, message string) {
		if proto != nil {
			c.Data(statusCode, "application/json", proto.BuildErrorResponse(errType, message))
		} else {
			c.JSON(statusCode, gin.H{"error": message})
		}
	}

	// 追踪并发数（所有协议统一追踪，不仅限 OpenAI 中间件）
	if p.concurrencyTracker != nil {
		p.concurrencyTracker.Acquire(req.UserID.String())
		defer p.concurrencyTracker.Release(req.UserID.String())
	}

	// 获取用户信息
	user, err := p.userStore.GetByID(req.UserID)
	if err != nil {
		sendErr(http.StatusInternalServerError, "api_error", "failed to get user info")
		return
	}
	if user == nil {
		sendErr(http.StatusUnauthorized, "invalid_request_error", "user not found")
		return
	}

	// 将用户信息填充到请求中，便于后续日志记录
	req.UserName = user.Name
	req.UserEmail = user.Email

	// 检查配额
	quotaResult, err := p.quotaService.CheckQuota(req.UserID, user.QuotaPolicy, req.ModelID)
	if err != nil {
		sendErr(http.StatusInternalServerError, "api_error", "quota check failed")
		return
	}

	// 当指定的模型不被允许时，降级使用默认模型重试
	if !quotaResult.Allowed && quotaResult.Reason == "model not allowed" && quotaResult.DefaultModel != "" {
		req.ModelID = quotaResult.DefaultModel
		quotaResult, err = p.quotaService.CheckQuota(req.UserID, user.QuotaPolicy, req.ModelID)
		if err != nil {
			sendErr(http.StatusInternalServerError, "api_error", "quota check failed")
			return
		}
	}

	if !quotaResult.Allowed {
		// quotaResult.Reason 可能需要客户端处理，传递为 error message
		sendErr(http.StatusTooManyRequests, "rate_limit_error", quotaResult.Reason)
		return
	}

	// CheckQuota 只是校验限额，这里需要真实累加内存中的 rate limit 计数器，否则速率限制永远为 0
	_ = p.quotaService.IncrementRate(req.UserID, quotaResult.RateLimitWindow)

	// 选择后端
	backend, actualModelID, ok := p.lb.Next(req.ModelID, quotaResult.DefaultModel)
	if !ok {
		p.usageService.RecordUsageDetailed(&usage.Record{
			UserID:         req.UserID,
			UserName:       req.UserName,
			UserEmail:      req.UserEmail,
			ModelID:        req.ModelID,
			ClientIP:       req.ClientIP,
			UserAgent:      req.UserAgent,
			StatusCode:     http.StatusServiceUnavailable,
			Error:          "no backend available",
			TraceID:        req.TraceID,
			RequestPayload: req.RequestPayload,
		})
		sendErr(http.StatusServiceUnavailable, "api_error", "no backend available for model: "+req.ModelID)
		return
	}

	// 使用实际的模型 ID（可能是 fallback 后的模型）
	req.ModelID = actualModelID

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

	// 构造目标 URL：如果 base_url 已含 /openai 路径（如 Gemini），只追加 /chat/completions
	baseURL := strings.TrimSuffix(backend.URL, "/")
	var url string
	if strings.HasSuffix(baseURL, "/openai") {
		url = baseURL + "/chat/completions"
	} else {
		url = baseURL + "/v1/chat/completions"
	}
	proxyReq, err := http.NewRequest(c.Request.Method, url, bytes.NewReader(requestBody))
	if err != nil {
		sendErr(http.StatusInternalServerError, "api_error", "failed to create proxy request")
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
			sendErr(http.StatusGatewayTimeout, "api_error", "backend request timeout")
		} else {
			sendErr(http.StatusServiceUnavailable, "api_error", "backend unavailable: "+err.Error())
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
			UserID:          req.UserID,
			UserName:        req.UserName,
			UserEmail:       req.UserEmail,
			ModelID:         req.ModelID,
			LatencyMs:       int(time.Since(startTime).Milliseconds()),
			ClientIP:        req.ClientIP,
			UserAgent:       req.UserAgent,
			BackendID:       backend.ID,
			StatusCode:      resp.StatusCode,
			InputTokens:     inputTokens,
			OutputTokens:    outputTokens,
			TraceID:         req.TraceID,
			RequestPayload:  req.RequestPayload,
			ResponsePayload: string(respBody),
			TTFTMs:          time.Since(startTime).Milliseconds(),
		})
		// 尝试从后端返回的 OpenAI 错误中提取真正的错误信息
		errType := "api_error"
		errMsg := string(respBody)
		var backendErr struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &backendErr); err == nil && backendErr.Error.Message != "" {
			errType = backendErr.Error.Type
			if errType == "" {
				errType = "api_error"
			}
			errMsg = backendErr.Error.Message
		}

		if proto != nil {
			c.Data(resp.StatusCode, "application/json", proto.BuildErrorResponse(errType, errMsg))
		} else {
			c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
		}
		return
	}

	// 检查后端实际响应的内容类型
	contentType := resp.Header.Get("Content-Type")
	isStreamResponse := req.IsStream && (strings.Contains(contentType, "text/event-stream") || strings.Contains(contentType, "application/x-ndjson"))

	// 根据是否流式响应选择处理方式
	if isStreamResponse {
		// handleConvertedStreamResponse 负责在结束后调用 resp.Body.Close()
		p.handleConvertedStreamResponse(c, resp, req, backend.ID, startTime, proto, inputTokens)
	} else {
		defer resp.Body.Close()
		p.handleConvertedNormalResponse(c, resp, req, backend.ID, startTime, proto, inputTokens)
	}
}

// handleConvertedNormalResponse 处理非流式响应（带转换）
func (p *Proxy) handleConvertedNormalResponse(
	c *gin.Context,
	resp *http.Response,
	req *BackendRequest,
	backendID string,
	startTime time.Time,
	proto Protocol,
	inputTokens int,
) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		p.usageService.RecordUsageDetailed(&usage.Record{
			UserID:      req.UserID,
			UserName:    req.UserName,
			UserEmail:   req.UserEmail,
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

	// 使用协议接口转换响应并获取精准 Token
	var preciseInput, preciseOutput int
	var convertedRespBody []byte
	if proto != nil {
		var err error
		convertedRespBody, preciseInput, preciseOutput, err = proto.FormatResponse(respBody)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert response: " + err.Error()})
			return
		}
	} else {
		convertedRespBody = respBody
	}

	if p.trafficDumper != nil && p.trafficDumper.IsEnabled() {
		p.trafficDumper.Dump(req.TraceID, logger.Stage3BackendResponse, respBody, false)
		p.trafficDumper.Dump(req.TraceID, logger.Stage4ConvertedResponse, convertedRespBody, false)
	}

	// 计算最终 Token（优先使用精确 Token）
	if preciseInput > 0 {
		inputTokens = preciseInput
	}
	outputTokens := preciseOutput
	if outputTokens == 0 {
		outputTokens = utils.EstimateTokens(string(convertedRespBody))
	}

	latency := int(time.Since(startTime).Milliseconds())
	c.Set("input_tokens", inputTokens)
	c.Set("output_tokens", outputTokens)

	// 记录使用日志
	p.usageService.RecordUsageDetailed(&usage.Record{
		UserID:          req.UserID,
		UserName:        req.UserName,
		UserEmail:       req.UserEmail,
		ModelID:         req.ModelID,
		LatencyMs:       latency,
		ClientIP:        req.ClientIP,
		UserAgent:       req.UserAgent,
		BackendID:       backendID,
		StatusCode:      resp.StatusCode,
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		TraceID:         req.TraceID,
		RequestPayload:  req.RequestPayload,
		ResponsePayload: string(convertedRespBody),
		TTFTMs:          int64(latency),
	})

	// 记录请求并扣除 Token
	_ = p.quotaService.RecordRequestTokens(req.UserID, req.ModelID, req.APIKeyID, inputTokens, outputTokens, int64(latency))

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

	// 返回响应
	c.Data(resp.StatusCode, contentType, convertedRespBody)
}

// handleConvertedStreamResponse 处理流式响应（带转换）
func (p *Proxy) handleConvertedStreamResponse(
	c *gin.Context,
	resp *http.Response,
	req *BackendRequest,
	backendID string,
	startTime time.Time,
	proto Protocol,
	inputTokens int,
) {
	defer resp.Body.Close()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 告知 Nginx 不要缓存响应
	c.Status(resp.StatusCode)

	pingMessage := ": ping\n\n"
	if proto != nil && proto.PingMessage() != "" {
		pingMessage = proto.PingMessage()
	}

	// 立即发送一个 SSE 注释并 Flush，确保客户端收到 Header，防止首字节超时
	c.Writer.WriteString(pingMessage)
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
	var preciseInputTokens, preciseOutputTokens int
	var firstTokenOnce sync.Once
	var ttftMs int64

	// 使用 defer 确保无论流式循环如何退出（正常 EOF、错误、ctx 取消），都记录 Token
	defer func() {
		// 优先使用精确的 OutputTokens，如果未获取到则进行估算
		outputTokens := preciseOutputTokens
		if outputTokens == 0 {
			outputTokens = utils.EstimateTokens(fullCollectedText.String())
		}

		// 优先使用精确的 InputTokens
		finalInputTokens := inputTokens
		if preciseInputTokens > 0 {
			finalInputTokens = preciseInputTokens
		}
		latency := int(time.Since(startTime).Milliseconds())

		c.Set("input_tokens", finalInputTokens)
		c.Set("output_tokens", outputTokens)

		p.usageService.RecordUsageDetailed(&usage.Record{
			UserID:          req.UserID,
			UserName:        req.UserName,
			UserEmail:       req.UserEmail,
			ModelID:         req.ModelID,
			LatencyMs:       latency,
			ClientIP:        req.ClientIP,
			UserAgent:       req.UserAgent,
			BackendID:       backendID,
			StatusCode:      resp.StatusCode,
			InputTokens:     finalInputTokens,
			OutputTokens:    outputTokens,
			TraceID:         req.TraceID,
			RequestPayload:  req.RequestPayload,
			ResponsePayload: fullCollectedText.String(),
			TTFTMs:          ttftMs,
		})

		_ = p.quotaService.RecordRequestTokens(req.UserID, req.ModelID, req.APIKeyID, finalInputTokens, outputTokens, int64(latency))
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Warn("Stream processing timeout or cancelled")
			return // defer 会确保记录 Token
		default:
		}

		line, err := reader.ReadString('\n')
		firstTokenOnce.Do(func() {
			ttftMs = time.Since(startTime).Milliseconds()
		})

		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Errorw("Failed to read stream", "error", err)
			break
		}

		// 转换每一行
		if proto != nil {
			converted, inToks, outToks, contentDelta, err := proto.FormatStreamLine(line, streamState)
			if inToks > 0 {
				preciseInputTokens = inToks
			}
			if outToks > 0 {
				preciseOutputTokens = outToks
			}

			// Dump Stage 3 & 4
			if p.trafficDumper != nil && p.trafficDumper.IsEnabled() {
				p.trafficDumper.Dump(req.TraceID, logger.Stage3BackendResponse, []byte(line), true)
				if err == nil {
					p.trafficDumper.Dump(req.TraceID, logger.Stage4ConvertedResponse, []byte(converted), true)
				} else {
					p.trafficDumper.Dump(req.TraceID, logger.Stage4ConvertedResponse, []byte(line), true)
				}
			}

			writeMu.Lock()
			if err != nil {
				// 转换失败时透传原始行，并尝试提取原始行的文本
				_, _ = c.Writer.WriteString(line)
				content, _, _ := ParseOpenAISSE(line)
				fullCollectedText.WriteString(content)
			} else {
				_, _ = c.Writer.WriteString(converted)
				fullCollectedText.WriteString(contentDelta)
			}
			c.Writer.Flush()
			writeMu.Unlock()
		} else {
			// Dump Stage 3 & 4 for direct proxy
			if p.trafficDumper != nil && p.trafficDumper.IsEnabled() {
				p.trafficDumper.Dump(req.TraceID, logger.Stage3BackendResponse, []byte(line), true)
				p.trafficDumper.Dump(req.TraceID, logger.Stage4ConvertedResponse, []byte(line), true)
			}

			writeMu.Lock()
			_, _ = c.Writer.WriteString(line)
			c.Writer.Flush()
			writeMu.Unlock()
			contentDelta, inToks, outToks := ParseOpenAISSE(line)
			if inToks > 0 {
				preciseInputTokens = inToks
			}
			if outToks > 0 {
				preciseOutputTokens = outToks
			}
			fullCollectedText.WriteString(contentDelta)
		}
	}
}

// ParseOpenAISSE 解析 OpenAI SSE 格式的行
// 返回:
// - contentText: 提取的文本内容（用于估算Token）
// - preciseInputTokens: 如果包含 Usage，提取精确 Input Tokens
// - preciseOutputTokens: 如果包含 Usage，提取精确 Output Tokens
func ParseOpenAISSE(line string) (string, int, int) {
	var result strings.Builder
	var preciseInput, preciseOutput int

	// 处理可能包含多个 SSE 事件的字符串
	for _, segment := range strings.Split(line, "\n") {
		segment = strings.TrimSpace(segment)

		// 兼容 "data: {...}" 和 "data:{...}" 两种格式
		var jsonStr string
		if strings.HasPrefix(segment, "data: ") {
			jsonStr = strings.TrimPrefix(segment, "data: ")
		} else if strings.HasPrefix(segment, "data:") {
			jsonStr = strings.TrimPrefix(segment, "data:")
		} else {
			continue
		}

		jsonStr = strings.TrimSpace(jsonStr)
		if jsonStr == "[DONE]" || jsonStr == "" {
			continue
		}

		// 尝试 OpenAI 格式
		var streamResp StreamResponse
		if err := json.Unmarshal([]byte(jsonStr), &streamResp); err == nil {
			// 提取 Usage
			if streamResp.Usage != nil {
				if streamResp.Usage.PromptTokens > 0 {
					preciseInput = streamResp.Usage.PromptTokens
				}
				if streamResp.Usage.CompletionTokens > 0 {
					preciseOutput = streamResp.Usage.CompletionTokens
				}
			}

			// 提取 Content
			if len(streamResp.Choices) > 0 {
				if content, ok := streamResp.Choices[0].Delta["content"].(string); ok {
					result.WriteString(content)
				}
				// 提取 reasoning_content（思考模型如 Kimi、DeepSeek R1 等使用）
				if reasoning, ok := streamResp.Choices[0].Delta["reasoning_content"].(string); ok {
					result.WriteString(reasoning)
				}
				// 提取 tool_calls arguments
				if toolCalls, ok := streamResp.Choices[0].Delta["tool_calls"].([]interface{}); ok {
					for _, tc := range toolCalls {
						if tcMap, ok := tc.(map[string]interface{}); ok {
							if fn, ok := tcMap["function"].(map[string]interface{}); ok {
								if args, ok := fn["arguments"].(string); ok {
									result.WriteString(args)
								}
							}
						}
					}
				}
			}
		}
	}
	return result.String(), preciseInput, preciseOutput
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

	inputTokens := utils.EstimateTokensFromOpenAIRequest(body)

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
