package onboard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	projectconfig "github.com/argobell/clawcord/pkg/config"
)

func onboard() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	rootDir := filepath.Join(home, ".clawcord")
	configPath := filepath.Join(rootDir, "config.json")
	workspacePath := filepath.Join(rootDir, "workspace")

	if err := runOnboard(configPath, workspacePath); err != nil {
		return err
	}

	fmt.Printf("clawcord is ready\n\n")
	fmt.Printf("Config:    %s\n", configPath)
	fmt.Printf("Workspace: %s\n", workspacePath)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. Edit %s\n", configPath)
	fmt.Printf("  2. Add your API key to model_list\n")
	fmt.Printf("  3. Run clawcord agent or clawcord gateway\n")
	return nil
}

func runOnboard(configPath, workspacePath string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		return err
	}

	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	cfg := defaultConfig(workspacePath)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0o600)
}

func defaultConfig(workspacePath string) projectconfig.Config {
	temperature := 0.0
	return projectconfig.Config{
		Agents: projectconfig.AgentsConfig{
			Defaults: projectconfig.AgentDefaults{
				Workspace:         workspacePath,
				ModelName:         "main",
				MaxTokens:         8192,
				Temperature:       &temperature,
				MaxToolIterations: 20,
			},
			List: []projectconfig.AgentConfig{
				{
					ID:   "main",
					Name: "Main Agent",
				},
			},
		},
		ModelList: []projectconfig.ModelConfig{
			{
				ModelName: "main",
				Model:     "openai/gpt-5.4",
				APIBase:   "https://api.openai.com/v1",
				APIKey:    "YOUR_API_KEY",
			},
		},
		Channels: projectconfig.ChannelsConfig{
			Discord: projectconfig.DiscordConfig{
				Enabled:        false,
				Token:          "YOUR_DISCORD_BOT_TOKEN",
				MessageContent: true,
				Typing: projectconfig.TypingConfig{
					Enabled: true,
				},
				Placeholder: projectconfig.PlaceholderConfig{
					Enabled: true,
					Text:    "Thinking... 💭",
				},
			},
		},
		Session: projectconfig.SessionConfig{
			DMScope: "per-peer",
		},
	}
}
