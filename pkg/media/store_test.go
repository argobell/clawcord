package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileMediaStoreStoreResolveAndRelease(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := NewFileMediaStore()
	ref, err := store.Store(filePath, MediaMeta{
		Filename:    "hello.txt",
		ContentType: "text/plain",
		Source:      "test",
	}, "scope-1")
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if !strings.HasPrefix(ref, "media://") {
		t.Fatalf("ref = %q, want media://...", ref)
	}

	resolvedPath, err := store.Resolve(ref)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolvedPath != filePath {
		t.Fatalf("Resolve() = %q, want %q", resolvedPath, filePath)
	}

	resolvedPath, meta, err := store.ResolveWithMeta(ref)
	if err != nil {
		t.Fatalf("ResolveWithMeta() error = %v", err)
	}
	if resolvedPath != filePath {
		t.Fatalf("ResolveWithMeta() path = %q, want %q", resolvedPath, filePath)
	}
	if meta.Filename != "hello.txt" {
		t.Fatalf("ResolveWithMeta() filename = %q, want hello.txt", meta.Filename)
	}

	if err := store.ReleaseAll("scope-1"); err != nil {
		t.Fatalf("ReleaseAll() error = %v", err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("file still exists after ReleaseAll(), err = %v", err)
	}
	if _, err := store.Resolve(ref); err == nil {
		t.Fatal("Resolve() expected error after release")
	}
}
