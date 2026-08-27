package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

			key := ExtractSessionKey(c, nil, uuid.Nil, "192.168.1.100")
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
	key1 := ExtractSessionKey(c, body1, uuid.Nil, "192.168.1.100")
	assert.Equal(t, "body:session:sess-body-01", key1)

	// 2. conversation_id in body
	body2 := []byte(`{"model": "gpt-4", "conversation_id": "conv-body-02"}`)
	key2 := ExtractSessionKey(c, body2, uuid.Nil, "192.168.1.100")
	assert.Equal(t, "body:conv:conv-body-02", key2)

	// 3. chat_id in body
	body3 := []byte(`{"model": "gpt-4", "chat_id": "chat-body-03"}`)
	key3 := ExtractSessionKey(c, body3, uuid.Nil, "192.168.1.100")
	assert.Equal(t, "body:chat:chat-body-03", key3)

	// 4. user in body (OpenAI standard)
	body4 := []byte(`{"model": "gpt-4", "user": "user-opencode-04"}`)
	key4 := ExtractSessionKey(c, body4, uuid.Nil, "192.168.1.100")
	assert.Equal(t, "body:user:user-opencode-04", key4)
}

func TestExtractSessionKey_FromMultiTurnMessagesFingerprint(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)

	// Turn 1 (with System Prompt)
	bodyTurn1 := []byte(`{
		"model": "gpt-4",
		"messages": [
			{"role": "system", "content": "You are OpenCode Assistant."},
			{"role": "user", "content": "Hello!"}
		]
	}`)
	keyTurn1 := ExtractSessionKey(c, bodyTurn1, uuid.Nil, "")
	assert.True(t, len(keyTurn1) > 5)
	assert.Contains(t, keyTurn1, "msg0:")

	// Turn 2 (Context grows, but messages[0] remains identical)
	bodyTurn2 := []byte(`{
		"model": "gpt-4",
		"messages": [
			{"role": "system", "content": "You are OpenCode Assistant."},
			{"role": "user", "content": "Hello!"},
			{"role": "assistant", "content": "Hi! How can I help you today?"},
			{"role": "user", "content": "Write a go function."}
		]
	}`)
	keyTurn2 := ExtractSessionKey(c, bodyTurn2, uuid.Nil, "")
	// Fingerprints must be exactly the same!
	assert.Equal(t, keyTurn1, keyTurn2)
}

func TestExtractSessionKey_FromClientIP(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)

	// Intranet IP without any session info in body or header
	body := []byte(`{"model": "gpt-4"}`)
	key := ExtractSessionKey(c, body, uuid.Nil, "10.0.12.34")
	assert.Equal(t, "ip:10.0.12.34", key)
}

func TestExtractSessionKey_FromUserID(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)

	uid := uuid.New()
	key := ExtractSessionKey(c, nil, uid, "127.0.0.1")
	assert.Equal(t, "uid:"+uid.String(), key)
}
