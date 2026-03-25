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

// ToolRegistry 管理已注册工具，并负责将工具暴露给执行层和 provider 层。
type ToolRegistry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// NewToolRegistry 创建一个空的工具注册表。
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register 注册工具；同名工具会被新实例覆盖。
func (r *ToolRegistry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		logger.WarnCF("tools", "Tool registration overwrites existing tool", map[string]any{"name": name})
	}
	r.tools[name] = tool
	logger.DebugCF("tools", "Registered core tool", map[string]any{"name": name})
}

// Get 按名称读取工具。
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// Execute 在不注入 channel/chatID 的情况下执行工具。
func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]any) *ToolResult {
	return r.ExecuteWithContext(ctx, name, args, "", "", nil)
}

// ExecuteWithContext 执行工具，并把请求级上下文注入到 ctx 中。
// 如果工具实现了 AsyncExecutor 且提供了回调，则走异步执行路径。
func (r *ToolRegistry) ExecuteWithContext(
	ctx context.Context,
	name string,
	args map[string]any,
	channel, chatID string,
	asyncCallback AsyncCallback,
) *ToolResult {
	logger.InfoCF("tool", "Tool execution started", map[string]any{
		"tool": name,
		"args": args,
	})

	tool, ok := r.Get(name)
	if !ok {
		logger.ErrorCF("tool", "Tool not found", map[string]any{"tool": name})
		return ErrorResult(fmt.Sprintf("tool %q not found", name)).WithError(fmt.Errorf("tool not found"))
	}

	ctx = WithToolContext(ctx, channel, chatID)

	var result *ToolResult
	// 记录实际执行耗时，便于后续从日志中观察慢工具。
	start := time.Now()
	if asyncExec, ok := tool.(AsyncExecutor); ok && asyncCallback != nil {
		logger.DebugCF("tool", "Executing async tool via ExecuteAsync", map[string]any{"tool": name})
		result = asyncExec.ExecuteAsync(ctx, args, asyncCallback)
	} else {
		result = tool.Execute(ctx, args)
	}
	duration := time.Since(start)

	if result.IsError {
		logger.ErrorCF("tool", "Tool execution failed", map[string]any{
			"tool":     name,
			"duration": duration.Milliseconds(),
			"error":    result.ForLLM,
		})
	} else if result.Async {
		logger.InfoCF("tool", "Tool started (async)", map[string]any{
			"tool":     name,
			"duration": duration.Milliseconds(),
		})
	} else {
		logger.InfoCF("tool", "Tool execution completed", map[string]any{
			"tool":          name,
			"duration_ms":   duration.Milliseconds(),
			"result_length": len(result.ForLLM),
		})
	}

	return result
}

// sortedToolNames 返回稳定排序后的工具名，避免 map 迭代顺序不确定。
func (r *ToolRegistry) sortedToolNames() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetDefinitions 返回通用 map 结构的工具定义。
func (r *ToolRegistry) GetDefinitions() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sorted := r.sortedToolNames()
	definitions := make([]map[string]any, 0, len(sorted))
	for _, name := range sorted {
		definitions = append(definitions, ToolToSchema(r.tools[name]))
	}
	return definitions
}

// ToProviderDefs 将工具定义转换为 provider 层使用的结构化定义。
func (r *ToolRegistry) ToProviderDefs() []providers.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sorted := r.sortedToolNames()
	definitions := make([]providers.ToolDefinition, 0, len(sorted))
	for _, name := range sorted {
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

// List 返回排序后的工具名列表。
func (r *ToolRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sortedToolNames()
}

// Count 返回已注册工具数量。
func (r *ToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// GetSummaries 返回适合展示给模型或调试日志的工具摘要。
func (r *ToolRegistry) GetSummaries() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sorted := r.sortedToolNames()
	summaries := make([]string, 0, len(sorted))
	for _, name := range sorted {
		tool := r.tools[name]
		summaries = append(summaries, fmt.Sprintf("- `%s` - %s", tool.Name(), tool.Description()))
	}
	return summaries
}
