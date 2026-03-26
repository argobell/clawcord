package agent

import (
	"context"
	"testing"

	"github.com/argobell/clawcord/pkg/providers"
	"github.com/argobell/clawcord/pkg/session"
	"github.com/argobell/clawcord/pkg/tools"
)

type recordingProvider struct {
	defaultModel string
	responses    []*providers.LLMResponse
	err          error
	errAtCall    int
	calls        int
	messages     [][]providers.Message
	options      []map[string]any
}

func (p *recordingProvider) Chat(
	_ context.Context,
	messages []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	p.calls++
	p.messages = append(p.messages, append([]providers.Message(nil), messages...))
	if options != nil {
		cloned := make(map[string]any, len(options))
		for k, v := range options {
			cloned[k] = v
		}
		p.options = append(p.options, cloned)
	} else {
		p.options = append(p.options, nil)
	}
	if p.err != nil && (p.errAtCall == 0 || p.calls == p.errAtCall) {
		return nil, p.err
	}
	if len(p.responses) == 0 {
		return &providers.LLMResponse{}, nil
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func (p *recordingProvider) GetDefaultModel() string {
	return p.defaultModel
}

type loopMockTool struct {
	name        string
	description string
	result      *tools.ToolResult
	lastChannel string
	lastChatID  string
}

func (t *loopMockTool) Name() string        { return t.name }
func (t *loopMockTool) Description() string { return t.description }
func (t *loopMockTool) Parameters() map[string]any {
	return map[string]any{"type": "object"}
}
func (t *loopMockTool) Execute(ctx context.Context, _ map[string]any) *tools.ToolResult {
	t.lastChannel = tools.ToolChannel(ctx)
	t.lastChatID = tools.ToolChatID(ctx)
	return t.result
}

func TestRunTurnReturnsDirectAnswerAndPersistsMessages(t *testing.T) {
	store := session.NewSessionManager("")
	provider := &recordingProvider{
		defaultModel: "gpt-5.4",
		responses: []*providers.LLMResponse{
			{Content: "pong"},
		},
	}

	instance, err := New(Config{
		Provider:     provider,
		SessionStore: store,
		SystemPrompt: "system prompt",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	result, err := instance.RunTurn(context.Background(), TurnInput{
		SessionKey:      "discord:1",
		Channel:         "discord",
		ChatID:          "chat-1",
		UserMessage:     "ping",
		DefaultResponse: "fallback",
	})
	if err != nil {
		t.Fatalf("RunTurn returned error: %v", err)
	}
	if result.Content != "pong" {
		t.Fatalf("expected final content pong, got %q", result.Content)
	}
	if result.Iterations != 1 {
		t.Fatalf("expected 1 iteration, got %d", result.Iterations)
	}

	history := store.GetHistory("discord:1")
	if len(history) != 2 {
		t.Fatalf("expected user and assistant in history, got %d messages", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "ping" {
		t.Fatalf("expected persisted user message first, got %#v", history[0])
	}
	if history[1].Role != "assistant" || history[1].Content != "pong" {
		t.Fatalf("expected persisted assistant message second, got %#v", history[1])
	}

	if provider.calls != 1 {
		t.Fatalf("expected provider to be called once, got %d", provider.calls)
	}
	if got := provider.options[0]["max_tokens"]; got != 8192 {
		t.Fatalf("expected max_tokens option 8192, got %#v", got)
	}
	if got := provider.options[0]["temperature"]; got != 0.7 {
		t.Fatalf("expected temperature option 0.7, got %#v", got)
	}
}

func TestRunTurnPersistsToolTranscriptInOrder(t *testing.T) {
	store := session.NewSessionManager("")
	provider := &recordingProvider{
		defaultModel: "gpt-5.4",
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

	registry := tools.NewToolRegistry()
	tool := &loopMockTool{
		name:        "weather",
		description: "weather lookup",
		result:      tools.SilentResult("weather=25C"),
	}
	registry.Register(tool)

	instance, err := New(Config{
		Provider:     provider,
		SessionStore: store,
		Tools:        registry,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	result, err := instance.RunTurn(context.Background(), TurnInput{
		SessionKey:      "discord:2",
		Channel:         "discord",
		ChatID:          "chat-2",
		UserMessage:     "weather?",
		DefaultResponse: "fallback",
	})
	if err != nil {
		t.Fatalf("RunTurn returned error: %v", err)
	}
	if result.Content != "sunny" {
		t.Fatalf("expected final content sunny, got %q", result.Content)
	}
	if result.Iterations != 2 {
		t.Fatalf("expected 2 iterations, got %d", result.Iterations)
	}

	history := store.GetHistory("discord:2")
	if len(history) != 4 {
		t.Fatalf("expected user, assistant tool-call, tool, assistant final; got %d messages", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "weather?" {
		t.Fatalf("unexpected first history message: %#v", history[0])
	}
	if history[1].Role != "assistant" || len(history[1].ToolCalls) != 1 {
		t.Fatalf("expected assistant tool-call message second, got %#v", history[1])
	}
	if history[2].Role != "tool" || history[2].ToolCallID != "call-1" || history[2].Content != "weather=25C" {
		t.Fatalf("expected tool result third, got %#v", history[2])
	}
	if history[3].Role != "assistant" || history[3].Content != "sunny" {
		t.Fatalf("expected final assistant message fourth, got %#v", history[3])
	}

	if tool.lastChannel != "discord" {
		t.Fatalf("expected tool channel context discord, got %q", tool.lastChannel)
	}
	if tool.lastChatID != "chat-2" {
		t.Fatalf("expected tool chatID context chat-2, got %q", tool.lastChatID)
	}
}

func TestRunTurnNoHistorySkipsExistingHistoryInProviderMessages(t *testing.T) {
	store := session.NewSessionManager("")
	store.AddMessage("discord:3", "user", "old user")
	store.AddMessage("discord:3", "assistant", "old assistant")
	store.SetSummary("discord:3", "old summary")

	provider := &recordingProvider{
		defaultModel: "gpt-5.4",
		responses: []*providers.LLMResponse{
			{Content: "fresh reply"},
		},
	}

	instance, err := New(Config{
		Provider:     provider,
		SessionStore: store,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = instance.RunTurn(context.Background(), TurnInput{
		SessionKey:  "discord:3",
		Channel:     "discord",
		ChatID:      "chat-3",
		UserMessage: "new message",
		NoHistory:   true,
	})
	if err != nil {
		t.Fatalf("RunTurn returned error: %v", err)
	}

	if provider.calls != 1 {
		t.Fatalf("expected provider to be called once, got %d", provider.calls)
	}
	if len(provider.messages[0]) != 2 {
		t.Fatalf("expected only system and current user messages, got %d", len(provider.messages[0]))
	}
	if provider.messages[0][1].Content != "new message" {
		t.Fatalf("expected current user message in provider input, got %#v", provider.messages[0][1])
	}
}

func TestRunTurnUsesDefaultResponseWhenProviderReturnsEmptyContent(t *testing.T) {
	store := session.NewSessionManager("")
	provider := &recordingProvider{
		defaultModel: "gpt-5.4",
		responses: []*providers.LLMResponse{
			{},
		},
	}

	instance, err := New(Config{
		Provider:     provider,
		SessionStore: store,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	result, err := instance.RunTurn(context.Background(), TurnInput{
		SessionKey:      "discord:4",
		UserMessage:     "hello",
		DefaultResponse: "fallback response",
	})
	if err != nil {
		t.Fatalf("RunTurn returned error: %v", err)
	}
	if result.Content != "fallback response" {
		t.Fatalf("expected fallback response, got %q", result.Content)
	}
}

func TestRunTurnPersistsToolTranscriptWhenFollowUpLLMCallFails(t *testing.T) {
	store := session.NewSessionManager("")
	provider := &recordingProvider{
		defaultModel: "gpt-5.4",
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
		},
		err:       context.DeadlineExceeded,
		errAtCall: 2,
	}

	registry := tools.NewToolRegistry()
	registry.Register(&loopMockTool{
		name:        "weather",
		description: "weather lookup",
		result:      tools.SilentResult("weather=25C"),
	})

	instance, err := New(Config{
		Provider:     provider,
		SessionStore: store,
		Tools:        registry,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = instance.RunTurn(context.Background(), TurnInput{
		SessionKey:  "discord:5",
		Channel:     "discord",
		ChatID:      "chat-5",
		UserMessage: "weather?",
	})
	if err == nil {
		t.Fatal("expected RunTurn to return error")
	}

	history := store.GetHistory("discord:5")
	if len(history) != 3 {
		t.Fatalf("expected user, assistant tool-call, and tool result in history, got %d messages", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "weather?" {
		t.Fatalf("unexpected first history message: %#v", history[0])
	}
	if history[1].Role != "assistant" || len(history[1].ToolCalls) != 1 {
		t.Fatalf("expected assistant tool-call message second, got %#v", history[1])
	}
	if history[2].Role != "tool" || history[2].ToolCallID != "call-1" || history[2].Content != "weather=25C" {
		t.Fatalf("expected tool result third, got %#v", history[2])
	}
}

func TestRunTurnSavesUserMessageWhenFirstLLMCallFails(t *testing.T) {
	store := &fakeSessionStore{}
	provider := &recordingProvider{
		defaultModel: "gpt-5.4",
		err:          context.DeadlineExceeded,
		errAtCall:    1,
	}

	instance, err := New(Config{
		Provider:     provider,
		SessionStore: store,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = instance.RunTurn(context.Background(), TurnInput{
		SessionKey:  "discord:6",
		UserMessage: "hello",
	})
	if err == nil {
		t.Fatal("expected RunTurn to return error")
	}
	if store.saveCalls != 1 {
		t.Fatalf("expected Save to be called once, got %d", store.saveCalls)
	}
}
