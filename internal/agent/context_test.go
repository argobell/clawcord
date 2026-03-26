package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/argobell/clawcord/pkg/providers"
)

func TestBuildSystemPromptIncludesWorkspaceSystemPromptAndBootstrapFiles(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, filepath.Join(workspace, "AGENTS.md"), "# Agent Rules")
	writeTestFile(t, filepath.Join(workspace, "USER.md"), "user preferences")

	builder := NewContextBuilder(workspace, "custom system prompt")

	prompt := builder.BuildSystemPrompt()

	if !strings.Contains(prompt, "clawcord") {
		t.Fatalf("expected prompt to include clawcord identity, got %q", prompt)
	}
	if !strings.Contains(prompt, workspace) {
		t.Fatalf("expected prompt to include workspace path, got %q", prompt)
	}
	if !strings.Contains(prompt, "custom system prompt") {
		t.Fatalf("expected prompt to include supplied system prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "## AGENTS.md") {
		t.Fatalf("expected prompt to include AGENTS.md content, got %q", prompt)
	}
	if !strings.Contains(prompt, "## USER.md") {
		t.Fatalf("expected prompt to include USER.md content, got %q", prompt)
	}
}

func TestBuildSystemPromptIgnoresMissingBootstrapFiles(t *testing.T) {
	builder := NewContextBuilder(t.TempDir(), "")

	prompt := builder.BuildSystemPrompt()

	if !strings.Contains(prompt, "clawcord") {
		t.Fatalf("expected prompt to include identity even without bootstrap files, got %q", prompt)
	}
}

func TestBuildSystemPromptWithCacheUsesCachedValueUntilInvalidated(t *testing.T) {
	workspace := t.TempDir()
	agentsPath := filepath.Join(workspace, "AGENTS.md")
	writeTestFile(t, agentsPath, "first version")

	builder := NewContextBuilder(workspace, "")

	first := builder.BuildSystemPromptWithCache()
	writeTestFile(t, agentsPath, "second version")
	cached := builder.BuildSystemPromptWithCache()
	if cached != first {
		t.Fatalf("expected cached prompt before invalidation")
	}

	builder.InvalidateCache()
	updated := builder.BuildSystemPromptWithCache()
	if updated == first {
		t.Fatalf("expected prompt to change after cache invalidation")
	}
	if !strings.Contains(updated, "second version") {
		t.Fatalf("expected invalidated prompt to include updated bootstrap content, got %q", updated)
	}
}

func TestBuildMessagesBuildsSingleSystemMessageHistoryAndCurrentUser(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, filepath.Join(workspace, "AGENTS.md"), "agent bootstrap")
	builder := NewContextBuilder(workspace, "system prompt")

	history := []providers.Message{
		{Role: "system", Content: "old system"},
		{Role: "user", Content: "earlier user"},
		{Role: "assistant", Content: "earlier assistant"},
	}

	messages := builder.BuildMessages(
		history,
		"summary text",
		"current user",
		[]string{"image://1"},
		"discord",
		"chat-1",
	)

	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}
	if messages[0].Role != "system" {
		t.Fatalf("expected first message to be system, got %q", messages[0].Role)
	}
	if len(messages[0].SystemParts) != 3 {
		t.Fatalf("expected system message to include static, dynamic, and summary parts, got %d", len(messages[0].SystemParts))
	}
	if !strings.Contains(messages[0].Content, "summary text") {
		t.Fatalf("expected system content to include summary, got %q", messages[0].Content)
	}
	if !strings.Contains(messages[0].Content, "Channel: discord") {
		t.Fatalf("expected system content to include dynamic session context, got %q", messages[0].Content)
	}
	if messages[1].Role != "user" || messages[1].Content != "earlier user" {
		t.Fatalf("expected sanitized history user message in slot 1, got %#v", messages[1])
	}
	if messages[2].Role != "assistant" || messages[2].Content != "earlier assistant" {
		t.Fatalf("expected sanitized history assistant message in slot 2, got %#v", messages[2])
	}
	if messages[3].Role != "user" || messages[3].Content != "current user" {
		t.Fatalf("expected current user message last, got %#v", messages[3])
	}
	if len(messages[3].Media) != 1 || messages[3].Media[0] != "image://1" {
		t.Fatalf("expected current user media to be preserved, got %#v", messages[3].Media)
	}
}

func TestBuildMessagesSkipsEmptyCurrentMessage(t *testing.T) {
	builder := NewContextBuilder(t.TempDir(), "")

	messages := builder.BuildMessages(
		[]providers.Message{{Role: "user", Content: "history"}},
		"",
		"   ",
		nil,
		"",
		"",
	)

	if len(messages) != 2 {
		t.Fatalf("expected only system plus history, got %d messages", len(messages))
	}
}

func TestSanitizeHistoryForProviderDropsInvalidMessages(t *testing.T) {
	history := []providers.Message{
		{Role: "system", Content: "drop me"},
		{Role: "tool", Content: "orphan tool", ToolCallID: "orphan"},
		{
			Role:    "assistant",
			Content: "call tool",
			ToolCalls: []providers.ToolCall{
				{ID: "call-1", Name: "echo"},
			},
		},
		{Role: "tool", Content: "partial result", ToolCallID: "other-call"},
		{Role: "user", Content: "keep me"},
		{
			Role:    "assistant",
			Content: "valid tool call",
			ToolCalls: []providers.ToolCall{
				{ID: "call-2", Name: "echo"},
			},
		},
		{Role: "tool", Content: "valid result", ToolCallID: "call-2"},
	}

	sanitized := sanitizeHistoryForProvider(history)

	if len(sanitized) != 3 {
		t.Fatalf("expected 3 sanitized messages, got %d", len(sanitized))
	}
	if sanitized[0].Role != "user" {
		t.Fatalf("expected first sanitized message to be user, got %q", sanitized[0].Role)
	}
	if sanitized[1].Role != "assistant" || len(sanitized[1].ToolCalls) != 1 {
		t.Fatalf("expected valid assistant tool-call message to remain, got %#v", sanitized[1])
	}
	if sanitized[2].Role != "tool" || sanitized[2].ToolCallID != "call-2" {
		t.Fatalf("expected valid tool result to remain, got %#v", sanitized[2])
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
