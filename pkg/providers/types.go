package providers

import (
	"context"
	"fmt"

	"github.com/argobell/clawcord/pkg/providers/protocoltypes"
)

// Re-export protocoltypes 中的核心协议类型，供 providers 包统一引用。
type (
	ToolCall               = protocoltypes.ToolCall
	FunctionCall           = protocoltypes.FunctionCall
	UsageInfo              = protocoltypes.UsageInfo
	CacheControl           = protocoltypes.CacheControl
	ContentBlock           = protocoltypes.ContentBlock
	Message                = protocoltypes.Message
	ToolDefinition         = protocoltypes.ToolDefinition
	ToolFunctionDefinition = protocoltypes.ToolFunctionDefinition
	LLMResponse            = protocoltypes.LLMResponse
)

// LLMProvider 定义了统一的大模型提供方接口。
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
type StatefulProvider interface {
	LLMProvider
	// Close 释放 provider 持有的连接或其他资源。
	Close() error
}

// ThinkingCapable 用于标识 provider 是否支持思考能力。
type ThinkingCapable interface {
	// SupportsThinking 返回当前 provider 是否支持思考模型/思考输出。
	SupportsThinking() bool
}

// FailoverReason 表示故障转移的触发原因。
type FailoverReason string

const (
	// FailoverAuth 认证失败。
	FailoverAuth FailoverReason = "auth"
	// FailoverRateLimit 命中限流。
	FailoverRateLimit FailoverReason = "rate_limit"
	// FailoverBilling 计费或额度相关失败。
	FailoverBilling FailoverReason = "billing"
	// FailoverTimeout 请求超时。
	FailoverTimeout FailoverReason = "timeout"
	// FailoverFormat 响应格式不符合预期。
	FailoverFormat FailoverReason = "format"
	// FailoverOverloaded 服务端过载。
	FailoverOverloaded FailoverReason = "overloaded"
	// FailoverUnknown 未分类错误。
	FailoverUnknown FailoverReason = "unknown"
)

// FailoverError 封装可用于故障转移判断的错误信息。
type FailoverError struct {
	Reason   FailoverReason
	Provider string
	Model    string
	Status   int
	Wrapped  error
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

// IsRetriable 判断是否可重试；格式错误通常不可重试。
func (e *FailoverError) IsRetriable() bool {
	return e.Reason != FailoverFormat
}

// ModelConfig 定义主模型及其回退模型列表。
type ModelConfig struct {
	Primary   string
	Fallbacks []string
}
