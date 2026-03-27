package config

import (
	"os"
	"path/filepath"
)

// DefaultConfig returns the default clawcord configuration.
func DefaultConfig() Config {
	homePath := os.Getenv("CLAWCORD_HOME")
	if homePath == "" {
		userHome, _ := os.UserHomeDir()
		homePath = filepath.Join(userHome, ".clawcord")
	}
	return DefaultConfigForHome(homePath)
}

// DefaultConfigForHome returns the default config rooted at the given home dir.
func DefaultConfigForHome(homePath string) Config {
	workspacePath := filepath.Join(homePath, "workspace")

	return Config{
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Workspace:         workspacePath,
				ModelName:         "main",
				MaxTokens:         8192,
				Temperature:       nil,
				MaxToolIterations: 20,
			},
			List: []AgentConfig{
				{
					ID:   "main",
					Name: "Main Agent",
				},
			},
		},
		ModelList: []ModelConfig{
			{
				ModelName: "main",
				Model:     "openai/gpt-5.4",
				APIBase:   "https://api.openai.com/v1",
				APIKey:    "YOUR_API_KEY",
			},
		},
		Channels: ChannelsConfig{
			Discord: DiscordConfig{
				Enabled:        false,
				Token:          "YOUR_DISCORD_BOT_TOKEN",
				MessageContent: true,
				Typing: TypingConfig{
					Enabled: true,
				},
				Placeholder: PlaceholderConfig{
					Enabled: true,
					Text:    "Thinking... 💭",
				},
			},
		},
		Session: SessionConfig{
			DMScope: "per-channel-peer",
		},
	}
}
