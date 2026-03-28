package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/argobell/clawcord/cmd/clawcord/internal"
	"github.com/argobell/clawcord/cmd/clawcord/internal/runtime"
	"github.com/argobell/clawcord/internal/agent"
	"github.com/argobell/clawcord/pkg/logger"
)

func agentRun(flags agentFlags) error {
	// Normalize session key
	sessionKey := strings.TrimSpace(flags.Session)
	if sessionKey == "" {
		sessionKey = "default"
	}

	// Apply debug logging if enabled
	if flags.Debug {
		logger.SetLevel(logger.DEBUG)
		fmt.Fprintln(os.Stderr, "Debug mode enabled")
	}

	// Load config
	cfg, err := internal.LoadConfig()
	if err != nil {
		return err
	}

	// Resolve the default agent
	agentCfg := runtime.ResolveDefaultAgent(cfg)
	instance, err := runtime.NewConfiguredAgentInstance(cfg, agentCfg, flags.Model)
	if err != nil {
		return err
	}
	defer instance.Close()

	// Choose one-shot vs interactive behavior
	if flags.Message != "" {
		return runOneShot(instance, sessionKey, flags.Message)
	}

	return runInteractive(instance, sessionKey)
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
