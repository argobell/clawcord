package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/argobell/clawcord/cmd/clawcord/internal"
	"github.com/argobell/clawcord/internal/agent"
	"github.com/argobell/clawcord/pkg/config"
	"github.com/argobell/clawcord/pkg/providers"
	"github.com/argobell/clawcord/pkg/providers/openai"
	"github.com/argobell/clawcord/pkg/session"
	"github.com/argobell/clawcord/pkg/tools"
)

func agentRun(flags agentFlags) error {
	// Normalize session key
	sessionKey := strings.TrimSpace(flags.Session)
	if sessionKey == "" {
		sessionKey = "default"
	}

	// Apply debug logging if enabled
	if flags.Debug {
		fmt.Fprintln(os.Stderr, "Debug mode enabled")
	}

	// Load config
	cfg, err := internal.LoadConfig()
	if err != nil {
		return err
	}

	// Override model from flag if provided
	modelName := cfg.Agents.Defaults.GetModelName()
	if flags.Model != "" {
		modelName = flags.Model
	}

	// Resolve the default agent
	agentCfg := resolveDefaultAgent(cfg)

	// Resolve model_list to get provider configuration
	modelCfg, err := cfg.GetModelConfig(modelName)
	if err != nil {
		return fmt.Errorf("failed to resolve model %q: %w", modelName, err)
	}

	// Create provider
	provider, resolvedModel, err := createProviderFromModelConfig(modelCfg)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	// Create session store
	sessions := session.NewSessionManager(internal.GetClawcordHome())
	defer sessions.Close()

	// Create tool registry
	registry := tools.NewToolRegistry()

	// Create AgentInstance
	instance, err := agent.NewAgentInstance(agentCfg, cfg.Agents.Defaults, cfg, provider, sessions, registry)
	if err != nil {
		return fmt.Errorf("failed to create agent instance: %w", err)
	}
	defer instance.Close()

	sessions = session.NewSessionManager(sessionStoragePath(instance.Workspace))
	defer sessions.Close()
	instance.Sessions = sessions

	// Override model if resolved from model_list
	if resolvedModel != "" {
		instance.Model = resolvedModel
	}

	// Choose one-shot vs interactive behavior
	if flags.Message != "" {
		return runOneShot(instance, sessionKey, flags.Message)
	}

	return runInteractive(instance, sessionKey)
}

func resolveDefaultAgent(cfg *config.Config) config.AgentConfig {
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

func sessionStoragePath(workspace string) string {
	return filepath.Join(workspace, "sessions")
}

// httpProvider wraps openai.Provider to satisfy the LLMProvider interface.
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

func createProviderFromModelConfig(modelCfg *config.ModelConfig) (providers.LLMProvider, string, error) {
	model := strings.TrimSpace(modelCfg.Model)
	if model == "" {
		return nil, "", fmt.Errorf("model is required in model_list entry")
	}

	// Determine timeout
	timeout := 120 // default 120 seconds
	if modelCfg.RequestTimeout > 0 {
		timeout = modelCfg.RequestTimeout
	}

	// Create OpenAI-compatible provider
	inner := openai.NewProvider(
		modelCfg.APIKey,
		modelCfg.APIBase,
		modelCfg.Proxy,
		openai.WithRequestTimeout(time.Duration(timeout)*time.Second),
	)
	provider := &httpProvider{delegate: inner}

	return provider, model, nil
}

func runOneShot(instance *agent.AgentInstance, sessionKey, message string) error {
	ctx := context.Background()
	input := agent.TurnInput{
		SessionKey:  sessionKey,
		Channel:     "cli",
		ChatID:      "cli",
		UserMessage: message,
	}

	result, err := instance.RunTurn(ctx, input)
	if err != nil {
		return fmt.Errorf("agent turn failed: %w", err)
	}

	fmt.Println(result.Content)
	return nil
}

func runInteractive(instance *agent.AgentInstance, sessionKey string) error {
	fmt.Println("Interactive mode. Type 'exit' or 'quit' to exit.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	ctx := context.Background()

	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}

		// Trim and check for exit
		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			return nil
		}

		// Run turn
		turnInput := agent.TurnInput{
			SessionKey:  sessionKey,
			Channel:     "cli",
			ChatID:      "cli",
			UserMessage: input,
		}

		result, err := instance.RunTurn(ctx, turnInput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		fmt.Println(result.Content)
		fmt.Println()
	}
}
