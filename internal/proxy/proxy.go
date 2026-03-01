package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkoukk/tiktoken-go"
	"llmgate/internal/models"
	"llmgate/internal/quota"
	"llmgate/internal/usage"
	"llmgate/internal/usage"
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
	ID      string                   `json:"id"`
	Object  string                   `json:"object"`
	Created int64                    `json:"created"`
	Model   string                   `json:"model"`
	Choices []StreamChoice           `json:"choices"`
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

	// 修改请求体以替换 model 名称
	requestBody := bodyBytes
	if backend.ModelName != "" {
		requestBody = modifyRequestModel(bodyBytes, backend.ModelName)
	}

	// 转发请求
	url := backend.URL + "/v1/chat/completions"
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

	// 根据是否流式响应选择处理方式
	if req.Stream {
		p.handleStreamResponse(c, resp, userID, modelID, user.QuotaPolicy, startTime, bodyBytes, clientIP, userAgent, backend.ID)
	} else {
		p.handleNormalResponse(c, resp, userID, modelID, user.QuotaPolicy, startTime, bodyBytes, clientIP, userAgent, backend.ID)
	}
}

// handleNormalResponse 处理非流式响应
func (p *Proxy) handleNormalResponse(c *gin.Context, resp *http.Response, userID uuid.UUID, modelID string, quotaPolicy string, startTime time.Time, reqBody []byte, clientIP, userAgent, backendID string) {
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

	// 解析 Token 使用量
	var inputTokens, outputTokens int
	var openAIResp OpenAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err == nil {
		inputTokens = openAIResp.Usage.PromptTokens
		outputTokens = openAIResp.Usage.CompletionTokens
	}

	// 如果没有 usage 信息，使用 tiktoken 计算
	if inputTokens == 0 {
		inputTokens = countTokensFromRequest(reqBody, modelID)
	}
	if outputTokens == 0 {
		outputTokens = countTokens(string(respBody), modelID)
	}

	latency := int(time.Since(startTime).Milliseconds())

	// 记录详细使用日志
	p.usageService.RecordUsageDetailed(&usage.Record{
		UserID:       userID,
		ModelID:      modelID,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		LatencyMs:    latency,
		ClientIP:     clientIP,
		UserAgent:    userAgent,
		BackendID:    backendID,
		StatusCode:   resp.StatusCode,
	})

	// 扣除配额
	_ = p.quotaService.DeductQuota(userID, quotaPolicy, modelID, inputTokens, outputTokens)

	// 返回响应
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
}

// handleStreamResponse 处理流式响应（SSE）
func (p *Proxy) handleStreamResponse(c *gin.Context, resp *http.Response, userID uuid.UUID, modelID string, quotaPolicy string, startTime time.Time, reqBody []byte, clientIP, userAgent, backendID string) {
	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(resp.StatusCode)

	// 用于统计生成的内容
	var fullContent strings.Builder
	var outputTokens int
	var inputTokens int

	// 计算输入 token
	inputTokens = countTokensFromRequest(reqBody, modelID)

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

		// 解析内容用于统计
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			data = strings.TrimSpace(data)
			
			if data == "[DONE]" {
				// 流结束
				c.Writer.WriteString(line)
				c.Writer.Flush()
				break
			}

			var streamResp StreamResponse
			if err := json.Unmarshal([]byte(data), &streamResp); err == nil {
				for _, choice := range streamResp.Choices {
					if content, ok := choice.Delta["content"].(string); ok {
						fullContent.WriteString(content)
					}
				}
			}
		}

		// 转发给客户端
		c.Writer.WriteString(line)
		c.Writer.Flush()
	}

	// 计算输出 token
	outputTokens = countTokens(fullContent.String(), modelID)

	latency := int(time.Since(startTime).Milliseconds())

	// 记录详细使用日志
	p.usageService.RecordUsageDetailed(
		&usage.Record{
			UserID:       userID,
			ModelID:      modelID,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			LatencyMs:    latency,
			ClientIP:     clientIP,
			UserAgent:    userAgent,
			BackendID:    backendID,
			StatusCode:   resp.StatusCode,
		})

	// 扣除配额
	_ = p.quotaService.DeductQuota(userID, quotaPolicy, modelID, inputTokens, outputTokens)
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
			"owned_by": "llmgate",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   data,
	})
}

// countTokens 使用 tiktoken 计算 token 数
func countTokens(text string, model string) int {
	if text == "" {
		return 0
	}

	// 尝试获取模型的 encoding
	encoding, err := tiktoken.EncodingForModel(model)
	if err != nil {
		// 如果失败，尝试用 cl100k_base（大多数模型使用）
		encoding, err = tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			// 最后回退到估算
			return len(text) / 4
		}
	}

	tokens := encoding.Encode(text, nil, nil)
	return len(tokens)
}

// countTokensFromRequest 从请求体计算输入 token
func countTokensFromRequest(reqBody []byte, model string) int {
	var req OpenAIRequest
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return len(reqBody) / 4
	}

	totalTokens := 0

	// 计算 messages 中的 token
	for _, msg := range req.Messages {
		if content, ok := msg["content"].(string); ok {
			totalTokens += countTokens(content, model)
		}
		// 每个 message 有额外的 token 开销（role, 格式等）
		totalTokens += 4
	}

	// 额外的系统开销
	totalTokens += 3

	return totalTokens
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
