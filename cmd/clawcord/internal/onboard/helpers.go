package onboard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/argobell/clawcord/pkg/config"
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

	cfg := config.DefaultConfigForHome(filepath.Dir(configPath))
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0o600)
}
