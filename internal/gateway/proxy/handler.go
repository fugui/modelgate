package proxy

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/infra/logger"
	"modelgate/internal/infra/middleware"
)

// ExtractFunc 负责解析原生请求体，提取模型 ID、是否为流式请求以及转换后的请求体
type ExtractFunc func(bodyBytes []byte) (modelID string, isStream bool, requestBody []byte, err error)

// HandleProxyRequest 是泛化代理处理器，负责处理 HTTP 层的通用逻辑
// passthrough 参数控制是否跳过请求体解析（直通模式）
func (p *Proxy) HandleProxyRequest(c *gin.Context, proto Protocol, passthrough bool, extract ExtractFunc) {
	// 获取认证信息 (由中间件设置)
	userID, exists := c.Get(middleware.ContextKeyUserID)
	if !exists {
		c.Data(http.StatusUnauthorized, "application/json", proto.BuildErrorResponse("authentication_error", "unauthorized"))
		return
	}
	uid := userID.(uuid.UUID)

	apiKeyID, hasAPIKey := c.Get(middleware.ContextKeyAPIKeyID)
	var akid uuid.UUID
	if hasAPIKey {
		if id, ok := apiKeyID.(uuid.UUID); ok {
			akid = id
		}
	} else {
		akid = uuid.Nil
	}

	// 读取原始请求体
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Data(http.StatusBadRequest, "application/json", proto.BuildErrorResponse("invalid_request_error", "failed to read request body: "+err.Error()))
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// 解析请求并提取信息
	modelID, isStream, requestBody, err := extract(bodyBytes)
	if err != nil {
		c.Data(http.StatusBadRequest, "application/json", proto.BuildErrorResponse("invalid_request_error", err.Error()))
		return
	}

	if modelID == "" {
		c.Data(http.StatusBadRequest, "application/json", proto.BuildErrorResponse("invalid_request_error", "model is required"))
		return
	}

	// 提前获取/生成 TraceID
	traceID, _ := ExtractTraceID(c)
	c.Request.Header.Set("X-Request-ID", traceID)

	// Dump 阶段 1（客户端原始请求）
	if p.trafficDumper != nil && p.trafficDumper.IsEnabled() {
		p.trafficDumper.Dump(traceID, logger.Stage1ClientRequest, bodyBytes, false)
	}

	// 提取显式会话特征 Key (仅显式指定会话时用于 KV Cache 亲和性粘性路由)
	sessionKey := ExtractSessionKey(c, bodyBytes)
	c.Set("session_key", sessionKey)

	// 构造 BackendRequest
	backendReq := &BackendRequest{
		ModelID:     modelID,
		UserID:      uid,
		APIKeyID:    akid,
		RequestBody: requestBody,
		IsStream:    isStream,
		ClientIP:    c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		Passthrough: passthrough,
		SessionKey:  sessionKey,
	}

	// 调用核心工作流
	p.ExecuteCoreWorkflow(c, backendReq, proto)

	// 请求结束后，决定是否 Flush 原始报文 Dump
	if p.trafficDumper != nil && p.trafficDumper.IsEnabled() {
		status := c.Writer.Status()
		hasError := status >= 400 && status != http.StatusTooManyRequests
		p.trafficDumper.FlushOrDiscard(traceID, hasError)
	}
}
