package agent

import (
	"path/filepath"
	"testing"

	"github.com/argobell/clawcord/cmd/clawcord/internal/runtime"
	"github.com/argobell/clawcord/pkg/config"
)

func TestNewAgentCommand(t *testing.T) {
	cmd := NewAgentCommand()

	if cmd.Use != "agent" {
		t.Errorf("Use = %v, want agent", cmd.Use)
	}

	if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "a" {
		t.Errorf("Aliases = %v, want [a]", cmd.Aliases)
	}

	if cmd.Short == "" {
		t.Error("Short description should not be empty")
	}

	// Check flags
	flags := []struct {
		name      string
		shorthand string
	}{
		{"message", "m"},
		{"session", "s"},
		{"model", ""},
		{"debug", ""},
	}

	for _, f := range flags {
		flag := cmd.Flags().Lookup(f.name)
		if flag == nil {
			t.Errorf("Flag %q not found", f.name)
			continue
		}
		if f.shorthand != "" && flag.Shorthand != f.shorthand {
			t.Errorf("Flag %q shorthand = %v, want %v", f.name, flag.Shorthand, f.shorthand)
		}
	}
}

func TestResolveDefaultAgent(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{
			name: "uses first agent if id is main",
			cfg: config.Config{
				Agents: config.AgentsConfig{
					List: []config.AgentConfig{
						{ID: "main", Name: "Main Agent"},
					},
				},
			},
			want: "main",
		},
		{
			name: "synthesizes default if no agents",
			cfg:  config.Config{},
			want: "main",
		},
		{
			name: "uses main agent even if it is not first",
			cfg: config.Config{
				Agents: config.AgentsConfig{
					List: []config.AgentConfig{
						{ID: "other", Name: "Other Agent"},
						{ID: "main", Name: "Main Agent"},
					},
				},
			},
			want: "main",
		},
		{
			name: "falls back to first configured agent when main is absent",
			cfg: config.Config{
				Agents: config.AgentsConfig{
					List: []config.AgentConfig{
						{ID: "other", Name: "Other Agent"},
					},
				},
			},
			want: "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runtime.ResolveDefaultAgent(&tt.cfg)
			if got.ID != tt.want {
				t.Errorf("ResolveDefaultAgent() = %v, want %v", got.ID, tt.want)
			}
		})
	}
}

func TestSessionStoragePath(t *testing.T) {
	workspace := filepath.Join("/tmp", "clawcord-workspace")
	got := runtime.SessionStoragePath(workspace)
	want := filepath.Join(workspace, "sessions")
	if got != want {
		t.Fatalf("SessionStoragePath() = %q, want %q", got, want)
	}
}
