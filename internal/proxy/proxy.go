package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/models"
	"modelgate/internal/quota"
	"modelgate/internal/usage"
)

// Proxy LLM 代理
type Proxy struct {
	lb           *RoundRobinBalancer
	quotaService *quota.Service
	usageService *usage.Service
	httpClient   *http.Client
	modelStore   *models.ModelStore
	backendStore *models.BackendStore
	userStore    *models.UserStore
}

func NewProxy(lb *RoundRobinBalancer, quotaService *quota.Service, usageService *usage.Service, modelStore *models.ModelStore, backendStore *models.BackendStore, userStore *models.UserStore) *Proxy {
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
	startTime := time.Now()

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

	// 获取用户信息
	user, err := p.userStore.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user info"})
		return
	}
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	// 检查配额
	quotaResult, err := p.quotaService.CheckQuota(userID, user.QuotaPolicy, modelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "quota check failed"})
		return
	}

	if !quotaResult.Allowed {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": quotaResult.Reason,
			"quota": quotaResult,
		})
		return
	}

	// 获取客户端信息
	clientIP := c.ClientIP()
	userAgent := c.Request.UserAgent()

	// 选择后端
	backend, ok := p.lb.Next(modelID)
	if !ok {
		// 记录失败日志
		p.usageService.RecordUsageDetailed(&usage.Record{
			UserID:     userID,
			ModelID:    modelID,
			ClientIP:   clientIP,
			UserAgent:  userAgent,
			StatusCode: http.StatusServiceUnavailable,
			Error:      "no backend available",
		})
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no backend available for model: " + modelID})
		return
	}

	// 获取模型配置并注入参数
	modelConfig, _ := p.modelStore.GetByID(modelID)
	if modelConfig != nil && len(modelConfig.ModelParams) > 0 {
		bodyBytes = injectModelParams(bodyBytes, modelConfig.ModelParams)
	}

	// 修改请求体以替换 model 名称
	requestBody := bodyBytes
	if backend.ModelName != "" {
		requestBody = modifyRequestModel(bodyBytes, backend.ModelName)
	}

	// 转发请求 - 自动处理 base_url 末尾的斜杠
	url := strings.TrimSuffix(backend.URL, "/") + "/v1/chat/completions"
	proxyReq, err := http.NewRequest(c.Request.Method, url, bytes.NewReader(requestBody))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create proxy request"})
		return
	}

	// 复制请求头
	for key, values := range c.Request.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// 添加后端认证（如果有）
	if backend.APIKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+backend.APIKey)
	}

	// 注入自定义 header（来自 model_params，覆盖原始值）
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

	// 更新 Content-Length
	proxyReq.ContentLength = int64(len(requestBody))

	// 发送请求
	resp, err := p.httpClient.Do(proxyReq)
	if err != nil {
		p.lb.MarkFailed(backend.ID)
		// 区分错误类型返回不同状态码
		if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
			// 超时错误
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "backend request timeout"})
		} else {
			// 连接错误或其他错误
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backend unavailable: " + err.Error()})
		}
		return
	}
	defer resp.Body.Close()

	p.lb.MarkSuccess(backend.ID)

	// 如果后端返回 429，直接透传
	if resp.StatusCode == http.StatusTooManyRequests {
		// 读取响应体并透传
		respBody, _ := io.ReadAll(resp.Body)
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
		return
	}

	// 如果后端返回非 200 状态码，透传错误
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
		return
	}

	// 根据是否流式响应选择处理方式
	if req.Stream {
		p.handleStreamResponse(c, resp, userID, modelID, user.QuotaPolicy, startTime, clientIP, userAgent, backend.ID)
	} else {
		p.handleNormalResponse(c, resp, userID, modelID, user.QuotaPolicy, startTime, clientIP, userAgent, backend.ID)
	}
}

// handleNormalResponse 处理非流式响应
func (p *Proxy) handleNormalResponse(c *gin.Context, resp *http.Response, userID uuid.UUID, modelID string, quotaPolicy string, startTime time.Time, clientIP, userAgent, backendID string) {
	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		// 记录失败日志
		p.usageService.RecordUsageDetailed(&usage.Record{
			UserID:     userID,
			ModelID:    modelID,
			ClientIP:   clientIP,
			UserAgent:  userAgent,
			BackendID:  backendID,
			StatusCode: http.StatusBadGateway,
			Error:      "failed to read backend response",
		})
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read backend response"})
		return
	}

	latency := int(time.Since(startTime).Milliseconds())

	// 记录使用日志（不含 token）
	p.usageService.RecordUsageDetailed(&usage.Record{
		UserID:    userID,
		ModelID:   modelID,
		LatencyMs: latency,
		ClientIP:  clientIP,
		UserAgent: userAgent,
		BackendID: backendID,
		StatusCode: resp.StatusCode,
	})

	// 记录请求（增加请求计数）
	_ = p.quotaService.RecordRequest(userID, modelID)

	// 返回响应
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
}

// BackendRequest 后端请求参数
type BackendRequest struct {
	ModelID     string
	UserID      uuid.UUID
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
	streamLineConverter func(string) (string, error),
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

	if !quotaResult.Allowed {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": quotaResult.Reason,
			"quota": quotaResult,
		})
		return
	}

	// 选择后端
	backend, ok := p.lb.Next(req.ModelID)
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
	if modelConfig != nil && len(modelConfig.ModelParams) > 0 {
		requestBody = injectModelParams(requestBody, modelConfig.ModelParams)
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

	// 复制请求头
	for key, values := range c.Request.Header {
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
	defer resp.Body.Close()

	p.lb.MarkSuccess(backend.ID)

	// 透传非 200 状态码
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
		return
	}

	// 根据是否流式响应选择处理方式
	if req.IsStream {
		p.handleConvertedStreamResponse(c, resp, req, backend.ID, startTime, streamLineConverter)
	} else {
		p.handleConvertedNormalResponse(c, resp, req, backend.ID, startTime, responseConverter)
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
) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		p.usageService.RecordUsageDetailed(&usage.Record{
			UserID:     req.UserID,
			ModelID:    req.ModelID,
			ClientIP:   req.ClientIP,
			UserAgent:  req.UserAgent,
			BackendID:  backendID,
			StatusCode: http.StatusBadGateway,
			Error:      "failed to read backend response",
		})
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read backend response"})
		return
	}

	latency := int(time.Since(startTime).Milliseconds())

	// 记录使用日志
	p.usageService.RecordUsageDetailed(&usage.Record{
		UserID:     req.UserID,
		ModelID:    req.ModelID,
		LatencyMs:  latency,
		ClientIP:   req.ClientIP,
		UserAgent:  req.UserAgent,
		BackendID:  backendID,
		StatusCode: resp.StatusCode,
	})

	// 记录请求
	_ = p.quotaService.RecordRequest(req.UserID, req.ModelID)

	// 转换响应
	if converter != nil {
		converted, err := converter(respBody)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert response: " + err.Error()})
			return
		}
		c.Data(resp.StatusCode, "application/json", converted)
		return
	}

	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
}

// handleConvertedStreamResponse 处理流式响应（带转换）
func (p *Proxy) handleConvertedStreamResponse(
	c *gin.Context,
	resp *http.Response,
	req *BackendRequest,
	backendID string,
	startTime time.Time,
	lineConverter func(string) (string, error),
) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(resp.StatusCode)

	reader := bufio.NewReader(resp.Body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("Error reading stream: %v\n", err)
			break
		}

		// 转换每一行
		if lineConverter != nil {
			converted, err := lineConverter(line)
			if err != nil {
				// 转换失败时透传原始行
				c.Writer.WriteString(line)
			} else {
				c.Writer.WriteString(converted)
			}
		} else {
			c.Writer.WriteString(line)
		}
		c.Writer.Flush()
	}

	latency := int(time.Since(startTime).Milliseconds())

	p.usageService.RecordUsageDetailed(&usage.Record{
		UserID:     req.UserID,
		ModelID:    req.ModelID,
		LatencyMs:  latency,
		ClientIP:   req.ClientIP,
		UserAgent:  req.UserAgent,
		BackendID:  backendID,
		StatusCode: resp.StatusCode,
	})

	_ = p.quotaService.RecordRequest(req.UserID, req.ModelID)
}

// handleStreamResponse 处理流式响应（SSE）
func (p *Proxy) handleStreamResponse(c *gin.Context, resp *http.Response, userID uuid.UUID, modelID string, quotaPolicy string, startTime time.Time, clientIP, userAgent, backendID string) {
	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(resp.StatusCode)

	// 创建 reader
	reader := bufio.NewReader(resp.Body)

	// 流式转发
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			// 记录错误但不中断
			fmt.Printf("Error reading stream: %v\n", err)
			break
		}

		// 转发给客户端
		c.Writer.WriteString(line)
		c.Writer.Flush()
	}

	latency := int(time.Since(startTime).Milliseconds())

	// 记录使用日志（不含 token）
	p.usageService.RecordUsageDetailed(&usage.Record{
		UserID:     userID,
		ModelID:    modelID,
		LatencyMs:  latency,
		ClientIP:   clientIP,
		UserAgent:  userAgent,
		BackendID:  backendID,
		StatusCode: resp.StatusCode,
	})

	// 记录请求（增加请求计数）
	_ = p.quotaService.RecordRequest(userID, modelID)
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
