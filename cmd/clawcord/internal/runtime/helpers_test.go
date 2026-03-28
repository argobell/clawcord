package runtime

import (
	"testing"

	"github.com/argobell/clawcord/pkg/config"
)

func TestResolveModelName(t *testing.T) {
	tests := []struct {
		name      string
		agentCfg   config.AgentConfig
		defaults  config.AgentDefaults
		override  string
		want      string
	}{
		{
			name:     "uses flag override first",
			agentCfg: config.AgentConfig{Model: "agent-model"},
			defaults: config.AgentDefaults{ModelName: "default-model"},
			override: "flag-model",
			want:     "flag-model",
		},
		{
			name:     "uses agent model before defaults",
			agentCfg: config.AgentConfig{Model: "agent-model"},
			defaults: config.AgentDefaults{ModelName: "default-model"},
			want:     "agent-model",
		},
		{
			name:     "falls back to defaults",
			defaults: config.AgentDefaults{ModelName: "default-model"},
			want:     "default-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveModelName(tt.agentCfg, tt.defaults, tt.override); got != tt.want {
				t.Fatalf("ResolveModelName() = %q, want %q", got, tt.want)
			}
		})
	}
}
