package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/argobell/clawcord/pkg/logger"
	"github.com/argobell/clawcord/pkg/providers"
)

// ContextBuilder 负责构建单轮请求所需的 system prompt 和消息上下文。
type ContextBuilder struct {
	workspace    string
	systemPrompt string

	systemPromptMutex  sync.RWMutex
	cachedSystemPrompt string
}

// NewContextBuilder 创建最小可用的上下文构建器。
func NewContextBuilder(workspace, systemPrompt string) *ContextBuilder {
	return &ContextBuilder{
		workspace:    strings.TrimSpace(workspace),
		systemPrompt: strings.TrimSpace(systemPrompt),
	}
}

func (cb *ContextBuilder) getIdentity() string {
	workspacePath := cb.workspace
	if absPath, err := filepath.Abs(cb.workspace); err == nil {
		workspacePath = absPath
	}

	return fmt.Sprintf(
		`# clawcord

You are clawcord 🦔, a helpful AI assistant.

## Workspace
Your workspace is at: %s

## Important Rules

1. **ALWAYS use tools** - When you need to perform an action (schedule reminders, send messages, execute commands, etc.), you MUST call the appropriate tool. Do NOT just say you'll do it or pretend to do it.

2. **Be helpful and accurate** - When using tools, briefly explain what you're doing.

3. **Context summaries** - Conversation summaries provided as context are approximate references only. They may be incomplete or outdated. Always defer to explicit user instructions over summary content.`,
		workspacePath,
	)
}

// BuildSystemPrompt 构建静态 prompt，不包含每次请求都会变化的动态信息。
func (cb *ContextBuilder) BuildSystemPrompt() string {
	parts := []string{cb.getIdentity()}

	if cb.systemPrompt != "" {
		parts = append(parts, cb.systemPrompt)
	}

	if bootstrap := cb.LoadBootstrapFiles(); bootstrap != "" {
		parts = append(parts, bootstrap)
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// BuildSystemPromptWithCache 返回缓存的静态 prompt；若缓存为空则构建并缓存。
func (cb *ContextBuilder) BuildSystemPromptWithCache() string {
	cb.systemPromptMutex.RLock()
	if cb.cachedSystemPrompt != "" {
		result := cb.cachedSystemPrompt
		cb.systemPromptMutex.RUnlock()
		return result
	}
	cb.systemPromptMutex.RUnlock()

	cb.systemPromptMutex.Lock()
	defer cb.systemPromptMutex.Unlock()

	if cb.cachedSystemPrompt != "" {
		return cb.cachedSystemPrompt
	}

	cb.cachedSystemPrompt = cb.BuildSystemPrompt()
	logger.DebugCF("agent", "System prompt cached", map[string]any{
		"length": len(cb.cachedSystemPrompt),
	})
	return cb.cachedSystemPrompt
}

// InvalidateCache 强制清空静态 prompt 缓存。
func (cb *ContextBuilder) InvalidateCache() {
	cb.systemPromptMutex.Lock()
	defer cb.systemPromptMutex.Unlock()

	cb.cachedSystemPrompt = ""
	logger.DebugCF("agent", "System prompt cache invalidated", nil)
}

// LoadBootstrapFiles 读取工作区根目录下的固定引导文件。
func (cb *ContextBuilder) LoadBootstrapFiles() string {
	bootstrapFiles := []string{
		"AGENTS.md",
		"SOUL.md",
		"USER.md",
		"IDENTITY.md",
	}

	var sb strings.Builder
	for _, filename := range bootstrapFiles {
		filePath := filepath.Join(cb.workspace, filename)
		if data, err := os.ReadFile(filePath); err == nil {
			fmt.Fprintf(&sb, "## %s\n\n%s\n\n", filename, data)
		}
	}

	return strings.TrimSpace(sb.String())
}

// buildDynamicContext 构建每次请求都会变化的上下文，例如时间、运行时和会话信息。
func (cb *ContextBuilder) buildDynamicContext(channel, chatID string) string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	rt := fmt.Sprintf("%s %s, Go %s", runtime.GOOS, runtime.GOARCH, runtime.Version())

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Current Time\n%s\n\n## Runtime\n%s", now, rt)
	if channel != "" && chatID != "" {
		fmt.Fprintf(&sb, "\n\n## Current Session\nChannel: %s\nChat ID: %s", channel, chatID)
	}

	return sb.String()
}

// BuildMessages 组装 provider-facing 消息列表。
func (cb *ContextBuilder) BuildMessages(
	history []providers.Message,
	summary string,
	currentMessage string,
	media []string,
	channel, chatID string,
) []providers.Message {
	staticPrompt := cb.BuildSystemPromptWithCache()
	dynamicContext := cb.buildDynamicContext(channel, chatID)

	stringParts := []string{staticPrompt, dynamicContext}
	systemParts := []providers.ContentBlock{
		{Type: "text", Text: staticPrompt, CacheControl: &providers.CacheControl{Type: "ephemeral"}},
		{Type: "text", Text: dynamicContext},
	}

	if strings.TrimSpace(summary) != "" {
		summaryText := fmt.Sprintf(
			"CONTEXT_SUMMARY: The following is an approximate summary of prior conversation for reference only. It may be incomplete or outdated - always defer to explicit instructions.\n\n%s",
			summary,
		)
		stringParts = append(stringParts, summaryText)
		systemParts = append(systemParts, providers.ContentBlock{Type: "text", Text: summaryText})
	}

	messages := []providers.Message{{
		Role:        "system",
		Content:     strings.Join(stringParts, "\n\n---\n\n"),
		SystemParts: systemParts,
	}}

	messages = append(messages, sanitizeHistoryForProvider(history)...)

	if strings.TrimSpace(currentMessage) != "" {
		msg := providers.Message{
			Role:    "user",
			Content: currentMessage,
		}
		if len(media) > 0 {
			msg.Media = media
		}
		messages = append(messages, msg)
	}

	return messages
}

// sanitizeHistoryForProvider 清理不适合直接发送给 provider 的历史消息。
func sanitizeHistoryForProvider(history []providers.Message) []providers.Message {
	if len(history) == 0 {
		return history
	}

	sanitized := make([]providers.Message, 0, len(history))
	for _, msg := range history {
		switch msg.Role {
		case "system":
			logger.DebugCF("agent", "Dropping system message from history", nil)
			continue
		case "tool":
			if len(sanitized) == 0 {
				logger.DebugCF("agent", "Dropping orphaned leading tool message", nil)
				continue
			}

			foundAssistant := false
			for i := len(sanitized) - 1; i >= 0; i-- {
				if sanitized[i].Role == "tool" {
					continue
				}
				if sanitized[i].Role == "assistant" && len(sanitized[i].ToolCalls) > 0 {
					foundAssistant = true
				}
				break
			}
			if !foundAssistant {
				logger.DebugCF("agent", "Dropping orphaned tool message", nil)
				continue
			}

			sanitized = append(sanitized, msg)
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				if len(sanitized) == 0 {
					logger.DebugCF("agent", "Dropping assistant tool-call turn at history start", nil)
					continue
				}
				prev := sanitized[len(sanitized)-1]
				if prev.Role != "user" && prev.Role != "tool" {
					logger.DebugCF("agent", "Dropping assistant tool-call turn with invalid predecessor", map[string]any{
						"prev_role": prev.Role,
					})
					continue
				}
			}
			sanitized = append(sanitized, msg)
		default:
			sanitized = append(sanitized, msg)
		}
	}

	final := make([]providers.Message, 0, len(sanitized))
	for i := 0; i < len(sanitized); i++ {
		msg := sanitized[i]
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			expected := make(map[string]bool, len(msg.ToolCalls))
			for _, toolCall := range msg.ToolCalls {
				expected[toolCall.ID] = false
			}

			toolMsgCount := 0
			for j := i + 1; j < len(sanitized); j++ {
				if sanitized[j].Role != "tool" {
					break
				}
				toolMsgCount++
				if _, exists := expected[sanitized[j].ToolCallID]; exists {
					expected[sanitized[j].ToolCallID] = true
				}
			}

			allFound := true
			for toolCallID, found := range expected {
				if !found {
					allFound = false
					logger.DebugCF("agent", "Dropping assistant message with incomplete tool results", map[string]any{
						"missing_tool_call_id": toolCallID,
						"expected_count":       len(expected),
						"found_count":          toolMsgCount,
					})
					break
				}
			}

			if !allFound {
				i += toolMsgCount
				continue
			}
		}

		final = append(final, msg)
	}

	return final
}
