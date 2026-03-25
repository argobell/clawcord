package tools

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/argobell/clawcord/pkg/providers"
)

type mockRegistryTool struct {
	name   string
	desc   string
	params map[string]any
	result *ToolResult
}

func (m *mockRegistryTool) Name() string               { return m.name }
func (m *mockRegistryTool) Description() string        { return m.desc }
func (m *mockRegistryTool) Parameters() map[string]any { return m.params }
func (m *mockRegistryTool) Execute(_ context.Context, _ map[string]any) *ToolResult {
	return m.result
}

type mockContextAwareTool struct {
	mockRegistryTool
	lastCtx context.Context
}

func (m *mockContextAwareTool) Execute(ctx context.Context, _ map[string]any) *ToolResult {
	m.lastCtx = ctx
	return m.result
}

type mockAsyncRegistryTool struct {
	mockRegistryTool
	lastCB AsyncCallback
}

func (m *mockAsyncRegistryTool) ExecuteAsync(_ context.Context, args map[string]any, cb AsyncCallback) *ToolResult {
	m.lastCB = cb
	return m.result
}

func newMockTool(name, desc string) *mockRegistryTool {
	return &mockRegistryTool{
		name:   name,
		desc:   desc,
		params: map[string]any{"type": "object"},
		result: SilentResult("ok"),
	}
}

func TestNewToolRegistry(t *testing.T) {
	r := NewToolRegistry()
	if r.Count() != 0 {
		t.Fatalf("expected empty registry, got %d", r.Count())
	}
	if len(r.List()) != 0 {
		t.Fatalf("expected empty list, got %v", r.List())
	}
}

func TestToolRegistryRegisterAndGet(t *testing.T) {
	r := NewToolRegistry()
	r.Register(newMockTool("echo", "echoes input"))

	got, ok := r.Get("echo")
	if !ok {
		t.Fatal("expected to find registered tool")
	}
	if got.Name() != "echo" {
		t.Fatalf("expected name echo, got %q", got.Name())
	}
}

func TestToolRegistryExecuteNotFound(t *testing.T) {
	r := NewToolRegistry()
	result := r.Execute(context.Background(), "missing", nil)
	if !result.IsError {
		t.Fatal("expected missing tool to return error")
	}
	if !strings.Contains(result.ForLLM, "not found") {
		t.Fatalf("expected not found error, got %q", result.ForLLM)
	}
	if result.Err == nil {
		t.Fatal("expected underlying error to be preserved")
	}
}

func TestToolRegistryExecuteWithContextInjectsToolContext(t *testing.T) {
	r := NewToolRegistry()
	ct := &mockContextAwareTool{
		mockRegistryTool: *newMockTool("ctx_tool", "needs context"),
	}
	r.Register(ct)

	r.ExecuteWithContext(context.Background(), "ctx_tool", nil, "discord", "chat-42", nil)

	if ct.lastCtx == nil {
		t.Fatal("expected Execute to be called")
	}
	if got := ToolChannel(ct.lastCtx); got != "discord" {
		t.Fatalf("expected channel discord, got %q", got)
	}
	if got := ToolChatID(ct.lastCtx); got != "chat-42" {
		t.Fatalf("expected chatID chat-42, got %q", got)
	}
}

func TestToolRegistryExecuteWithContextAsyncCallback(t *testing.T) {
	r := NewToolRegistry()
	at := &mockAsyncRegistryTool{
		mockRegistryTool: *newMockTool("async_tool", "async work"),
	}
	at.result = AsyncResult("started")
	r.Register(at)

	called := false
	cb := func(_ context.Context, _ *ToolResult) { called = true }

	result := r.ExecuteWithContext(context.Background(), "async_tool", nil, "", "", cb)
	if at.lastCB == nil {
		t.Fatal("expected ExecuteAsync to receive callback")
	}
	if !result.Async {
		t.Fatal("expected async result")
	}

	at.lastCB(context.Background(), SilentResult("done"))
	if !called {
		t.Fatal("expected callback to be invoked")
	}
}

func TestToolRegistryToProviderDefs(t *testing.T) {
	r := NewToolRegistry()
	params := map[string]any{"type": "object", "properties": map[string]any{}}
	r.Register(&mockRegistryTool{
		name:   "beta",
		desc:   "tool B",
		params: params,
		result: SilentResult("ok"),
	})

	defs := r.ToProviderDefs()
	if len(defs) != 1 {
		t.Fatalf("expected 1 provider def, got %d", len(defs))
	}

	want := providers.ToolDefinition{
		Type: "function",
		Function: providers.ToolFunctionDefinition{
			Name:        "beta",
			Description: "tool B",
			Parameters:  params,
		},
	}
	if defs[0].Function.Name != want.Function.Name {
		t.Fatalf("expected name %q, got %q", want.Function.Name, defs[0].Function.Name)
	}
}

func TestToolRegistryListCountAndSummaries(t *testing.T) {
	r := NewToolRegistry()
	r.Register(newMockTool("zeta", "last"))
	r.Register(newMockTool("alpha", "first"))

	names := r.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "zeta" {
		t.Fatalf("expected sorted names, got %v", names)
	}
	if r.Count() != 2 {
		t.Fatalf("expected count 2, got %d", r.Count())
	}

	summaries := r.GetSummaries()
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	if !strings.Contains(summaries[0], "`alpha`") {
		t.Fatalf("expected summary to include tool name, got %q", summaries[0])
	}
}

func TestToolRegistryConcurrentAccess(t *testing.T) {
	r := NewToolRegistry()
	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := string(rune('A' + n%26))
			r.Register(newMockTool(name, "concurrent"))
			r.Get(name)
			r.Count()
			r.List()
			r.GetDefinitions()
		}(i)
	}

	wg.Wait()

	if r.Count() == 0 {
		t.Fatal("expected tools to be registered after concurrent access")
	}
}
