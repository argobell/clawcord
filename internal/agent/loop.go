package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/argobell/clawcord/pkg/bus"
	"github.com/argobell/clawcord/pkg/media"
	"github.com/argobell/clawcord/pkg/providers"
	"github.com/argobell/clawcord/pkg/tools"
)

// ProcessOptions 描述一次最小 agent turn 所需的输入。
type ProcessOptions struct {
	SessionKey       string
	Channel          string
	ChatID           string
	ReplyToMessageID string
	UserMessage      string
	Media            []string
	MediaStore       media.MediaStore
	PublishMedia     func(context.Context, bus.OutboundMediaMessage) error
	DefaultResponse  string
	NoHistory        bool
}

// TurnResult 表示单轮执行结束后的最终输出。
type TurnResult struct {
	Content         string
	Iterations      int
	ResponseHandled bool
}

// RunTurn 执行一次最小 agent turn，并按顺序持久化本轮新增消息。
func (a *AgentInstance) RunTurn(ctx context.Context, opts ProcessOptions) (*TurnResult, error) {
	if strings.TrimSpace(opts.SessionKey) == "" {
		return nil, fmt.Errorf("session key is required")
	}
	if strings.TrimSpace(opts.UserMessage) == "" && len(opts.Media) == 0 {
		return nil, fmt.Errorf("user message or media is required")
	}

	var history []providers.Message
	var summary string
	if !opts.NoHistory {
		// 允许历史时，先恢复 session 里的对话上下文和摘要。
		history = a.Sessions.GetHistory(opts.SessionKey)
		summary = a.Sessions.GetSummary(opts.SessionKey)
	}

	// 把历史、摘要和本轮输入组装成 provider 可以直接消费的消息序列。
	messages := a.ContextBuilder.BuildMessages(
		history,
		summary,
		opts.UserMessage,
		opts.Media,
		opts.Channel,
		opts.ChatID,
	)
	// 先解析媒体引用，确保发给模型的是可用资源而不是占位符。
	messages = resolveMediaRefs(messages, opts.MediaStore, defaultMaxMediaSize)

	// 先写入用户消息，保证后续任何失败都不会丢失这轮输入。
	userMessage := providers.Message{
		Role:    "user",
		Content: opts.UserMessage,
	}
	if len(opts.Media) > 0 {
		userMessage.Media = opts.Media
	}
	a.Sessions.AddFullMessage(opts.SessionKey, userMessage)

	finalContent, iterations, transcript, responseHandled, err := a.runLLMIteration(ctx, messages, opts)
	for _, msg := range transcript {
		a.Sessions.AddFullMessage(opts.SessionKey, msg)
	}
	if err != nil {
		if saveErr := a.Sessions.Save(opts.SessionKey); saveErr != nil {
			return nil, saveErr
		}
		return nil, err
	}

	if finalContent == "" && !responseHandled {
		finalContent = opts.DefaultResponse
	}

	if !responseHandled {
		a.Sessions.AddFullMessage(opts.SessionKey, providers.Message{
			Role:    "assistant",
			Content: finalContent,
		})
	}

	if err := a.Sessions.Save(opts.SessionKey); err != nil {
		return nil, err
	}

	return &TurnResult{
		Content:         finalContent,
		Iterations:      iterations,
		ResponseHandled: responseHandled,
	}, nil
}

// runLLMIteration 负责在一次 turn 内不断调用模型和工具，直到拿到最终答复或达到上限。
func (a *AgentInstance) runLLMIteration(
	ctx context.Context,
	messages []providers.Message,
	opts ProcessOptions,
) (finalContent string, iterations int, transcript []providers.Message, responseHandled bool, err error) {
	transcript = []providers.Message{}

	for iterations < a.MaxIterations {
		iterations++

		var toolDefs []providers.ToolDefinition
		if a.Tools != nil {
			// 每轮都按当前注册表生成工具定义，避免工具列表过期。
			toolDefs = a.Tools.ToProviderDefs()
		}

		options := map[string]any{
			"max_tokens":  a.MaxTokens,
			"temperature": a.Temperature,
		}

		response, err := a.Provider.Chat(ctx, messages, toolDefs, a.Model, options)
		if err != nil {
			return "", iterations, transcript, responseHandled, fmt.Errorf("LLM call failed: %w", err)
		}

		if len(response.ToolCalls) == 0 {
			// 没有工具调用时，直接把模型正文或 reasoning 当作最终输出。
			finalContent = response.Content
			if finalContent == "" && response.ReasoningContent != "" {
				finalContent = response.ReasoningContent
			}
			return finalContent, iterations, transcript, responseHandled, nil
		}

		normalizedToolCalls := make([]providers.ToolCall, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			normalizedToolCalls = append(normalizedToolCalls, tools.NormalizeToolCall(tc))
		}

		// 把这次 assistant 输出和工具调用一起记入消息流，供后续工具回填。
		assistantMsg := providers.Message{
			Role:             "assistant",
			Content:          response.Content,
			ReasoningContent: response.ReasoningContent,
		}
		for _, tc := range normalizedToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, providers.ToolCall{
				ID:        tc.ID,
				Type:      "function",
				Name:      tc.Name,
				Arguments: tc.Arguments,
				Function: &providers.FunctionCall{
					Name:      tc.Name,
					Arguments: string(argumentsJSON),
				},
			})
		}

		messages = append(messages, assistantMsg)
		transcript = append(transcript, assistantMsg)

		// 逐个执行工具，并把结果作为 tool 消息喂回模型。
		allResponsesHandled := len(normalizedToolCalls) > 0
		for _, tc := range normalizedToolCalls {
			var result *tools.ToolResult
			if a.Tools != nil {
				result = a.Tools.ExecuteWithContext(ctx, tc.Name, tc.Arguments, opts.Channel, opts.ChatID, nil)
			} else {
				result = tools.ErrorResult("No tools available")
			}
			if result == nil {
				result = tools.ErrorResult("tool returned nil result").WithError(fmt.Errorf("nil tool result"))
			}

			if err := publishToolMedia(ctx, opts, result.Media); err != nil {
				return "", iterations, transcript, responseHandled, err
			}

			if !result.ResponseHandled {
				allResponsesHandled = false
			}

			contentForLLM := result.ContentForLLM()

			toolMsg := providers.Message{
				Role:       "tool",
				Content:    contentForLLM,
				ToolCallID: tc.ID,
			}
			messages = append(messages, toolMsg)
			transcript = append(transcript, toolMsg)
		}

		if allResponsesHandled {
			// 如果所有工具都已经完全处理响应，就不再继续向模型追问。
			return "", iterations, transcript, true, nil
		}
	}

	return finalContent, iterations, transcript, responseHandled, nil
}
