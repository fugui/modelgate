package constants

// Error Codes
const (
	// Quota Errors
	ErrCodeQuotaExceeded      = "QUOTA_EXCEEDED"
	ErrCodeRateLimitExceeded  = "RATE_LIMIT_EXCEEDED"
	ErrCodeDailyQuotaExceeded = "DAILY_QUOTA_EXCEEDED"

	// Backend Errors
	ErrCodeNoBackendAvailable = "NO_BACKEND_AVAILABLE"
	ErrCodeBackendFailed      = "BACKEND_FAILED"
	ErrCodeBackendTimeout     = "BACKEND_TIMEOUT"
	ErrCodeBackendUnhealthy   = "BACKEND_UNHEALTHY"

	// Auth Errors
	ErrCodeUnauthorized   = "UNAUTHORIZED"
	ErrCodeInvalidToken   = "INVALID_TOKEN"
	ErrCodeExpiredToken   = "EXPIRED_TOKEN"
	ErrCodeForbidden      = "FORBIDDEN"
	ErrCodeInvalidAPIKey  = "INVALID_API_KEY"
	ErrCodeExpiredAPIKey  = "EXPIRED_API_KEY"
	ErrCodeDisabledAPIKey = "DISABLED_API_KEY"

	// Request Errors
	ErrCodeInvalidRequest   = "INVALID_REQUEST"
	ErrCodeInvalidModel     = "INVALID_MODEL"
	ErrCodeModelNotFound    = "MODEL_NOT_FOUND"
	ErrCodeBackendNotFound  = "BACKEND_NOT_FOUND"
	ErrCodeInvalidJSON      = "INVALID_JSON"
	ErrCodeMissingField     = "MISSING_FIELD"
	ErrCodeValidationFailed = "VALIDATION_FAILED"

	// Server Errors
	ErrCodeInternalError    = "INTERNAL_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	ErrCodeDatabaseError    = "DATABASE_ERROR"
	ErrCodeConfigError      = "CONFIG_ERROR"
)

// Error Messages
const (
	// Quota Messages
	MsgQuotaExceeded      = "quota exceeded"
	MsgRateLimitExceeded  = "rate limit exceeded"
	MsgDailyQuotaExceeded = "daily quota exceeded"

	// Backend Messages
	MsgNoBackendAvailable = "no backend available for model"
	MsgBackendFailed      = "backend request failed"
	MsgBackendTimeout     = "backend request timeout"

	// Auth Messages
	MsgUnauthorized = "unauthorized"
	MsgForbidden    = "forbidden"

	// Request Messages
	MsgModelNotFound   = "model not found"
	MsgBackendNotFound = "backend not found"
	MsgInvalidRequest  = "invalid request"
)
