package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/logger"
)

// trafficRecorder 包装 gin.ResponseWriter 以记录 TTFT 和响应时间
type trafficRecorder struct {
	gin.ResponseWriter
	startTime      time.Time
	firstTokenTime time.Time
}

func (r *trafficRecorder) Write(b []byte) (int, error) {
	if r.firstTokenTime.IsZero() {
		// 判断是否为实际数据
		// OpenAI SSE 格式以 "data: {" 开头，非流式 JSON 以 "{" 开头且包含 "choices"
		s := string(b)
		if strings.Contains(s, "data: {") || (strings.HasPrefix(strings.TrimSpace(s), "{") && strings.Contains(s, "\"choices\"")) {
			r.firstTokenTime = time.Now()
		}
	}
	return r.ResponseWriter.Write(b)
}

func (r *trafficRecorder) WriteString(s string) (int, error) {
	if r.firstTokenTime.IsZero() {
		if strings.Contains(s, "data: {") || (strings.HasPrefix(strings.TrimSpace(s), "{") && strings.Contains(s, "\"choices\"")) {
			r.firstTokenTime = time.Now()
		}
	}
	return r.ResponseWriter.WriteString(s)
}

// TrafficLogMiddleware 记录 OpenAI 兼容接口的流量
func TrafficLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 只处理 completions 或 messages 接口
		path := c.Request.URL.Path
		if !strings.Contains(path, "/completions") && !strings.Contains(path, "/messages") {
			c.Next()
			return
		}

		startTime := time.Now()

		// 1. 捕获请求载荷 (Request Payload)
		var requestPayload map[string]interface{}
		if c.Request.Body != nil && c.Request.Method == http.MethodPost {
			bodyBytes, _ := io.ReadAll(c.Request.Body)
			// 重新设置 body 以便后续处理
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			_ = json.Unmarshal(bodyBytes, &requestPayload)
		}

		// 2. 包装 ResponseWriter
		recorder := &trafficRecorder{
			ResponseWriter: c.Writer,
			startTime:      startTime,
		}
		c.Writer = recorder

		c.Next()

		// 3. 计算指标 (Metrics)
		endTime := time.Now()
		e2eMs := endTime.Sub(startTime).Milliseconds()

		var ttftMs int64
		if !recorder.firstTokenTime.IsZero() {
			ttftMs = recorder.firstTokenTime.Sub(startTime).Milliseconds()
		} else {
			// 如果没有捕获到特定的 token 时间，对于成功的非流式请求，TTFT 近似等于 E2E
			if c.Writer.Status() == http.StatusOK {
				ttftMs = e2eMs
			}
		}

		// 4. 获取 UserID 和 TraceID
		userID := "unknown"
		if uid, exists := c.Get("user_id"); exists {
			if u, ok := uid.(uuid.UUID); ok {
				userID = u.String()
			} else if s, ok := uid.(string); ok {
				userID = s
			}
		} else if claims := GetCurrentUser(c); claims != nil {
			userID = claims.UserID.String()
		}

		traceID := c.GetHeader("X-Request-ID")
		if traceID == "" {
			traceID = c.Writer.Header().Get("X-Request-ID")
		}
		if traceID == "" {
			traceID = "req-" + uuid.New().String()
		}

		// 5. 获取 Token 使用情况 (由 Proxy 设置到 Context)
		var inputTokens, outputTokens int
		if in, exists := c.Get("input_tokens"); exists {
			if v, ok := in.(int); ok {
				inputTokens = v
			}
		}
		if out, exists := c.Get("output_tokens"); exists {
			if v, ok := out.(int); ok {
				outputTokens = v
			}
		}

		// 6. 构造日志条目并异步写入
		entry := logger.TrafficEntry{
			Timestamp:      startTime.UnixNano() / 1e6,
			TraceID:        traceID,
			UserID:         userID,
			RequestPayload: requestPayload,
			Metrics: logger.TrafficMetrics{
				PromptTokens:     inputTokens,
				CompletionTokens: outputTokens,
				OriginalTTFTMs:   ttftMs,
				OriginalE2EMs:    e2eMs,
			},
		}

		// 异步写入文件日志
		go func() {
			_ = logger.GetTrafficLogger().Log(entry)
		}()
	}
}
