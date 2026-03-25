package tools

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/argobell/clawcord/pkg/logger"
	"github.com/argobell/clawcord/pkg/providers"
)

type mockRegistryTool struct {
	name    string
	desc    string
	params  map[string]any
	result  *ToolResult
	lastCtx context.Context
}

func (m *mockRegistryTool) Name() string               { return m.name }
func (m *mockRegistryTool) Description() string        { return m.desc }
func (m *mockRegistryTool) Parameters() map[string]any { return m.params }
func (m *mockRegistryTool) Execute(ctx context.Context, _ map[string]any) *ToolResult {
	m.lastCtx = ctx
	return m.result
}

func newMockRegistryTool(name, desc string) *mockRegistryTool {
	return &mockRegistryTool{
		name:   name,
		desc:   desc,
		params: map[string]any{"type": "object"},
		result: SilentResult("ok"),
	}
}

type mockAsyncRegistryTool struct {
	*mockRegistryTool
	executeCalled      bool
	executeAsyncCalled bool
	lastAsyncArgs      map[string]any
	lastCallback       AsyncCallback
}

func (m *mockAsyncRegistryTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	m.executeCalled = true
	return m.mockRegistryTool.Execute(ctx, args)
}

func (m *mockAsyncRegistryTool) ExecuteAsync(ctx context.Context, args map[string]any, callback AsyncCallback) *ToolResult {
	m.executeAsyncCalled = true
	m.lastCtx = ctx
	m.lastAsyncArgs = args
	m.lastCallback = callback
	return AsyncResult("started")
}

func TestToolRegistryRegisterGetAndExecute(t *testing.T) {
	registry := NewToolRegistry()
	tool := newMockRegistryTool("echo", "echo input")
	tool.result = UserResult("hello")
	registry.Register(tool)

	got, ok := registry.Get("echo")
	if !ok {
		t.Fatal("expected registered tool to be found")
	}
	if got.Name() != "echo" {
		t.Fatalf("expected tool name %q, got %q", "echo", got.Name())
	}

	result := registry.Execute(context.Background(), "echo", map[string]any{"text": "hello"})
	if result.IsError {
		t.Fatalf("expected success result, got %q", result.ForLLM)
	}
	if result.ForLLM != "hello" {
		t.Fatalf("expected ForLLM %q, got %q", "hello", result.ForLLM)
	}
}

func TestToolRegistryExecuteMissingTool(t *testing.T) {
	registry := NewToolRegistry()

	result := registry.Execute(context.Background(), "missing", nil)
	if !result.IsError {
		t.Fatal("expected missing tool to return error result")
	}
	if result.Err == nil {
		t.Fatal("expected missing tool error to preserve Err")
	}
	if !strings.Contains(result.ForLLM, "not found") {
		t.Fatalf("expected not found message, got %q", result.ForLLM)
	}
}

func TestToolRegistryExecuteWithContextInjectsToolContext(t *testing.T) {
	registry := NewToolRegistry()
	tool := newMockRegistryTool("ctx", "needs context")
	registry.Register(tool)

	registry.ExecuteWithContext(context.Background(), "ctx", nil, "discord", "chat-1", nil)

	if tool.lastCtx == nil {
		t.Fatal("expected tool to receive context")
	}
	if got := ToolChannel(tool.lastCtx); got != "discord" {
		t.Fatalf("expected channel %q, got %q", "discord", got)
	}
	if got := ToolChatID(tool.lastCtx); got != "chat-1" {
		t.Fatalf("expected chatID %q, got %q", "chat-1", got)
	}
}

func TestToolRegistryExecuteWithContextUsesAsyncExecutorWhenCallbackProvided(t *testing.T) {
	registry := NewToolRegistry()
	tool := &mockAsyncRegistryTool{
		mockRegistryTool: newMockRegistryTool("async", "runs async"),
	}
	registry.Register(tool)

	callback := func(context.Context, *ToolResult) {}
	args := map[string]any{"job": "sync"}
	result := registry.ExecuteWithContext(context.Background(), "async", args, "discord", "chat-2", callback)

	if !tool.executeAsyncCalled {
		t.Fatal("expected ExecuteAsync to be called")
	}
	if tool.executeCalled {
		t.Fatal("expected Execute to be skipped when callback is provided")
	}
	if tool.lastCallback == nil {
		t.Fatal("expected async callback to be forwarded")
	}
	if tool.lastAsyncArgs["job"] != "sync" {
		t.Fatalf("expected async args to be forwarded, got %#v", tool.lastAsyncArgs)
	}
	if got := ToolChannel(tool.lastCtx); got != "discord" {
		t.Fatalf("expected channel %q, got %q", "discord", got)
	}
	if got := ToolChatID(tool.lastCtx); got != "chat-2" {
		t.Fatalf("expected chatID %q, got %q", "chat-2", got)
	}
	if !result.Async {
		t.Fatalf("expected async result, got %#v", result)
	}
}

func TestToolRegistryExecuteWithContextLogsLifecycle(t *testing.T) {
	registry := NewToolRegistry()
	tool := newMockRegistryTool("echo", "echo input")
	tool.result = UserResult("hello")
	registry.Register(tool)

	logPath := t.TempDir() + "/registry.log"
	previousLevel := logger.GetLevel()
	logger.SetLevel(logger.DEBUG)
	if err := logger.EnableFileLogging(logPath); err != nil {
		t.Fatalf("enable file logging: %v", err)
	}
	t.Cleanup(func() {
		logger.DisableFileLogging()
		logger.SetLevel(previousLevel)
	})

	result := registry.ExecuteWithContext(context.Background(), "echo", map[string]any{"text": "hello"}, "discord", "chat-1", nil)
	if result.IsError {
		t.Fatalf("expected success result, got %q", result.ForLLM)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	logs := string(content)
	if !strings.Contains(logs, "\"component\":\"tool\"") {
		t.Fatalf("expected tool component in logs, got %q", logs)
	}
	if !strings.Contains(logs, "\"message\":\"Tool execution started\"") {
		t.Fatalf("expected start log, got %q", logs)
	}
	if !strings.Contains(logs, "\"message\":\"Tool execution completed\"") {
		t.Fatalf("expected completion log, got %q", logs)
	}
	if !strings.Contains(logs, "\"tool\":\"echo\"") {
		t.Fatalf("expected tool name in logs, got %q", logs)
	}
	if !strings.Contains(logs, "\"channel\":\"discord\"") {
		t.Fatalf("expected channel in logs, got %q", logs)
	}
	if !strings.Contains(logs, "\"chat_id\":\"chat-1\"") {
		t.Fatalf("expected chat_id in logs, got %q", logs)
	}
}

func TestToolRegistryToProviderDefsUsesSortedOrder(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(newMockRegistryTool("zeta", "z"))
	registry.Register(newMockRegistryTool("alpha", "a"))

	defs := registry.ToProviderDefs()
	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}

	want := []providers.ToolDefinition{
		{
			Type: "function",
			Function: providers.ToolFunctionDefinition{
				Name:        "alpha",
				Description: "a",
				Parameters:  map[string]any{"type": "object"},
			},
		},
		{
			Type: "function",
			Function: providers.ToolFunctionDefinition{
				Name:        "zeta",
				Description: "z",
				Parameters:  map[string]any{"type": "object"},
			},
		},
	}

	for i := range defs {
		if defs[i].Function.Name != want[i].Function.Name {
			t.Fatalf("definition %d name: want %q, got %q", i, want[i].Function.Name, defs[i].Function.Name)
		}
	}
}
