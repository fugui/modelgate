package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// maxBodySize 最大捕获的响应体大小 (1MB)
	maxBodySize = 1024 * 1024
)

// responseRecorder 包装 gin.ResponseWriter 以记录响应信息
type responseRecorder struct {
	gin.ResponseWriter
	written     int64
	body        *bytes.Buffer
	headers     http.Header
	captureBody bool
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	// 延迟判断是否应该捕获响应体（确保 Content-Type 已设置）
	if r.shouldCapture() && r.body != nil && r.body.Len() < maxBodySize {
		// 只捕获前 maxBodySize 字节
		remaining := maxBodySize - r.body.Len()
		if int64(len(b)) > int64(remaining) {
			r.body.Write(b[:remaining])
		} else {
			r.body.Write(b)
		}
	}
	n, err := r.ResponseWriter.Write(b)
	r.written += int64(n)
	return n, err
}

func (r *responseRecorder) WriteString(s string) (int, error) {
	// 延迟判断是否应该捕获响应体（确保 Content-Type 已设置）
	if r.shouldCapture() && r.body != nil {
		r.body.WriteString(s)
	}
	n, err := r.ResponseWriter.WriteString(s)
	r.written += int64(n)
	return n, err
}

// shouldCapture 动态判断是否应该捕获响应体
func (r *responseRecorder) shouldCapture() bool {
	// 根据实际响应的 Content-Type 动态判断
	contentType := r.ResponseWriter.Header().Get("Content-Type")
	return shouldCaptureBody(contentType)
}

func (r *responseRecorder) WriteHeader(code int) {
	// 保存响应头（在写入状态码之前，确保头信息已完整设置）
	if r.headers != nil {
		for key, values := range r.ResponseWriter.Header() {
			if len(values) > 0 {
				r.headers.Set(key, values[0])
			}
		}
	}
	r.ResponseWriter.WriteHeader(code)
}

// UsageRecorder 访问日志记录器接口
type UsageRecorder interface {
	RecordAccess(userID uuid.UUID, method, path, clientIP, userAgent string, statusCode int, requestBytes, responseBytes int64)
	RecordAccessDetailed(userID uuid.UUID, method, path, clientIP, userAgent string, statusCode int, requestBytes, responseBytes int64, requestHeaders map[string]string, requestBody string, responseHeaders map[string]string, responseBody string)
}

// AccessLogMiddleware 访问日志记录中间件
// 只记录已认证用户的请求
func AccessLogMiddleware(usageService UsageRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 读取请求体（限制大小）
		var requestBody []byte
		if c.Request.Body != nil && c.Request.ContentLength > 0 {
			var err error
			requestBody, err = io.ReadAll(io.LimitReader(c.Request.Body, 256*1024))
			if err != nil {
				// 读取失败时继续处理，只是记录空请求体
				requestBody = []byte{}
			}
			// 重新设置 body，以便后续处理程序可以读取
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// 2. 获取请求头
		requestHeaders := make(map[string]string)
		for key, values := range c.Request.Header {
			if len(values) > 0 {
				// 只记录关键头信息，过滤敏感信息
				if shouldRecordHeader(key) {
					requestHeaders[key] = values[0]
				}
			}
		}

		// 3. 创建增强的 response recorder
		recorder := &responseRecorder{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
			headers:        make(http.Header),
			captureBody:    true, // 默认启用捕获，实际是否捕获在写入时根据 Content-Type 动态判断
		}
		c.Writer = recorder

		// 获取请求信息
		method := c.Request.Method
		path := c.Request.URL.Path
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()
		requestBytes := c.Request.ContentLength

		// 4. 处理请求
		c.Next()

		// 5. 请求完成后记录访问日志（只记录已认证用户）
		var userID uuid.UUID
		var found bool

		// 尝试从标准 JWT 认证获取用户信息
		user := GetCurrentUser(c)
		if user != nil {
			userID = user.UserID
			found = true
		} else {
			// 尝试从代理端点认证获取（API Key 或 JWT）
			if uid, exists := c.Get("user_id"); exists {
				if uidUUID, ok := uid.(uuid.UUID); ok {
					userID = uidUUID
					found = true
				}
			}
		}

		if found {
			statusCode := c.Writer.Status()
			responseBytes := recorder.written

			// 获取响应头
			responseHeaders := make(map[string]string)
			for key, values := range c.Writer.Header() {
				if len(values) > 0 {
					responseHeaders[key] = values[0]
				}
			}

			// 处理响应体
			responseBody := ""
			if recorder.captureBody && recorder.body != nil {
				body := recorder.body.String()
				// 检测是否为流式响应
				if isStreamResponse(c.Writer.Header().Get("Content-Type")) {
					responseBody = parseStreamResponse([]byte(body))
				} else {
					responseBody = body
				}
			}

			// 异步记录访问日志，避免影响响应时间
		// 在启动 goroutine 前复制数据，避免竞态条件
		requestBodyCopy := make([]byte, len(requestBody))
		copy(requestBodyCopy, requestBody)
		responseBodyCopy := responseBody // string 是不可变的，无需深拷贝

		go usageService.RecordAccessDetailed(
			userID,
			method,
			path,
			clientIP,
			userAgent,
			statusCode,
			requestBytes,
			responseBytes,
			requestHeaders,
			string(requestBodyCopy),
			responseHeaders,
			responseBodyCopy,
		)
		}
	}
}

// shouldRecordHeader 判断是否应该记录该请求头
func shouldRecordHeader(key string) bool {
	// 转换为小写进行比较
	keyLower := strings.ToLower(key)

	// 只记录关键头信息，避免敏感信息泄露
	allowedHeaders := []string{
		"content-type",
		"accept",
		"user-agent",
		"x-request-id",
		"x-real-ip",
		"x-forwarded-for",
		"accept-encoding",
		"accept-language",
	}

	for _, allowed := range allowedHeaders {
		if keyLower == allowed {
			return true
		}
	}
	return false
}

// shouldCaptureBody 判断是否捕获响应体
func shouldCaptureBody(contentType string) bool {
	if contentType == "" {
		return true
	}
	contentType = strings.ToLower(contentType)
	// 只捕获 JSON 和文本类型
	return strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "text/plain") ||
		strings.Contains(contentType, "text/event-stream")
}

// isStreamResponse 判断是否为流式响应
func isStreamResponse(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

// parseStreamResponse 解析 SSE 流式响应，提取有效内容
func parseStreamResponse(body []byte) string {
	var contents []string
	var toolCalls []string

	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		// 解析 JSON
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		// 提取 delta/content
		if choices, ok := event["choices"].([]interface{}); ok && len(choices) > 0 {
			choice, ok := choices[0].(map[string]interface{})
			if !ok {
				continue
			}
			if delta, ok := choice["delta"].(map[string]interface{}); ok {
				// 提取 content
				if content, ok := delta["content"].(string); ok && content != "" {
					contents = append(contents, content)
				}
				// 提取 tool_calls
				if toolCall, ok := delta["tool_calls"]; ok {
					toolCalls = append(toolCalls, fmt.Sprintf("%v", toolCall))
				}
			}
		}
	}

	result := strings.Join(contents, "")
	if len(toolCalls) > 0 {
		result += "\n[TOOL_CALLS]: " + strings.Join(toolCalls, ", ")
	}

	return result
}
