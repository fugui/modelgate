package responses

type ReasoningConfig struct {
	Effort string `json:"effort,omitempty"`
}

type TextConfig struct {
	Format interface{} `json:"format,omitempty"`
}

// ResponsesRequest OpenAI Responses API 请求结构 (`POST /v1/responses`)
type ResponsesRequest struct {
	Model             string           `json:"model"`
	Instructions      string           `json:"instructions,omitempty"`
	Input             interface{}      `json:"input"` // 支持 string 或数组
	Tools             []interface{}    `json:"tools,omitempty"`
	ToolChoice        interface{}      `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool            `json:"parallel_tool_calls,omitempty"`
	MaxOutputTokens   *int             `json:"max_output_tokens,omitempty"`
	Temperature       *float64         `json:"temperature,omitempty"`
	TopP              *float64         `json:"top_p,omitempty"`
	Stream            bool             `json:"stream,omitempty"`
	Reasoning         *ReasoningConfig `json:"reasoning,omitempty"`
	Text              *TextConfig      `json:"text,omitempty"`
}

// ResponseItem Responses 响应中的 Output Item
type ResponseItem struct {
	ID      string        `json:"id"`
	Type    string        `json:"type"`              // "message", "function_call", "reasoning"
	Status  string        `json:"status,omitempty"`    // "completed"
	Role    string        `json:"role,omitempty"`      // "assistant"
	Content []ContentPart `json:"content,omitempty"`
	Name    string        `json:"name,omitempty"`      // 用于 function_call
	CallID  string        `json:"call_id,omitempty"`   // 用于 function_call
	Args    string        `json:"arguments,omitempty"` // 用于 function_call
	Summary string        `json:"summary,omitempty"`   // 用于 reasoning
}

// ContentPart 响应内容块
type ContentPart struct {
	Type        string        `json:"type"` // "output_text"
	Text        string        `json:"text"`
	Annotations []interface{} `json:"annotations"`
}

// Usage Token 使用量
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ResponsesResponse Responses API 非流式响应结构
type ResponsesResponse struct {
	ID         string         `json:"id"`
	Object     string         `json:"object"` // "response"
	CreatedAt  int64          `json:"created_at"`
	Status     string         `json:"status"` // "completed"
	Model      string         `json:"model"`
	Output     []ResponseItem `json:"output"`
	OutputText string         `json:"output_text"`
	Usage      *Usage         `json:"usage,omitempty"`
}
