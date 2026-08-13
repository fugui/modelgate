package codex

// CompletionRequest OpenAI Text Completions 请求结构 (`/v1/completions`)
type CompletionRequest struct {
	Model            string      `json:"model"`
	Prompt           interface{} `json:"prompt"` // 支持 string
	Suffix           string      `json:"suffix,omitempty"`
	MaxTokens        *int        `json:"max_tokens,omitempty"`
	Temperature      *float64    `json:"temperature,omitempty"`
	TopP             *float64    `json:"top_p,omitempty"`
	N                int         `json:"n,omitempty"`
	BestOf           *int        `json:"best_of,omitempty"`
	Stream           bool        `json:"stream,omitempty"`
	Stop             interface{} `json:"stop,omitempty"` // 支持 string 或 []string
	Echo             bool        `json:"echo,omitempty"`
	PresencePenalty  *float64    `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64    `json:"frequency_penalty,omitempty"`
	Logprobs         interface{} `json:"logprobs,omitempty"`
	User             string      `json:"user,omitempty"`
}

// CompletionChoice Text Completions 的 Choice 选项
type CompletionChoice struct {
	Text         string      `json:"text"`
	Index        int         `json:"index"`
	Logprobs     interface{} `json:"logprobs"`
	FinishReason *string     `json:"finish_reason"`
}

// Usage Token 使用量
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CompletionResponse Text Completions 的非流式响应结构
type CompletionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"` // "text_completion"
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []CompletionChoice `json:"choices"`
	Usage   *Usage             `json:"usage,omitempty"`
}

// CompletionStreamResponse Text Completions 的流式响应结构
type CompletionStreamResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"` // "text_completion"
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []CompletionChoice `json:"choices"`
	Usage   *Usage             `json:"usage,omitempty"`
}
