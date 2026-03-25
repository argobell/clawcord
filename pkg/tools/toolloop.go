package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/argobell/clawcord/pkg/logger"
	"github.com/argobell/clawcord/pkg/providers"
	"github.com/argobell/clawcord/pkg/utils"
)

// ToolLoopConfig 定义一次工具循环运行所需的依赖和限制。
type ToolLoopConfig struct {
	Provider      providers.LLMProvider
	Model         string
	Tools         *ToolRegistry
	MaxIterations int
	LLMOptions    map[string]any
}

// ToolLoopResult 表示循环结束后的最终文本和迭代次数。
type ToolLoopResult struct {
	Content    string
	Iterations int
}

// RunToolLoop 执行最小的 LLM -> tools -> LLM 闭环。
// 当模型不再请求工具，或达到最大迭代次数时结束。
func RunToolLoop(
	ctx context.Context,
	config ToolLoopConfig,
	messages []providers.Message,
	channel, chatID string,
) (*ToolLoopResult, error) {
	iteration := 0
	var finalContent string

	for iteration < config.MaxIterations {
		iteration++

		logger.DebugCF("toolloop", "LLM iteration", map[string]any{
			"iteration": iteration,
			"max":       config.MaxIterations,
		})

		var providerToolDefs []providers.ToolDefinition
		if config.Tools != nil {
			providerToolDefs = config.Tools.ToProviderDefs()
		}

		llmOpts := config.LLMOptions
		if llmOpts == nil {
			llmOpts = map[string]any{}
		}

		response, err := config.Provider.Chat(ctx, messages, providerToolDefs, config.Model, llmOpts)
		if err != nil {
			logger.ErrorCF("toolloop", "LLM call failed", map[string]any{
				"iteration": iteration,
				"error":     err.Error(),
			})
			return nil, fmt.Errorf("LLM call failed: %w", err)
		}

		if len(response.ToolCalls) == 0 {
			finalContent = response.Content
			logger.InfoCF("toolloop", "LLM response without tool calls (direct answer)", map[string]any{
				"iteration":     iteration,
				"content_chars": len(finalContent),
			})
			break
		}

		// 兼容 provider 返回的不同 tool_call 形态，统一成运行时可执行结构。
		normalizedToolCalls := make([]providers.ToolCall, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			normalizedToolCalls = append(normalizedToolCalls, normalizeToolCall(tc))
		}

		toolNames := make([]string, 0, len(normalizedToolCalls))
		for _, tc := range normalizedToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		logger.InfoCF("toolloop", "LLM requested tool calls", map[string]any{
			"tools":     toolNames,
			"count":     len(normalizedToolCalls),
			"iteration": iteration,
		})

		assistantMsg := providers.Message{
			Role:    "assistant",
			Content: response.Content,
		}
		for _, tc := range normalizedToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			// 把归一化后的工具调用重新写回 assistant 消息，供下一轮模型继续消费。
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

		type indexedResult struct {
			result *ToolResult
			tc     providers.ToolCall
		}

		results := make([]indexedResult, len(normalizedToolCalls))
		var wg sync.WaitGroup

		// 工具执行彼此独立时并行运行，最终再按原顺序写回消息历史。
		for i, tc := range normalizedToolCalls {
			results[i].tc = tc

			wg.Add(1)
			go func(idx int, tc providers.ToolCall) {
				defer wg.Done()

				argsJSON, _ := json.Marshal(tc.Arguments)
				argsPreview := utils.Truncate(string(argsJSON), 200)
				logger.InfoCF("toolloop", fmt.Sprintf("Tool call: %s(%s)", tc.Name, argsPreview), map[string]any{
					"tool":      tc.Name,
					"iteration": iteration,
				})

				var toolResult *ToolResult
				if config.Tools != nil {
					toolResult = config.Tools.ExecuteWithContext(ctx, tc.Name, tc.Arguments, channel, chatID, nil)
				} else {
					toolResult = ErrorResult("No tools available")
				}
				results[idx].result = toolResult
			}(i, tc)
		}
		wg.Wait()

		// tool 消息必须按原调用顺序追加，否则会破坏模型上下文的一致性。
		for _, r := range results {
			contentForLLM := r.result.ForLLM
			if contentForLLM == "" && r.result.Err != nil {
				contentForLLM = r.result.Err.Error()
			}

			messages = append(messages, providers.Message{
				Role:       "tool",
				Content:    contentForLLM,
				ToolCallID: r.tc.ID,
			})
		}
	}

	return &ToolLoopResult{
		Content:    finalContent,
		Iterations: iteration,
	}, nil
}
