package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/argobell/clawcord/pkg/providers"
)

type fakeProvider struct {
	responses []*providers.LLMResponse
	err       error
	calls     int
	messages  [][]providers.Message
	tools     [][]providers.ToolDefinition
}

func (f *fakeProvider) Chat(
	_ context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.calls++
	f.messages = append(f.messages, append([]providers.Message(nil), messages...))
	f.tools = append(f.tools, append([]providers.ToolDefinition(nil), tools...))
	if len(f.responses) == 0 {
		return &providers.LLMResponse{}, nil
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

func (f *fakeProvider) GetDefaultModel() string {
	return "test-model"
}

func TestRunToolLoopReturnsDirectAnswerWithoutToolCalls(t *testing.T) {
	provider := &fakeProvider{
		responses: []*providers.LLMResponse{
			{Content: "hello from model"},
		},
	}

	result, err := RunToolLoop(context.Background(), ToolLoopConfig{
		Provider:      provider,
		Model:         "test-model",
		MaxIterations: 2,
	}, []providers.Message{{Role: "user", Content: "hi"}}, "discord", "chat-1")
	if err != nil {
		t.Fatalf("RunToolLoop returned error: %v", err)
	}
	if result.Content != "hello from model" {
		t.Fatalf("expected direct answer, got %q", result.Content)
	}
	if result.Iterations != 1 {
		t.Fatalf("expected 1 iteration, got %d", result.Iterations)
	}
}

func TestRunToolLoopExecutesToolAndFeedsToolResultBack(t *testing.T) {
	provider := &fakeProvider{
		responses: []*providers.LLMResponse{
			{
				Content: "checking weather",
				ToolCalls: []providers.ToolCall{
					{
						ID:   "call-1",
						Type: "function",
						Name: "weather",
						Arguments: map[string]any{
							"city": "Shanghai",
						},
					},
				},
			},
			{Content: "sunny"},
		},
	}

	registry := NewToolRegistry()
	tool := newMockTool("weather", "weather lookup")
	tool.result = SilentResult("weather=25C")
	registry.Register(tool)

	result, err := RunToolLoop(context.Background(), ToolLoopConfig{
		Provider:      provider,
		Model:         "test-model",
		Tools:         registry,
		MaxIterations: 3,
	}, []providers.Message{{Role: "user", Content: "weather?"}}, "discord", "chat-42")
	if err != nil {
		t.Fatalf("RunToolLoop returned error: %v", err)
	}
	if result.Content != "sunny" {
		t.Fatalf("expected final content sunny, got %q", result.Content)
	}
	if result.Iterations != 2 {
		t.Fatalf("expected 2 iterations, got %d", result.Iterations)
	}
	if provider.calls != 2 {
		t.Fatalf("expected provider to be called twice, got %d", provider.calls)
	}

	secondCallMessages := provider.messages[1]
	if len(secondCallMessages) != 3 {
		t.Fatalf("expected second call to include assistant and tool messages, got %d", len(secondCallMessages))
	}
	if secondCallMessages[1].Role != "assistant" {
		t.Fatalf("expected assistant message at index 1, got %q", secondCallMessages[1].Role)
	}
	if len(secondCallMessages[1].ToolCalls) != 1 {
		t.Fatalf("expected assistant tool call to be preserved, got %d", len(secondCallMessages[1].ToolCalls))
	}
	if secondCallMessages[2].Role != "tool" {
		t.Fatalf("expected tool message at index 2, got %q", secondCallMessages[2].Role)
	}
	if secondCallMessages[2].ToolCallID != "call-1" {
		t.Fatalf("expected tool_call_id call-1, got %q", secondCallMessages[2].ToolCallID)
	}
	if secondCallMessages[2].Content != "weather=25C" {
		t.Fatalf("expected tool content weather=25C, got %q", secondCallMessages[2].Content)
	}
}

func TestRunToolLoopNormalizesToolCallFromFunctionPayload(t *testing.T) {
	argsJSON, err := json.Marshal(map[string]any{"city": "Shanghai"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	provider := &fakeProvider{
		responses: []*providers.LLMResponse{
			{
				ToolCalls: []providers.ToolCall{
					{
						ID:   "call-1",
						Type: "function",
						Function: &providers.FunctionCall{
							Name:      "weather",
							Arguments: string(argsJSON),
						},
					},
				},
			},
			{Content: "sunny"},
		},
	}

	registry := NewToolRegistry()
	tool := newMockTool("weather", "weather lookup")
	tool.result = SilentResult("weather=25C")
	registry.Register(tool)

	result, err := RunToolLoop(context.Background(), ToolLoopConfig{
		Provider:      provider,
		Model:         "test-model",
		Tools:         registry,
		MaxIterations: 3,
	}, []providers.Message{{Role: "user", Content: "weather?"}}, "discord", "chat-42")
	if err != nil {
		t.Fatalf("RunToolLoop returned error: %v", err)
	}
	if result.Content != "sunny" {
		t.Fatalf("expected final content sunny, got %q", result.Content)
	}
	if provider.messages[1][1].ToolCalls[0].Name != "weather" {
		t.Fatalf("expected normalized tool call name weather, got %q", provider.messages[1][1].ToolCalls[0].Name)
	}
	if provider.messages[1][1].ToolCalls[0].Arguments["city"] != "Shanghai" {
		t.Fatalf("expected normalized tool call args, got %#v", provider.messages[1][1].ToolCalls[0].Arguments)
	}
}

func TestRunToolLoopReturnsProviderError(t *testing.T) {
	provider := &fakeProvider{err: errors.New("network down")}

	_, err := RunToolLoop(context.Background(), ToolLoopConfig{
		Provider:      provider,
		Model:         "test-model",
		MaxIterations: 1,
	}, []providers.Message{{Role: "user", Content: "hi"}}, "discord", "chat-1")
	if err == nil {
		t.Fatal("expected provider error")
	}
}

func TestRunToolLoopReturnsTranscriptForDirectAnswer(t *testing.T) {
	provider := &fakeProvider{
		responses: []*providers.LLMResponse{
			{Content: "hello from model"},
		},
	}

	result, err := RunToolLoop(context.Background(), ToolLoopConfig{
		Provider:      provider,
		Model:         "test-model",
		MaxIterations: 2,
	}, []providers.Message{{Role: "user", Content: "hi"}}, "discord", "chat-1")
	if err != nil {
		t.Fatalf("RunToolLoop returned error: %v", err)
	}
	if result.Content != "hello from model" {
		t.Fatalf("expected direct answer, got %q", result.Content)
	}
}
