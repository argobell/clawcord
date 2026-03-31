package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/argobell/clawcord/pkg/config"
	"github.com/argobell/clawcord/pkg/providers"
	"github.com/argobell/clawcord/pkg/session"
	"github.com/argobell/clawcord/pkg/tools"
)

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

// NewAgentInstance 根据项目配置和默认值组装一个可运行的 Agent 实例。
func NewAgentInstance(
	agentCfg config.AgentConfig,
	defaults config.AgentDefaults,
	cfg *config.Config,
	provider providers.LLMProvider,
	sessions session.SessionStore,
	registry *tools.ToolRegistry,
) (*AgentInstance, error) {
	modelAlias := resolveAgentModel(agentCfg, defaults)
	workspace, err := resolveAgentWorkspace(agentCfg, defaults)
	if err != nil {
		return nil, err
	}

	model := strings.TrimSpace(modelAlias)
	if cfg != nil && model != "" && !strings.Contains(model, "/") {
		// 如果这里只给了模型别名，就尝试映射到具体模型配置。
		if resolved, err := cfg.GetModelConfig(model); err == nil && resolved != nil {
			model = strings.TrimSpace(resolved.Model)
		} else if err != nil {
			return nil, err
		}
	}

	if provider == nil {
		return nil, fmt.Errorf("provider is required")
	}
	if model == "" {
		model = strings.TrimSpace(provider.GetDefaultModel())
	}
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	// sessions 和 registry 允许外部注入；为空时使用默认实现兜底。
	if sessions == nil {
		sessions = session.NewSessionManager("")
	}

	if registry == nil {
		registry = tools.NewToolRegistry()
	}

	maxIterations := defaults.MaxToolIterations
	if maxIterations == 0 {
		maxIterations = 20
	}

	maxTokens := defaults.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	temperature := 0.7
	if defaults.Temperature != nil {
		temperature = *defaults.Temperature
	}

	id := strings.TrimSpace(agentCfg.ID)
	if id == "" {
		id = "main"
	}

	return &AgentInstance{
		ID:             id,
		Name:           strings.TrimSpace(agentCfg.Name),
		Model:          model,
		Workspace:      workspace,
		MaxIterations:  maxIterations,
		MaxTokens:      maxTokens,
		Temperature:    temperature,
		Provider:       provider,
		Sessions:       sessions,
		ContextBuilder: NewContextBuilder(workspace, ""),
		Tools:          registry,
	}, nil
}

// resolveAgentModel 优先读取 Agent 自己的模型配置，未配置时回退到默认模型名。
func resolveAgentModel(agentCfg config.AgentConfig, defaults config.AgentDefaults) string {
	if model := strings.TrimSpace(agentCfg.Model); model != "" {
		return model
	}
	return strings.TrimSpace(defaults.GetModelName())
}

// resolveAgentWorkspace 负责决定 Agent 的工作区路径。
// main agent 使用默认工作区，其他 agent 则派生独立子目录，避免互相覆盖。
func resolveAgentWorkspace(agentCfg config.AgentConfig, defaults config.AgentDefaults) (string, error) {
	if strings.TrimSpace(agentCfg.Workspace) != "" {
		return expandHome(strings.TrimSpace(agentCfg.Workspace))
	}

	base := strings.TrimSpace(defaults.Workspace)
	if normalizeAgentID(agentCfg.ID) == "main" {
		return resolveWorkspace(base)
	}
	if base == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	baseWorkspace, err := expandHome(base)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(baseWorkspace), "workspace-"+normalizeAgentID(agentCfg.ID)), nil
}

// normalizeAgentID 统一整理 Agent ID，空值时视为 main。
func normalizeAgentID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		return "main"
	}
	return id
}

// resolveWorkspace 解析最终工作区路径；空值时退回当前工作目录。
func resolveWorkspace(workspace string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		return os.Getwd()
	}
	return expandHome(strings.TrimSpace(workspace))
}

// expandHome 将 ~ 展开为用户主目录，并清理普通路径。
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
