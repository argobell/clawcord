package providers

import (
	"context"
	"fmt"

	"github.com/argobell/clawcord/pkg/providers/protocoltypes"
)

// 以下类型从 protocoltypes 包重导出，用于保持 providers 包的独立性。
// 这些类型定义了 LLM 交互的核心数据结构，包括消息、工具调用、用量统计等。
type (
	ToolCall               = protocoltypes.ToolCall
	FunctionCall           = protocoltypes.FunctionCall
	LLMResponse            = protocoltypes.LLMResponse
	UsageInfo              = protocoltypes.UsageInfo
	Message                = protocoltypes.Message
	ToolDefinition         = protocoltypes.ToolDefinition
	ToolFunctionDefinition = protocoltypes.ToolFunctionDefinition
	ContentBlock           = protocoltypes.ContentBlock
	CacheControl           = protocoltypes.CacheControl
)

// LLMProvider 定义了统一的大模型提供方接口。
// 所有具体的提供商（OpenAI、Anthropic 等）都需要实现此接口。
type LLMProvider interface {
	// Chat 发起一次对话请求并返回结构化响应。
	Chat(
		ctx context.Context,
		messages []Message,
		tools []ToolDefinition,
		model string,
		options map[string]any,
	) (*LLMResponse, error)
	// GetDefaultModel 返回该 provider 的默认模型名。
	GetDefaultModel() string
}

// StatefulProvider 表示带生命周期资源管理能力的 provider。
// 适用于需要显式关闭连接或释放资源的场景（如 WebSocket 连接）。
type StatefulProvider interface {
	LLMProvider
	// Close 释放 provider 持有的连接或其他资源。
	Close()
}

// StreamingProvider 表示支持流式输出的 provider 接口。
// 用于需要实时显示生成内容的场景（逐字输出）。
type StreamingProvider interface {
	// ChatStream 发起流式对话请求。
	// onChunk 回调接收累计生成的文本（不是单个增量），可用于实时更新 UI。
	// 返回的 LLMResponse 是完整响应，与工具调用处理兼容。
	ChatStream(
		ctx context.Context,
		messages []Message,
		tools []ToolDefinition,
		model string,
		options map[string]any,
		onChunk func(accumulated string),
	) (*LLMResponse, error)
}

// ThinkingCapable 表示支持扩展思考能力的 provider 接口。
// 用于 Anthropic 等支持 "thinking" 或 "extended thinking" 的模型。
// 当配置了 thinking_level 但当前 provider 不支持时，可用于向用户发出警告。
type ThinkingCapable interface {
	SupportsThinking() bool
}

// NativeSearchCapable 表示支持内置网络搜索的 provider 接口。
// 例如 OpenAI 的 web_search_preview、xAI Grok 的搜索功能。
// 当 provider 实现此接口并返回 true 时，可以隐藏客户端的 web_search 工具，
// 避免重复搜索，直接使用 provider 的原生搜索能力。
type NativeSearchCapable interface {
	SupportsNativeSearch() bool
}

// FailoverReason 表示故障转移的触发原因类型。
type FailoverReason string

const (
	// FailoverAuth 认证失败（API Key 无效或过期）。
	FailoverAuth FailoverReason = "auth"
	// FailoverRateLimit 命中限流（请求过于频繁）。
	FailoverRateLimit FailoverReason = "rate_limit"
	// FailoverBilling 计费或额度相关失败（余额不足）。
	FailoverBilling FailoverReason = "billing"
	// FailoverTimeout 请求超时。
	FailoverTimeout FailoverReason = "timeout"
	// FailoverFormat 请求格式错误（通常不可重试）。
	FailoverFormat FailoverReason = "format"
	// FailoverContextOverflow 上下文长度超出限制（不可重试）。
	FailoverContextOverflow FailoverReason = "context_overflow"
	// FailoverOverloaded 服务端过载。
	FailoverOverloaded FailoverReason = "overloaded"
	// FailoverUnknown 未分类错误。
	FailoverUnknown FailoverReason = "unknown"
)

// FailoverError 封装可用于故障转移判断的错误信息。
// 包含错误分类、提供商、模型和状态码等元数据。
type FailoverError struct {
	Reason   FailoverReason // 错误原因分类
	Provider string         // 出错的提供商
	Model    string         // 出错的模型
	Status   int            // HTTP 状态码（如果有）
	Wrapped  error          // 原始错误
}

// Error 返回包含 provider/model/status 的完整错误描述。
func (e *FailoverError) Error() string {
	return fmt.Sprintf("failover(%s): provider=%s model=%s status=%d: %v",
		e.Reason, e.Provider, e.Model, e.Status, e.Wrapped)
}

// Unwrap 暴露被包装的底层错误，便于 errors.Is/As 使用。
func (e *FailoverError) Unwrap() error {
	return e.Wrapped
}

// IsRetriable 判断是否可重试。
// 格式错误和上下文溢出通常不可重试（修改请求也没用）。
func (e *FailoverError) IsRetriable() bool {
	return e.Reason != FailoverFormat && e.Reason != FailoverContextOverflow
}
