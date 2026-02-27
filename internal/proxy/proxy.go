package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"llmgate/internal/models"
	"llmgate/internal/quota"
	"llmgate/internal/usage"
)

// Proxy LLM 代理
type Proxy struct {
	lb           *RoundRobinBalancer
	quotaService *quota.Service
	usageService *usage.Service
	httpClient   *http.Client
	modelStore   *models.ModelStore
}

func NewProxy(lb *RoundRobinBalancer, quotaService *quota.Service, usageService *usage.Service, modelStore *models.ModelStore) *Proxy {
	return &Proxy{
		lb:           lb,
		quotaService: quotaService,
		usageService: usageService,
		httpClient:   &http.Client{Timeout: 120 * time.Second},
		modelStore:   modelStore,
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

	// 检查配额
	quotaResult, err := p.quotaService.CheckQuota(userID, modelID)
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
	backend, ok := p.lb.Next(modelID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no backend available for model: " + modelID})
		return
	}

	// 转发请求
	url := backend + "/v1/chat/completions"
	proxyReq, err := http.NewRequest(c.Request.Method, url, bytes.NewReader(bodyBytes))
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

	// 发送请求
	resp, err := p.httpClient.Do(proxyReq)
	if err != nil {
		p.lb.MarkFailed(backend)
		c.JSON(http.StatusBadGateway, gin.H{"error": "backend request failed: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	p.lb.MarkSuccess(backend)

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
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

	// 如果没有 usage 信息，估算 token 数
	if inputTokens == 0 {
		inputTokens = estimateTokens(string(bodyBytes))
	}
	if outputTokens == 0 {
		outputTokens = len(respBody) / 4 // 粗略估计
	}

	latency := int(time.Since(startTime).Milliseconds())

	// 记录使用
	p.usageService.RecordUsage(userID, modelID, inputTokens, outputTokens, latency)

	// 扣除配额
	_ = p.quotaService.DeductQuota(userID, modelID, inputTokens, outputTokens)

	// 返回响应
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
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

// 简单估算 token 数（4 个字符约等于 1 个 token）
func estimateTokens(text string) int {
	return len(text) / 4
}
