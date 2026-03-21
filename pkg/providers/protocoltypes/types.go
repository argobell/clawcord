package protocoltypes

// ToolCall 表示 LLM 响应中的工具调用信息。
type ToolCall struct {
	ID        string         `json:"id"`
	Type      string         `json:"type,omitempty"`
	Function  *FunctionCall  `json:"function,omitempty"`
	Name      string         `json:"name,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// FunctionCall 表示 LLM 响应中的函数调用信息。
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// UsageInfo 包含一次 LLM 调用的 token 使用情况。
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// CacheControl 标记内容块的 LLM 端缓存控制信息。
// 目前仅支持 "ephemeral"（用于 Anthropic）。
type CacheControl struct {
	Type string `json:"type,omitempty"` // 例如 "ephemeral"
}

// ContentBlock 表示系统消息中的结构化内容块。
// 适用于支持 SystemParts 的适配器，以实现每块内容的缓存控制（如 Anthropic 的 cache_control: ephemeral）。
type ContentBlock struct {
	Type         string        `json:"type,omitempty"` // 例如 "text"
	Text         string        `json:"text,omitempty"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// Message 表示一次对话中的标准消息单元。
type Message struct {
	// Role 表示消息角色，如 system/user/assistant/tool。
	Role string `json:"role"`
	// Content 为消息文本内容。
	Content string `json:"content,omitempty"`
	// ReasoningContent 为可选的推理文本内容。
	ReasoningContent string `json:"reasoning_content,omitempty"`
	// Media 为可选的媒体资源列表（如图片 URL / data URI）。
	Media []string `json:"media,omitempty"`
	// SystemParts 为结构化 system 内容块，供支持缓存分块的适配器使用。
	SystemParts []ContentBlock `json:"system_parts,omitempty"`
	// ToolCalls 为 assistant 发起的工具调用列表。
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID 用于 tool 结果消息关联对应的工具调用。
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ToolDefinition 定义可供模型调用的工具。
type ToolDefinition struct {
	Type     string                 `json:"type,omitempty"`
	Function ToolFunctionDefinition `json:"function"`
}

// ToolFunctionDefinition 定义函数型工具的名称、描述与参数模式。
type ToolFunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// LLMResponse 表示模型一次回复的统一结构。
type LLMResponse struct {
	// Content 为模型输出的文本内容。
	Content string `json:"content,omitempty"`
	// ReasoningContent 为模型可选的推理文本。
	ReasoningContent string `json:"reasoning_content,omitempty"`
	// ToolCalls 为模型返回的工具调用请求。
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// FinishReason 表示本轮输出结束原因。
	FinishReason string `json:"finish_reason,omitempty"`
	// Usage 为 token 使用统计。
	Usage *UsageInfo `json:"usage,omitempty"`
}
