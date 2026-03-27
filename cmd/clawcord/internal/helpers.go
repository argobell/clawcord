package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/argobell/clawcord/pkg/config"
)

// GetClawcordHome returns the clawcord home directory.
// Uses CLAWCORD_HOME env var if set, otherwise ~/.clawcord.
func GetClawcordHome() string {
	if home := os.Getenv("CLAWCORD_HOME"); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".clawcord"
	}
	return filepath.Join(home, ".clawcord")
}

// GetConfigPath returns the path to the config file.
// Uses CLAWCORD_CONFIG env var if set, otherwise <clawcord_home>/config.json.
func GetConfigPath() string {
	if path := os.Getenv("CLAWCORD_CONFIG"); path != "" {
		return path
	}
	return filepath.Join(GetClawcordHome(), "config.json")
}

// LoadConfig loads the config from the default config path.
func LoadConfig() (*config.Config, error) {
	path := GetConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file at %s: %w", path, err)
	}

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}
