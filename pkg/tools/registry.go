package tools

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/argobell/clawcord/pkg/logger"
	"github.com/argobell/clawcord/pkg/providers"
)

// ToolRegistry 管理已注册工具及其 provider schema。
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

func (r *ToolRegistry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		logger.WarnCF("tools", "Tool registration overwrites existing tool", map[string]any{
			"name": name,
		})
	}
	r.tools[name] = tool
	logger.DebugCF("tools", "Registered tool", map[string]any{
		"name": name,
	})
}

func (r *ToolRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]any) *ToolResult {
	return r.ExecuteWithContext(ctx, name, args, "", "", nil)
}

func (r *ToolRegistry) ExecuteWithContext(
	ctx context.Context,
	name string,
	args map[string]any,
	channel, chatID string,
	asyncCallback AsyncCallback,
) *ToolResult {
	logFields := map[string]any{
		"tool": name,
		"args": args,
	}
	if channel != "" {
		logFields["channel"] = channel
	}
	if chatID != "" {
		logFields["chat_id"] = chatID
	}

	logger.InfoCF("tool", "Tool execution started", logFields)

	tool, ok := r.Get(name)
	if !ok {
		logger.ErrorCF("tool", "Tool not found", map[string]any{
			"tool":    name,
			"channel": channel,
			"chat_id": chatID,
		})
		return ErrorResult(fmt.Sprintf("tool %q not found", name)).WithError(fmt.Errorf("tool not found"))
	}

	ctx = WithToolContext(ctx, channel, chatID)

	start := time.Now()
	if asyncTool, ok := tool.(AsyncExecutor); ok && asyncCallback != nil {
		logger.DebugCF("tool", "Executing async tool via ExecuteAsync", map[string]any{
			"tool":    name,
			"channel": channel,
			"chat_id": chatID,
		})
		result := asyncTool.ExecuteAsync(ctx, args, asyncCallback)
		logger.InfoCF("tool", "Tool started (async)", map[string]any{
			"tool":        name,
			"duration_ms": time.Since(start).Milliseconds(),
			"channel":     channel,
			"chat_id":     chatID,
		})
		return result
	}

	result := tool.Execute(ctx, args)
	durationMs := time.Since(start).Milliseconds()
	if result != nil && result.IsError {
		logger.ErrorCF("tool", "Tool execution failed", map[string]any{
			"tool":        name,
			"duration_ms": durationMs,
			"error":       result.ForLLM,
			"channel":     channel,
			"chat_id":     chatID,
		})
		return result
	}

	logger.InfoCF("tool", "Tool execution completed", map[string]any{
		"tool":          name,
		"duration_ms":   durationMs,
		"result_length": len(result.ForLLM),
		"channel":       channel,
		"chat_id":       chatID,
	})

	return result
}

func (r *ToolRegistry) GetDefinitions() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := r.sortedToolNamesLocked()
	definitions := make([]map[string]any, 0, len(names))
	for _, name := range names {
		definitions = append(definitions, ToolToSchema(r.tools[name]))
	}
	return definitions
}

func (r *ToolRegistry) ToProviderDefs() []providers.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := r.sortedToolNamesLocked()
	definitions := make([]providers.ToolDefinition, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		definitions = append(definitions, providers.ToolDefinition{
			Type: "function",
			Function: providers.ToolFunctionDefinition{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.Parameters(),
			},
		})
	}
	return definitions
}

func (r *ToolRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sortedToolNamesLocked()
}

func (r *ToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

func (r *ToolRegistry) GetSummaries() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := r.sortedToolNamesLocked()
	summaries := make([]string, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		summaries = append(summaries, fmt.Sprintf("- `%s` - %s", tool.Name(), tool.Description()))
	}
	return summaries
}

func (r *ToolRegistry) sortedToolNamesLocked() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
