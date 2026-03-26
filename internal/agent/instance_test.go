package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/argobell/clawcord/pkg/providers"
	"github.com/argobell/clawcord/pkg/session"
	"github.com/argobell/clawcord/pkg/tools"
)

type fakeProvider struct {
	defaultModel string
}

func (f *fakeProvider) Chat(
	_ context.Context,
	_ []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{}, nil
}

func (f *fakeProvider) GetDefaultModel() string {
	return f.defaultModel
}

type fakeSessionStore struct {
	closeCalls int
}

func (f *fakeSessionStore) AddMessage(_, _, _ string)                    {}
func (f *fakeSessionStore) AddFullMessage(_ string, _ providers.Message) {}
func (f *fakeSessionStore) GetHistory(_ string) []providers.Message      { return nil }
func (f *fakeSessionStore) GetSummary(_ string) string                   { return "" }
func (f *fakeSessionStore) SetSummary(_, _ string)                       {}
func (f *fakeSessionStore) SetHistory(_ string, _ []providers.Message) {
}
func (f *fakeSessionStore) TruncateHistory(_ string, _ int) {}
func (f *fakeSessionStore) Save(_ string) error             { return nil }
func (f *fakeSessionStore) Close() error {
	f.closeCalls++
	return nil
}

func TestNewInstanceUsesExplicitConfig(t *testing.T) {
	workspace := t.TempDir()
	provider := &fakeProvider{defaultModel: "ignored-default"}
	sessions := &fakeSessionStore{}
	registry := tools.NewToolRegistry()

	instance, err := New(Config{
		Provider:      provider,
		ID:            "discord-main",
		Name:          "Discord Main Agent",
		Model:         "gpt-5.4",
		Workspace:     workspace,
		SystemPrompt:  "system prompt",
		SessionStore:  sessions,
		Tools:         registry,
		MaxIterations: 7,
		MaxTokens:     2048,
		Temperature:   0.2,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if instance.Provider != provider {
		t.Fatalf("expected provider to be preserved")
	}
	if instance.ID != "discord-main" {
		t.Fatalf("expected explicit ID, got %q", instance.ID)
	}
	if instance.Name != "Discord Main Agent" {
		t.Fatalf("expected explicit Name, got %q", instance.Name)
	}
	if instance.Sessions != sessions {
		t.Fatalf("expected session store to be preserved")
	}
	if instance.Tools != registry {
		t.Fatalf("expected tool registry to be preserved")
	}
	if instance.Model != "gpt-5.4" {
		t.Fatalf("expected explicit model, got %q", instance.Model)
	}
	if instance.Workspace != workspace {
		t.Fatalf("expected explicit workspace, got %q", instance.Workspace)
	}
	if instance.MaxIterations != 7 {
		t.Fatalf("expected MaxIterations=7, got %d", instance.MaxIterations)
	}
	if instance.MaxTokens != 2048 {
		t.Fatalf("expected MaxTokens=2048, got %d", instance.MaxTokens)
	}
	if instance.Temperature != 0.2 {
		t.Fatalf("expected Temperature=0.2, got %v", instance.Temperature)
	}
	if instance.ContextBuilder == nil {
		t.Fatal("expected ContextBuilder to be initialized")
	}
	prompt := instance.ContextBuilder.BuildSystemPrompt()
	if !strings.Contains(prompt, "system prompt") {
		t.Fatalf("expected context builder to include system prompt, got %q", prompt)
	}
}

func TestNewInstanceAppliesDefaults(t *testing.T) {
	provider := &fakeProvider{defaultModel: "gpt-5.4-mini"}

	instance, err := New(Config{Provider: provider})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd failed: %v", err)
	}
	if instance.Model != "gpt-5.4-mini" {
		t.Fatalf("expected provider default model, got %q", instance.Model)
	}
	if instance.ID != "main" {
		t.Fatalf("expected default ID \"main\", got %q", instance.ID)
	}
	if instance.Name != "" {
		t.Fatalf("expected default Name to be empty, got %q", instance.Name)
	}
	if instance.Workspace != wd {
		t.Fatalf("expected default workspace %q, got %q", wd, instance.Workspace)
	}
	if instance.MaxIterations != 20 {
		t.Fatalf("expected default MaxIterations=20, got %d", instance.MaxIterations)
	}
	if instance.MaxTokens != 8192 {
		t.Fatalf("expected default MaxTokens=8192, got %d", instance.MaxTokens)
	}
	if instance.Temperature != 0.7 {
		t.Fatalf("expected default Temperature=0.7, got %v", instance.Temperature)
	}
	if instance.ContextBuilder == nil {
		t.Fatal("expected ContextBuilder to be initialized")
	}
	if _, ok := instance.Sessions.(*session.SessionManager); !ok {
		t.Fatalf("expected default session manager, got %T", instance.Sessions)
	}
	if instance.Tools == nil {
		t.Fatal("expected default tool registry")
	}
	if instance.Tools.Count() != 0 {
		t.Fatalf("expected empty default tool registry, got %d tools", instance.Tools.Count())
	}
}

func TestNewInstanceFailsWithoutProvider(t *testing.T) {
	_, err := New(Config{Model: "gpt-5.4"})
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestNewInstanceFailsWithoutAnyModel(t *testing.T) {
	_, err := New(Config{Provider: &fakeProvider{}})
	if err == nil {
		t.Fatal("expected error when both explicit and provider default models are empty")
	}
}

func TestNewInstanceExpandsHomeInWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	instance, err := New(Config{
		Provider:  &fakeProvider{defaultModel: "gpt-5.4"},
		Workspace: "~/clawcord-workspace",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	expected := filepath.Join(home, "clawcord-workspace")
	if instance.Workspace != expected {
		t.Fatalf("expected expanded workspace %q, got %q", expected, instance.Workspace)
	}
	prompt := instance.ContextBuilder.BuildSystemPrompt()
	if !strings.Contains(prompt, expected) {
		t.Fatalf("expected context builder to use expanded workspace, got %q", prompt)
	}
}

func TestInstanceCloseDelegatesToSessionStore(t *testing.T) {
	sessions := &fakeSessionStore{}
	instance, err := New(Config{
		Provider:     &fakeProvider{defaultModel: "gpt-5.4"},
		SessionStore: sessions,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := instance.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if sessions.closeCalls != 1 {
		t.Fatalf("expected Close to delegate once, got %d", sessions.closeCalls)
	}
}
