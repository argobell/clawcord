package onboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOnboardCreatesConfigAndWorkspace(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".clawcord", "config.json")
	workspacePath := filepath.Join(home, ".clawcord", "workspace")

	if err := runOnboard(configPath, workspacePath); err != nil {
		t.Fatalf("runOnboard() error = %v", err)
	}

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config.json stat error = %v", err)
	}
	if info, err := os.Stat(workspacePath); err != nil {
		t.Fatalf("workspace stat error = %v", err)
	} else if !info.IsDir() {
		t.Fatalf("workspace is not a directory")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.json) error = %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("config.json is not valid JSON: %v", err)
	}

	agents, ok := raw["agents"].(map[string]any)
	if !ok {
		t.Fatal("config missing agents")
	}
	defaults, ok := agents["defaults"].(map[string]any)
	if !ok {
		t.Fatal("config missing agents.defaults")
	}
	if got := defaults["workspace"]; got != workspacePath {
		t.Fatalf("agents.defaults.workspace = %v, want %q", got, workspacePath)
	}
	if got := defaults["model_name"]; got != "main" {
		t.Fatalf("agents.defaults.model_name = %v, want %q", got, "main")
	}

	modelList, ok := raw["model_list"].([]any)
	if !ok || len(modelList) != 1 {
		t.Fatalf("model_list = %#v, want one entry", raw["model_list"])
	}

	session, ok := raw["session"].(map[string]any)
	if !ok {
		t.Fatal("config missing session")
	}
	if got := session["dm_scope"]; got != "per-channel-peer" {
		t.Fatalf("session.dm_scope = %v, want %q", got, "per-channel-peer")
	}
}

func TestOnboardDoesNotOverwriteExistingConfig(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".clawcord")
	configPath := filepath.Join(configDir, "config.json")
	workspacePath := filepath.Join(configDir, "workspace")

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	original := []byte(`{"keep":"me"}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := runOnboard(configPath, workspacePath); err != nil {
		t.Fatalf("runOnboard() error = %v", err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("config content = %q, want %q", string(got), string(original))
	}
}
