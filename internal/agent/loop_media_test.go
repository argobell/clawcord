package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/argobell/clawcord/pkg/media"
	"github.com/argobell/clawcord/pkg/providers"
)

func TestResolveMediaRefs_EncodesStoredImagesAsDataURLs(t *testing.T) {
	store := media.NewFileMediaStore()
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	if err := os.WriteFile(path, []byte("fake png bytes"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ref, err := store.Store(path, media.MediaMeta{
		Filename:    "image.png",
		ContentType: "image/png",
		Source:      "test",
	}, "scope-1")
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	messages := []providers.Message{
		{Role: "user", Content: "describe this image", Media: []string{ref}},
	}

	resolved := resolveMediaRefs(messages, store, 1024*1024)
	if len(resolved) != 1 {
		t.Fatalf("resolved message count = %d, want 1", len(resolved))
	}
	if len(resolved[0].Media) != 1 {
		t.Fatalf("resolved media count = %d, want 1", len(resolved[0].Media))
	}
	if !strings.HasPrefix(resolved[0].Media[0], "data:image/png;base64,") {
		t.Fatalf("resolved media = %q, want data:image/png base64 URL", resolved[0].Media[0])
	}
	if messages[0].Media[0] != ref {
		t.Fatalf("original message mutated, got %q want %q", messages[0].Media[0], ref)
	}
}
