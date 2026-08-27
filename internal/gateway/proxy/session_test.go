package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestExtractSessionKey_FromHeaders(t *testing.T) {
	headers := []string{
		"X-Session-ID",
		"X-Session-Id",
		"X-Conversation-ID",
		"X-Conversation-Id",
		"Session-ID",
		"Session-Id",
		"Conversation-ID",
		"Conversation-Id",
	}

	for _, h := range headers {
		t.Run(h, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)
			c.Request.Header.Set(h, "sess-12345")

			key := ExtractSessionKey(c, nil)
			assert.Equal(t, "hdr:sess-12345", key)
		})
	}
}

func TestExtractSessionKey_FromBodyExplicit(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)

	// 1. session_id in body
	body1 := []byte(`{"model": "gpt-4", "session_id": "sess-body-01"}`)
	key1 := ExtractSessionKey(c, body1)
	assert.Equal(t, "body:session:sess-body-01", key1)

	// 2. conversation_id in body
	body2 := []byte(`{"model": "gpt-4", "conversation_id": "conv-body-02"}`)
	key2 := ExtractSessionKey(c, body2)
	assert.Equal(t, "body:conv:conv-body-02", key2)

	// 3. chat_id in body
	body3 := []byte(`{"model": "gpt-4", "chat_id": "chat-body-03"}`)
	key3 := ExtractSessionKey(c, body3)
	assert.Equal(t, "body:chat:chat-body-03", key3)
}

func TestExtractSessionKey_WithoutExplicitSessionReturnsEmpty(t *testing.T) {
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
	key := ExtractSessionKey(c, body)
	assert.Equal(t, "", key, "Regular requests without explicit session IDs must return empty string to enable least-connections load balancing")
}
