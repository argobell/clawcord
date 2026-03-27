package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetClawcordHome(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		envSet   bool
		wantSub  string
	}{
		{
			name:     "uses CLAWCORD_HOME when set",
			envValue: "/custom/clawcord/path",
			envSet:   true,
			wantSub:  "/custom/clawcord/path",
		},
		{
			name:    "uses default when CLAWCORD_HOME not set",
			envSet:  false,
			wantSub: ".clawcord",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envSet {
				t.Setenv("CLAWCORD_HOME", tt.envValue)
			} else {
				os.Unsetenv("CLAWCORD_HOME")
			}

			got := GetClawcordHome()
			if tt.envSet && got != tt.wantSub {
				t.Errorf("GetClawcordHome() = %v, want %v", got, tt.wantSub)
			}
			if !tt.envSet && !contains(got, tt.wantSub) {
				t.Errorf("GetClawcordHome() = %v, should contain %v", got, tt.wantSub)
			}
		})
	}
}

func TestGetConfigPath(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		envSet   bool
	}{
		{
			name:     "uses CLAWCORD_CONFIG when set",
			envValue: "/custom/config.json",
			envSet:   true,
		},
		{
			name:   "uses default when CLAWCORD_CONFIG not set",
			envSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envSet {
				t.Setenv("CLAWCORD_CONFIG", tt.envValue)
			} else {
				os.Unsetenv("CLAWCORD_CONFIG")
			}

			got := GetConfigPath()
			if tt.envSet && got != tt.envValue {
				t.Errorf("GetConfigPath() = %v, want %v", got, tt.envValue)
			}
			if !tt.envSet && !contains(got, "config.json") {
				t.Errorf("GetConfigPath() = %v, should contain config.json", got)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	t.Run("loads valid config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		configData := `{
			"agents": {
				"defaults": {
					"model_name": "test-model"
				}
			},
			"model_list": [
				{
					"model_name": "test-model",
					"model": "gpt-4"
				}
			]
		}`

		if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
			t.Fatalf("Failed to write test config: %v", err)
		}

		t.Setenv("CLAWCORD_CONFIG", configPath)

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}

		if cfg.Agents.Defaults.ModelName != "test-model" {
			t.Errorf("ModelName = %v, want test-model", cfg.Agents.Defaults.ModelName)
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "nonexistent.json")
		t.Setenv("CLAWCORD_CONFIG", configPath)

		_, err := LoadConfig()
		if err == nil {
			t.Error("LoadConfig() expected error for missing file")
		}
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		if err := os.WriteFile(configPath, []byte("invalid json"), 0644); err != nil {
			t.Fatalf("Failed to write test config: %v", err)
		}

		t.Setenv("CLAWCORD_CONFIG", configPath)

		_, err := LoadConfig()
		if err == nil {
			t.Error("LoadConfig() expected error for invalid JSON")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
