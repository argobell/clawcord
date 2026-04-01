// protocoltypes 包定义了与各大 LLM 提供商（OpenAI、Anthropic）交互的通用数据类型。
// 该包作为适配层，统一不同提供商的 API 差异，使上层代码可以统一处理。
package protocoltypes

// ToolCall 表示 LLM 发起的一次工具调用请求。
// 该结构体兼容 OpenAI 和 Anthropic 两种格式：
//   - OpenAI: ID + Type + Function（嵌套的 Name 和 Arguments 字符串）
//   - Anthropic: ID + Type + Name + Arguments（顶层的 JSON 对象）
//
// Name 和 Arguments 字段使用 json:"-" 标记，用于内部转换，不参与序列化。
type ToolCall struct {
	ID        string         `json:"id"`                 // 工具调用的唯一标识符
	Type      string         `json:"type,omitempty"`     // 类型：OpenAI 为 "function"，Anthropic 为 "tool_use"
	Function  *FunctionCall  `json:"function,omitempty"` // OpenAI 格式：嵌套的函数调用信息
	Name      string         `json:"-"`                  // Anthropic 格式：函数名（内部转换用）
	Arguments map[string]any `json:"-"`                  // Anthropic 格式：参数对象（内部转换用）
}

// FunctionCall 表示具体的函数调用信息，包含函数名和参数。
// OpenAI 中 Arguments 是 JSON 字符串，Anthropic 中是结构化对象。
type FunctionCall struct {
	Name      string `json:"name"`      // 要调用的函数名称
	Arguments string `json:"arguments"` // 函数参数，JSON 格式字符串
}

// LLMResponse 封装 LLM 的完整响应结果。
// 包含生成的内容、工具调用请求、结束原因、用量统计等信息。
type LLMResponse struct {
	Content          string            `json:"content"`                     // 生成的文本内容
	ReasoningContent string            `json:"reasoning_content,omitempty"` // Anthropic: 思考过程内容
	ToolCalls        []ToolCall        `json:"tool_calls,omitempty"`        // 模型请求的工具调用列表
	FinishReason     string            `json:"finish_reason"`               // 响应结束原因（stop、tool_calls、length 等）
	Usage            *UsageInfo        `json:"usage,omitempty"`             // 令牌使用量统计
	Reasoning        string            `json:"reasoning"`                   // Anthropic: 推理内容
	ReasoningDetails []ReasoningDetail `json:"reasoning_details"`           // Anthropic: 详细推理信息
}

// ReasoningDetail 表示 Anthropic 扩展思考功能的详细推理信息。
type ReasoningDetail struct {
	Format string `json:"format"` // 格式类型
	Index  int    `json:"index"`  // 序号索引
	Type   string `json:"type"`   // 内容类型
	Text   string `json:"text"`   // 推理文本内容
}

// UsageInfo 记录 LLM 请求的令牌（token）使用量统计。
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`     // 输入提示词的令牌数
	CompletionTokens int `json:"completion_tokens"` // 生成内容的令牌数
	TotalTokens      int `json:"total_tokens"`      // 总令牌数
}

// CacheControl 用于控制 Anthropic 等平台的提示词缓存行为。
// 目前仅支持 "ephemeral" 类型，用于标记可缓存的内容块。
type CacheControl struct {
	Type string `json:"type"` // 缓存类型，当前仅 "ephemeral" 有效
}

// ContentBlock 表示结构化的内容块，支持按块设置缓存控制。
// 主要用于 Anthropic 的 system message，实现精细化的缓存策略。
type ContentBlock struct {
	Type         string        `json:"type"`                    // 内容类型，通常为 "text"
	Text         string        `json:"text"`                    // 文本内容
	CacheControl *CacheControl `json:"cache_control,omitempty"` // 该内容块的缓存控制设置
}

// Message 表示对话中的一条消息，支持文本、多模态、工具调用等多种内容。
type Message struct {
	Role             string         `json:"role"`                        // 消息角色：system、user、assistant、tool
	Content          string         `json:"content"`                     // 消息文本内容
	Media            []string       `json:"media,omitempty"`             // 多模态媒体内容（图片、音频等）的 URL 或 base64
	ReasoningContent string         `json:"reasoning_content,omitempty"` // Anthropic: 思考内容
	SystemParts      []ContentBlock `json:"system_parts,omitempty"`      // 结构化 system 消息块，支持缓存控制
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`        // assistant 消息中的工具调用请求
	ToolCallID       string         `json:"tool_call_id,omitempty"`      // tool 消息对应的工具调用 ID
}

// ToolDefinition 定义一个可用的工具（函数），用于 function calling。
// 遵循 OpenAI Functions 格式规范，同时也兼容 Anthropic 的工具定义。
type ToolDefinition struct {
	Type     string                 `json:"type"`     // 工具类型，通常为 "function"
	Function ToolFunctionDefinition `json:"function"` // 函数的详细定义
}

// ToolFunctionDefinition 定义工具函数的具体信息，包括名称、描述和参数模式。
type ToolFunctionDefinition struct {
	Name        string         `json:"name"`        // 函数名称
	Description string         `json:"description"` // 函数功能描述，帮助模型理解何时调用
	Parameters  map[string]any `json:"parameters"`  // JSON Schema 格式的参数定义
}
