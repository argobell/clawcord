package config

import (
	"encoding/json"
	"testing"
)

func TestFlexibleStringSliceUnmarshalJSONAcceptsNumbers(t *testing.T) {
	var got FlexibleStringSlice
	if err := json.Unmarshal([]byte(`["123", 456, "789"]`), &got); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	want := []string{"123", "456", "789"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFlexibleStringSliceUnmarshalTextAcceptsChineseComma(t *testing.T) {
	var got FlexibleStringSlice
	if err := got.UnmarshalText([]byte("123，456, 789")); err != nil {
		t.Fatalf("UnmarshalText returned error: %v", err)
	}

	want := []string{"123", "456", "789"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDiscordConfigJSONShape(t *testing.T) {
	temp := 0.0
	cfg := Config{
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Workspace:         "/tmp/clawcord",
				ModelName:         "gpt-5.4",
				MaxTokens:         4096,
				Temperature:       &temp,
				MaxToolIterations: 12,
			},
			List: []AgentConfig{
				{ID: "main", Name: "Main", Workspace: "/tmp/clawcord", Model: "gpt-5.4"},
			},
		},
		Channels: ChannelsConfig{
			Discord: DiscordConfig{
				Enabled:            true,
				Token:              "token",
				Proxy:              "http://127.0.0.1:7890",
				AllowFrom:          FlexibleStringSlice{"123456", "discord:789"},
				MentionOnly:        true,
				GroupTrigger:       GroupTriggerConfig{MentionOnly: true, Prefixes: []string{"/ask"}},
				Typing:             TypingConfig{Enabled: true},
				Placeholder:        PlaceholderConfig{Enabled: true, Text: "Thinking..."},
				ReasoningChannelID: "reasoning-1",
				MessageContent:     true,
			},
		},
		Session: SessionConfig{
			DMScope:       "peer",
			IdentityLinks: map[string][]string{"discord:123": {"telegram:456"}},
		},
		ModelList: []ModelConfig{
			{
				ModelName:      "gpt-5.4",
				Model:          "openai/gpt-5.4",
				APIBase:        "https://api.openai.com/v1",
				APIKey:         "test-key",
				Proxy:          "http://127.0.0.1:7890",
				RequestTimeout: 30,
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var roundTrip Config
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if roundTrip.Agents.Defaults.Temperature == nil {
		t.Fatal("Temperature is nil after round trip")
	}
	if *roundTrip.Agents.Defaults.Temperature != 0 {
		t.Fatalf("Temperature = %v, want 0", *roundTrip.Agents.Defaults.Temperature)
	}
	if !roundTrip.Channels.Discord.MessageContent {
		t.Fatal("MessageContent was not preserved")
	}
	if !roundTrip.Channels.Discord.MentionOnly {
		t.Fatal("MentionOnly was not preserved")
	}
	if !roundTrip.Channels.Discord.GroupTrigger.MentionOnly {
		t.Fatal("GroupTrigger.MentionOnly was not preserved")
	}
	if len(roundTrip.Channels.Discord.GroupTrigger.Prefixes) != 1 || roundTrip.Channels.Discord.GroupTrigger.Prefixes[0] != "/ask" {
		t.Fatalf("GroupTrigger.Prefixes = %#v, want []string{\"/ask\"}", roundTrip.Channels.Discord.GroupTrigger.Prefixes)
	}
	if roundTrip.Session.DMScope != "peer" {
		t.Fatalf("DMScope = %q, want %q", roundTrip.Session.DMScope, "peer")
	}
	if got := roundTrip.Session.IdentityLinks["discord:123"][0]; got != "telegram:456" {
		t.Fatalf("IdentityLinks[discord:123][0] = %q, want %q", got, "telegram:456")
	}
	if len(roundTrip.ModelList) != 1 {
		t.Fatalf("len(ModelList) = %d, want 1", len(roundTrip.ModelList))
	}
	if roundTrip.ModelList[0].ModelName != "gpt-5.4" {
		t.Fatalf("ModelList[0].ModelName = %q, want %q", roundTrip.ModelList[0].ModelName, "gpt-5.4")
	}
}

func TestAgentDefaultsGetModelName(t *testing.T) {
	tests := []struct {
		name string
		in   AgentDefaults
		want string
	}{
		{
			name: "prefer model_name",
			in: AgentDefaults{
				ModelName: "gpt-5.4",
				Model:     "legacy-model",
			},
			want: "gpt-5.4",
		},
		{
			name: "fallback to legacy model",
			in: AgentDefaults{
				Model: "legacy-model",
			},
			want: "legacy-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.GetModelName(); got != tt.want {
				t.Fatalf("GetModelName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestModelConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ModelConfig
		wantErr bool
	}{
		{
			name: "valid",
			cfg: ModelConfig{
				ModelName: "gpt-5.4",
				Model:     "openai/gpt-5.4",
			},
			wantErr: false,
		},
		{
			name: "missing model name",
			cfg: ModelConfig{
				Model: "openai/gpt-5.4",
			},
			wantErr: true,
		},
		{
			name: "missing model",
			cfg: ModelConfig{
				ModelName: "gpt-5.4",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestConfigGetModelConfig(t *testing.T) {
	cfg := &Config{
		ModelList: []ModelConfig{
			{ModelName: "fast", Model: "openai/gpt-5.4-mini"},
			{ModelName: "main", Model: "openai/gpt-5.4"},
		},
	}

	got, err := cfg.GetModelConfig("main")
	if err != nil {
		t.Fatalf("GetModelConfig() error = %v", err)
	}
	if got.Model != "openai/gpt-5.4" {
		t.Fatalf("Model = %q, want %q", got.Model, "openai/gpt-5.4")
	}
}

func TestConfigGetModelConfigReturnsErrorForMissingModel(t *testing.T) {
	cfg := &Config{
		ModelList: []ModelConfig{
			{ModelName: "main", Model: "openai/gpt-5.4"},
		},
	}

	if _, err := cfg.GetModelConfig("missing"); err == nil {
		t.Fatal("GetModelConfig() error = nil, want error")
	}
}
