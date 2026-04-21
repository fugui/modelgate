package proxy

// Protocol 定义了客户端协议的响应处理接口
// 它将响应转换和精确 Token 提取的职责从 Proxy 中解耦
type Protocol interface {
	// FormatResponse 处理非流式响应
	// 接收后端的 OpenAI 格式响应，转换为客户端所需的格式
	// 返回:
	// - clientResp: 转换后的客户端格式数据
	// - preciseInputTokens: 提取的精确 Input Tokens (若无法获取或不支持则返回 0)
	// - preciseOutputTokens: 提取的精确 Output Tokens (若无法获取或不支持则返回 0)
	// - err: 转换错误
	FormatResponse(backendResp []byte) (clientResp []byte, preciseInputTokens int, preciseOutputTokens int, err error)

	// FormatStreamLine 处理流式响应的单行
	// 接收后端的 OpenAI 格式数据行，转换为客户端所需的格式
	// 返回:
	// - clientLines: 转换后的 SSE 事件文本 (可能包含多个事件，或为空)
	// - preciseInputTokens: 若流式事件中包含 Usage 信息，提取精确 Input Tokens
	// - preciseOutputTokens: 若流式事件中包含 Usage 信息，提取精确 Output Tokens
	// - contentText: 提取出的文本内容，用于在缺乏 Usage 时回退到本地估算 Token
	// - err: 转换错误
	FormatStreamLine(line string, state map[string]interface{}) (clientLines string, preciseInputTokens int, preciseOutputTokens int, contentText string, err error)

	// PingMessage 返回协议特有的 Keep-Alive 消息
	// 例如 Anthropic 返回 "event: ping\ndata: {\"type\": \"ping\"}\n\n"
	PingMessage() string

	// BuildErrorResponse 构造标准的协议错误响应
	// 接收标准的错误类型和错误详情，返回协议特定的错误 JSON 数据
	BuildErrorResponse(errType, message string) []byte
}
