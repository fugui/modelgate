package constants

// HTTP Paths
const (
	ChatCompletionsPath = "/v1/chat/completions"
	ModelsPath          = "/v1/models"
	MessagesPath        = "/v1/messages" // Anthropic API
)

// HTTP Headers
const (
	HeaderAuthorization   = "Authorization"
	HeaderRequestID       = "X-Request-ID"
	HeaderContentType     = "Content-Type"
	HeaderUserAgent       = "User-Agent"
	HeaderXRealIP         = "X-Real-IP"
	HeaderXForwardedFor   = "X-Forwarded-For"
	HeaderXAPIKeyID       = "X-API-Key-ID"
	HeaderXUserID         = "X-User-ID"
	HeaderAcceptEncoding  = "Accept-Encoding"
	HeaderContentEncoding = "Content-Encoding"
)

// Content Types
const (
	ContentTypeJSON      = "application/json"
	ContentTypeJSONLines = "application/x-ndjson"
	ContentTypeText      = "text/plain"
)

// API prefixes
const (
	APIPrefix      = "/api/v1"
	AdminAPIPrefix = "/api/v1/admin"
	ProxyAPIPrefix = "/v1"
)

// Log Body Size Limits
const (
	MaxLogRequestBodySize  = 512 * 1024 // 最大请求体记录大小: 512KB
	MaxLogResponseBodySize = 512 * 1024 // 最大响应体记录大小: 512KB
)
