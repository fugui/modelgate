package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestExtractTraceID(t *testing.T) {
	headers := []string{
		"X-Request-ID",
		"Request-ID",
		"X-Trace-ID",
		"X-Correlation-ID",
	}

	for _, h := range headers {
		t.Run(h, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)
			c.Request.Header.Set(h, "custom-trace-999")

			traceID, source := ExtractTraceID(c)
			assert.Equal(t, "custom-trace-999", traceID)
			assert.Equal(t, "header:"+h, source)
		})
	}

	t.Run("GeneratedWhenMissing", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)

		traceID, source := ExtractTraceID(c)
		assert.True(t, strings.HasPrefix(traceID, "req-"))
		assert.Equal(t, "generated", source)
	})
}

func TestExtractSessionInfo_FromHeaders(t *testing.T) {
	headers := []string{
		"X-Session-ID",
		"X-Conversation-ID",
		"Session-ID",
		"Conversation-ID",
		"X-Chat-ID",
		"Chat-ID",
	}

	for _, h := range headers {
		t.Run(h, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)
			c.Request.Header.Set(h, "sess-12345")

			info := ExtractSessionInfo(c, nil)
			assert.Equal(t, "hdr:sess-12345", info.Key)
			assert.Equal(t, "header:"+h, info.Source)
			assert.Equal(t, "sess-12345", info.RawValue)
		})
	}
}

func TestExtractSessionInfo_FromBodyExplicit(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)

	// 1. session_id in body
	body1 := []byte(`{"model": "gpt-4", "session_id": "sess-body-01"}`)
	info1 := ExtractSessionInfo(c, body1)
	assert.Equal(t, "body:session:sess-body-01", info1.Key)
	assert.Equal(t, "body:session_id", info1.Source)
	assert.Equal(t, "sess-body-01", info1.RawValue)

	// 2. conversation_id in body
	body2 := []byte(`{"model": "gpt-4", "conversation_id": "conv-body-02"}`)
	info2 := ExtractSessionInfo(c, body2)
	assert.Equal(t, "body:conv:conv-body-02", info2.Key)
	assert.Equal(t, "body:conversation_id", info2.Source)
	assert.Equal(t, "conv-body-02", info2.RawValue)

	// 3. chat_id in body
	body3 := []byte(`{"model": "gpt-4", "chat_id": "chat-body-03"}`)
	info3 := ExtractSessionInfo(c, body3)
	assert.Equal(t, "body:chat:chat-body-03", info3.Key)
	assert.Equal(t, "body:chat_id", info3.Source)
	assert.Equal(t, "chat-body-03", info3.RawValue)
}

func TestExtractSessionInfo_WithoutExplicitSessionReturnsEmpty(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)

	// Normal request with only model and messages (no session headers, no session_id)
	body := []byte(`{
		"model": "gpt-4",
		"messages": [
			{"role": "user", "content": "Hello!"}
		]
	}`)
	info := ExtractSessionInfo(c, body)
	assert.Equal(t, "", info.Key, "Regular requests without explicit session IDs must return empty key")
	assert.Equal(t, "none", info.Source)
	assert.Equal(t, "", info.RawValue)
}
