package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/argobell/clawcord/pkg/providers"
	"github.com/argobell/clawcord/pkg/session"
	"github.com/argobell/clawcord/pkg/tools"
)

// Config 描述构造最小 agent instance 所需的依赖和运行时参数。
type Config struct {
	Provider      providers.LLMProvider
	ID            string
	Name          string
	Model         string
	Workspace     string
	SystemPrompt  string
	SessionStore  session.SessionStore
	Tools         *tools.ToolRegistry
	MaxIterations int
	MaxTokens     int
	Temperature   *float64
}

// AgentInstance 持有未来 loop 运行所需的核心依赖和默认值。
type AgentInstance struct {
	ID            string
	Name          string
	Model         string
	Workspace     string
	MaxIterations int
	MaxTokens     int
	Temperature   float64

	Provider       providers.LLMProvider
	Sessions       session.SessionStore
	ContextBuilder *ContextBuilder
	Tools          *tools.ToolRegistry
}

// New 创建最小可用的 agent instance。
func New(cfg Config) (*AgentInstance, error) {
	if cfg.Provider == nil {
		return nil, fmt.Errorf("provider is required")
	}

	model := resolveModel(cfg)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	workspace, err := resolveWorkspace(cfg.Workspace)
	if err != nil {
		return nil, err
	}

	sessions := cfg.SessionStore
	if sessions == nil {
		sessions = session.NewSessionManager("")
	}

	registry := cfg.Tools
	if registry == nil {
		registry = tools.NewToolRegistry()
	}

	maxIterations := cfg.MaxIterations
	if maxIterations == 0 {
		maxIterations = 20
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	temperature := 0.7
	if cfg.Temperature != nil {
		temperature = *cfg.Temperature
	} else {
		temperature = 0.7
	}

	id := strings.TrimSpace(cfg.ID)
	if id == "" {
		id = "main"
	}

	return &AgentInstance{
		ID:             id,
		Name:           strings.TrimSpace(cfg.Name),
		Model:          model,
		Workspace:      workspace,
		MaxIterations:  maxIterations,
		MaxTokens:      maxTokens,
		Temperature:    temperature,
		Provider:       cfg.Provider,
		Sessions:       sessions,
		ContextBuilder: NewContextBuilder(workspace, cfg.SystemPrompt),
		Tools:          registry,
	}, nil
}

func resolveModel(cfg Config) string {
	if model := strings.TrimSpace(cfg.Model); model != "" {
		return model
	}
	if cfg.Provider == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Provider.GetDefaultModel())
}

func resolveWorkspace(workspace string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		return os.Getwd()
	}
	return expandHome(strings.TrimSpace(workspace))
}

func expandHome(path string) (string, error) {
	if path == "" {
		return path, nil
	}
	if path[0] != '~' {
		return filepath.Clean(path), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if len(path) == 1 {
		return home, nil
	}
	if path[1] == '/' {
		return filepath.Join(home, path[2:]), nil
	}

	return filepath.Clean(path), nil
}

// Close 释放 instance 持有的 session store 资源。
func (i *AgentInstance) Close() error {
	if i.Sessions != nil {
		return i.Sessions.Close()
	}
	return nil
}
