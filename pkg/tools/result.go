package tools

import "encoding/json"

// ToolResult 表示工具执行的结构化返回值。
// 它提供了不同类型结果的清晰语义，并支持异步操作、用户消息和错误处理。
type ToolResult struct {
	// ForLLM 是发送给 LLM（大语言模型）的内容。
	// 所有结果都必须包含此字段。
	ForLLM string `json:"for_llm"`

	// ForUser 是直接发送给用户的内容。
	// 如果为空，则不会发送用户消息。
	// 如果 Silent=true，则忽略此字段。
	ForUser string `json:"for_user,omitempty"`

	// Silent 表示是否抑制向用户发送任何消息。
	// 如果为 true，即使设置了 ForUser，也会被忽略。
	Silent bool `json:"silent"`

	// IsError 表示工具执行是否失败。
	// 如果为 true，则结果应被视为错误。
	IsError bool `json:"is_error"`

	// Async 表示工具是否以异步方式运行。
	// 如果为 true，工具将在后台完成并通过回调通知。
	Async bool `json:"async"`

	// Err 是底层错误（不会被 JSON 序列化）。
	// 用于内部错误处理和日志记录。
	Err error `json:"-"`

	// Media 包含此工具生成的媒体资源引用。
	// 如果非空，代理将这些资源作为 OutboundMediaMessage 发布。
	Media []string `json:"media,omitempty"`
}

// NewToolResult 创建一个包含 LLM 内容的基本 ToolResult。
// 用于需要简单结果且具有默认行为的场景。
//
// 示例：
//
//	result := NewToolResult("文件更新成功")
func NewToolResult(forLLM string) *ToolResult {
	return &ToolResult{
		ForLLM: forLLM,
	}
}

// SilentResult 创建一个静默的 ToolResult。
// 内容仅发送给 LLM，不会发送给用户。
//
// 适用于不应打扰用户的操作，例如：
// - 文件读写
// - 状态更新
// - 后台操作
//
// 示例：
//
//	result := SilentResult("配置文件已保存")
func SilentResult(forLLM string) *ToolResult {
	return &ToolResult{
		ForLLM:  forLLM,
		Silent:  true,
		IsError: false,
		Async:   false,
	}
}

// AsyncResult 创建一个用于异步操作的 ToolResult。
// 任务将在后台运行并稍后完成。
//
// 适用于长时间运行的操作，例如：
// - 子代理生成
// - 后台处理
// - 带有回调的外部 API 调用
//
// 示例：
//
//	result := AsyncResult("子代理已生成，将稍后报告")
func AsyncResult(forLLM string) *ToolResult {
	return &ToolResult{
		ForLLM:  forLLM,
		Silent:  false,
		IsError: false,
		Async:   true,
	}
}

// ErrorResult 创建一个表示错误的 ToolResult。
// 设置 IsError=true 并包含错误消息。
//
// 示例：
//
//	result := ErrorResult("连接数据库失败：连接被拒绝")
func ErrorResult(message string) *ToolResult {
	return &ToolResult{
		ForLLM:  message,
		Silent:  false,
		IsError: true,
		Async:   false,
	}
}

// UserResult 创建一个同时提供给 LLM 和用户的 ToolResult。
// ForLLM 和 ForUser 都设置为相同的内容。
//
// 适用于用户需要直接查看结果的场景：
// - 命令执行输出
// - 获取的网页内容
// - 查询结果
//
// 示例：
//
//	result := UserResult("找到的文件总数：42")
func UserResult(content string) *ToolResult {
	return &ToolResult{
		ForLLM:  content,
		ForUser: content,
		Silent:  false,
		IsError: false,
		Async:   false,
	}
}

// MediaResult 创建一个包含媒体资源引用的 ToolResult。
// 代理将这些引用作为 OutboundMediaMessage 发布。
//
// 示例：
//
//	result := MediaResult("图片生成成功", []string{"media://abc123"})
func MediaResult(forLLM string, mediaRefs []string) *ToolResult {
	return &ToolResult{
		ForLLM: forLLM,
		Media:  mediaRefs,
	}
}

// MarshalJSON 实现自定义 JSON 序列化逻辑。
// Err 字段通过 json:"-" 标签从 JSON 输出中排除。
func (tr *ToolResult) MarshalJSON() ([]byte, error) {
	type Alias ToolResult
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(tr),
	})
}

// WithError 设置 Err 字段并返回结果以支持链式调用。
// 这保留了日志记录的错误，同时将其排除在 JSON 之外。
//
// 示例：
//
//	result := ErrorResult("操作失败").WithError(err)
func (tr *ToolResult) WithError(err error) *ToolResult {
	tr.Err = err
	return tr
}
