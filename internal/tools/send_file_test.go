package builtintools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/argobell/clawcord/pkg/media"
	pkgtools "github.com/argobell/clawcord/pkg/tools"
)

func TestSendFileToolMissingPath(t *testing.T) {
	store := media.NewFileMediaStore()
	tool := NewSendFileTool("/tmp", 0, store)

	result := tool.Execute(pkgtools.WithToolContext(context.Background(), "discord", "chat-1"), map[string]any{})
	if !result.IsError {
		t.Fatal("expected missing path error")
	}
}

func TestSendFileToolRequiresChannelContext(t *testing.T) {
	store := media.NewFileMediaStore()
	tool := NewSendFileTool("/tmp", 0, store)

	result := tool.Execute(context.Background(), map[string]any{"path": "/tmp/test.txt"})
	if !result.IsError {
		t.Fatal("expected missing channel/chat context error")
	}
}

func TestSendFileToolSuccess(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(testFile, []byte("fake png bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := media.NewFileMediaStore()
	tool := NewSendFileTool(dir, 0, store)
	result := tool.Execute(
		pkgtools.WithToolContext(context.Background(), "discord", "chat-1"),
		map[string]any{"path": testFile},
	)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if len(result.Media) != 1 {
		t.Fatalf("expected 1 media ref, got %d", len(result.Media))
	}
	if !result.ResponseHandled {
		t.Fatal("expected send_file to mark response handled")
	}

	_, meta, err := store.ResolveWithMeta(result.Media[0])
	if err != nil {
		t.Fatalf("ResolveWithMeta() error = %v", err)
	}
	if meta.Filename != "photo.png" {
		t.Fatalf("resolved filename = %q, want photo.png", meta.Filename)
	}
	if meta.Source != "tool:send_file" {
		t.Fatalf("resolved source = %q, want tool:send_file", meta.Source)
	}
}

func TestSendFileToolDetectsMimeFromContent(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "image.bin")
	pngHeader := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	}
	if err := os.WriteFile(testFile, pngHeader, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := media.NewFileMediaStore()
	tool := NewSendFileTool(dir, 0, store)
	result := tool.Execute(
		pkgtools.WithToolContext(context.Background(), "discord", "chat-1"),
		map[string]any{"path": testFile},
	)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	_, meta, err := store.ResolveWithMeta(result.Media[0])
	if err != nil {
		t.Fatalf("ResolveWithMeta() error = %v", err)
	}
	if meta.ContentType != "image/png" {
		t.Fatalf("resolved content type = %q, want image/png", meta.ContentType)
	}
}
