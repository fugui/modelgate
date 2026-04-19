package proxy

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/middleware"
)

// ExtractFunc 负责解析原生请求体，提取模型 ID、是否为流式请求以及转换后的 OpenAI 标准请求体
type ExtractFunc func(bodyBytes []byte) (modelID string, isStream bool, openaiBody []byte, err error)

// HandleProxyRequest 是泛化代理处理器，负责处理 HTTP 层的通用逻辑
func (p *Proxy) HandleProxyRequest(c *gin.Context, proto Protocol, extract ExtractFunc) {
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
	// 重新设置 body 以便后续中间件(如TrafficLog)可能需要
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// 解析请求并提取信息
	modelID, isStream, openaiBody, err := extract(bodyBytes)
	if err != nil {
		c.Data(http.StatusBadRequest, "application/json", proto.BuildErrorResponse("invalid_request_error", err.Error()))
		return
	}

	if modelID == "" {
		c.Data(http.StatusBadRequest, "application/json", proto.BuildErrorResponse("invalid_request_error", "model is required"))
		return
	}

	// 构造 BackendRequest
	backendReq := &BackendRequest{
		ModelID:     modelID,
		UserID:      uid,
		APIKeyID:    akid,
		RequestBody: openaiBody,
		IsStream:    isStream,
		ClientIP:    c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
	}

	// 调用核心工作流
	p.ExecuteCoreWorkflow(c, backendReq, proto)
}
