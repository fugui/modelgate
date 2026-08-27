package proxy

// Protocol 定义了客户端协议的行为接口
// 重构后只负责：路由、Usage 提取、错误格式化，不再做响应转换
type Protocol interface {
	// BackendPath 返回该协议对应的后端 URL 路径后缀
	// 例如 "/v1/chat/completions" 或 "/v1/responses"
	BackendPath() string

	// ExtractUsage 从非流式响应体中提取精确的 Token 使用量
	// 返回 (inputTokens, outputTokens)，无法提取时返回 (0, 0)
	ExtractUsage(respBody []byte) (inputTokens int, outputTokens int)

	// ExtractStreamUsage 从流式 SSE 行中提取精确的 Token 使用量
	// 返回 (inputTokens, outputTokens)，该行不含 usage 时返回 (0, 0)
	ExtractStreamUsage(line string) (inputTokens int, outputTokens int)

	// PingMessage 返回协议特有的 Keep-Alive 消息
	PingMessage() string

	// BuildErrorResponse 构造协议特定的 JSON 错误响应
	BuildErrorResponse(errType, message string) []byte
}
