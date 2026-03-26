package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/argobell/clawcord/pkg/providers"
	"github.com/argobell/clawcord/pkg/tools"
)

// TurnInput 描述一次最小 agent turn 所需的输入。
type TurnInput struct {
	SessionKey      string
	Channel         string
	ChatID          string
	UserMessage     string
	Media           []string
	DefaultResponse string
	NoHistory       bool
}

// TurnResult 表示单轮执行结束后的最终输出。
type TurnResult struct {
	Content    string
	Iterations int
}

// RunTurn 执行一次最小 agent turn，并按顺序持久化本轮新增消息。
func (a *AgentInstance) RunTurn(ctx context.Context, input TurnInput) (*TurnResult, error) {
	if strings.TrimSpace(input.SessionKey) == "" {
		return nil, fmt.Errorf("session key is required")
	}
	if strings.TrimSpace(input.UserMessage) == "" && len(input.Media) == 0 {
		return nil, fmt.Errorf("user message or media is required")
	}

	var history []providers.Message
	var summary string
	if !input.NoHistory {
		history = a.Sessions.GetHistory(input.SessionKey)
		summary = a.Sessions.GetSummary(input.SessionKey)
	}

	messages := a.ContextBuilder.BuildMessages(
		history,
		summary,
		input.UserMessage,
		input.Media,
		input.Channel,
		input.ChatID,
	)

	userMessage := providers.Message{
		Role:    "user",
		Content: input.UserMessage,
	}
	if len(input.Media) > 0 {
		userMessage.Media = input.Media
	}
	a.Sessions.AddFullMessage(input.SessionKey, userMessage)

	finalContent, iterations, transcript, err := a.runLLMIteration(ctx, messages, input)
	for _, msg := range transcript {
		a.Sessions.AddFullMessage(input.SessionKey, msg)
	}
	if err != nil {
		if saveErr := a.Sessions.Save(input.SessionKey); saveErr != nil {
			return nil, saveErr
		}
		return nil, err
	}

	if finalContent == "" {
		finalContent = input.DefaultResponse
	}

	a.Sessions.AddFullMessage(input.SessionKey, providers.Message{
		Role:    "assistant",
		Content: finalContent,
	})

	if err := a.Sessions.Save(input.SessionKey); err != nil {
		return nil, err
	}

	return &TurnResult{
		Content:    finalContent,
		Iterations: iterations,
	}, nil
}

func (a *AgentInstance) runLLMIteration(
	ctx context.Context,
	messages []providers.Message,
	input TurnInput,
) (finalContent string, iterations int, transcript []providers.Message, err error) {
	transcript = []providers.Message{}

	for iterations < a.MaxIterations {
		iterations++

		var toolDefs []providers.ToolDefinition
		if a.Tools != nil {
			toolDefs = a.Tools.ToProviderDefs()
		}

		options := map[string]any{
			"max_tokens":  a.MaxTokens,
			"temperature": a.Temperature,
		}

		response, err := a.Provider.Chat(ctx, messages, toolDefs, a.Model, options)
		if err != nil {
			return "", iterations, transcript, fmt.Errorf("LLM call failed: %w", err)
		}

		if len(response.ToolCalls) == 0 {
			finalContent = response.Content
			if finalContent == "" && response.ReasoningContent != "" {
				finalContent = response.ReasoningContent
			}
			return finalContent, iterations, transcript, nil
		}

		normalizedToolCalls := make([]providers.ToolCall, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			normalizedToolCalls = append(normalizedToolCalls, tools.NormalizeToolCall(tc))
		}

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

		for _, tc := range normalizedToolCalls {
			var result *tools.ToolResult
			if a.Tools != nil {
				result = a.Tools.ExecuteWithContext(ctx, tc.Name, tc.Arguments, input.Channel, input.ChatID, nil)
			} else {
				result = tools.ErrorResult("No tools available")
			}
			if result == nil {
				result = tools.ErrorResult("tool returned nil result").WithError(fmt.Errorf("nil tool result"))
			}

			contentForLLM := result.ForLLM
			if contentForLLM == "" && result.Err != nil {
				contentForLLM = result.Err.Error()
			}

			toolMsg := providers.Message{
				Role:       "tool",
				Content:    contentForLLM,
				ToolCallID: tc.ID,
			}
			messages = append(messages, toolMsg)
			transcript = append(transcript, toolMsg)
		}
	}

	return finalContent, iterations, transcript, nil
}
