package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultConfigUsesClawcordHomeForWorkspace(t *testing.T) {
	t.Setenv("CLAWCORD_HOME", "/tmp/clawcord-home")

	cfg := DefaultConfig()

	want := filepath.Join("/tmp/clawcord-home", "workspace")
	if cfg.Agents.Defaults.Workspace != want {
		t.Fatalf("Workspace = %q, want %q", cfg.Agents.Defaults.Workspace, want)
	}
}

func TestDefaultConfigIncludesDefaultModelListEntry(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.GetModelName() != "main" {
		t.Fatalf("default model name = %q, want %q", cfg.Agents.Defaults.GetModelName(), "main")
	}
	if len(cfg.ModelList) != 1 {
		t.Fatalf("len(ModelList) = %d, want 1", len(cfg.ModelList))
	}
	if cfg.ModelList[0].ModelName != "main" {
		t.Fatalf("ModelList[0].ModelName = %q, want %q", cfg.ModelList[0].ModelName, "main")
	}
}

func TestDefaultConfigLeavesTemperatureUnset(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.Temperature != nil {
		t.Fatalf("Temperature = %v, want nil", *cfg.Agents.Defaults.Temperature)
	}
}

func TestDefaultConfigUsesPerChannelPeerSessionScope(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Session.DMScope != "per-channel-peer" {
		t.Fatalf("Session.DMScope = %q, want %q", cfg.Session.DMScope, "per-channel-peer")
	}
}
