package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/argobell/clawcord/internal/agent"
	builtintools "github.com/argobell/clawcord/internal/tools"
	"github.com/argobell/clawcord/pkg/config"
	"github.com/argobell/clawcord/pkg/media"
	"github.com/argobell/clawcord/pkg/providers"
	"github.com/argobell/clawcord/pkg/providers/openai"
	"github.com/argobell/clawcord/pkg/session"
	"github.com/argobell/clawcord/pkg/tools"
)

// ResolveDefaultAgent returns the main agent when present, otherwise the first configured agent.
func ResolveDefaultAgent(cfg *config.Config) config.AgentConfig {
	for _, agentCfg := range cfg.Agents.List {
		if strings.TrimSpace(agentCfg.ID) == "main" {
			return agentCfg
		}
	}
	if len(cfg.Agents.List) > 0 {
		return cfg.Agents.List[0]
	}
	return config.AgentConfig{ID: "main"}
}

// SessionStoragePath returns the workspace-scoped session directory.
func SessionStoragePath(workspace string) string {
	return filepath.Join(workspace, "sessions")
}

// ResolveModelName picks the CLI override first, then per-agent config, then defaults.
func ResolveModelName(agentCfg config.AgentConfig, defaults config.AgentDefaults, override string) string {
	if model := strings.TrimSpace(override); model != "" {
		return model
	}
	if model := strings.TrimSpace(agentCfg.Model); model != "" {
		return model
	}
	return strings.TrimSpace(defaults.GetModelName())
}

// httpProvider wraps the OpenAI-compatible provider to satisfy the shared provider interface.
type httpProvider struct {
	delegate *openai.Provider
}

func (p *httpProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	return p.delegate.Chat(ctx, messages, tools, model, options)
}

func (p *httpProvider) GetDefaultModel() string {
	return ""
}

// CreateProviderFromModelConfig builds an OpenAI-compatible provider from a model_list entry.
func CreateProviderFromModelConfig(modelCfg *config.ModelConfig) (providers.LLMProvider, string, error) {
	model := strings.TrimSpace(modelCfg.Model)
	if model == "" {
		return nil, "", fmt.Errorf("model is required in model_list entry")
	}

	timeout := 120
	if modelCfg.RequestTimeout > 0 {
		timeout = modelCfg.RequestTimeout
	}

	inner := openai.NewProvider(
		modelCfg.APIKey,
		modelCfg.APIBase,
		modelCfg.Proxy,
		openai.WithRequestTimeout(time.Duration(timeout)*time.Second),
	)

	return &httpProvider{delegate: inner}, model, nil
}

// NewDefaultToolRegistry creates the default runtime tool registry.
func NewDefaultToolRegistry(workspace string, store media.MediaStore) *tools.ToolRegistry {
	registry := tools.NewToolRegistry()
	RegisterDefaultTools(registry, workspace, store)
	return registry
}

// RegisterDefaultTools installs the currently supported default tools into an existing registry.
func RegisterDefaultTools(registry *tools.ToolRegistry, workspace string, store media.MediaStore) {
	if registry == nil {
		return
	}
	// send_file only makes sense when the runtime has a media store and an outbound delivery path.
	if store != nil {
		registry.Register(builtintools.NewSendFileTool(workspace, 0, store))
	}
}

// NewConfiguredAgentInstance builds the default runtime assembly for CLI commands.
func NewConfiguredAgentInstance(
	cfg *config.Config,
	agentCfg config.AgentConfig,
	modelOverride string,
) (*agent.AgentInstance, error) {
	modelName := ResolveModelName(agentCfg, cfg.Agents.Defaults, modelOverride)
	modelCfg, err := cfg.GetModelConfig(modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve model %q: %w", modelName, err)
	}

	provider, resolvedModel, err := CreateProviderFromModelConfig(modelCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	// Build once to reuse the resolved workspace for session storage.
	tmpInstance, err := agent.NewAgentInstance(agentCfg, cfg.Agents.Defaults, cfg, provider, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent instance: %w", err)
	}
	workspace := tmpInstance.Workspace
	_ = tmpInstance.Close()

	sessions := session.NewSessionManager(SessionStoragePath(workspace))
	registry := NewDefaultToolRegistry(workspace, nil)
	instance, err := agent.NewAgentInstance(agentCfg, cfg.Agents.Defaults, cfg, provider, sessions, registry)
	if err != nil {
		_ = sessions.Close()
		return nil, fmt.Errorf("failed to create agent instance: %w", err)
	}
	if resolvedModel != "" {
		instance.Model = resolvedModel
	}
	return instance, nil
}
