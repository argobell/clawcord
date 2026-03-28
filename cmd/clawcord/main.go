package main

import (
	"fmt"
	"os"

	"github.com/argobell/clawcord/cmd/clawcord/internal/agent"
	"github.com/argobell/clawcord/cmd/clawcord/internal/gateway"
	"github.com/argobell/clawcord/cmd/clawcord/internal/onboard"
	"github.com/spf13/cobra"
)

func NewClawcordCommand() *cobra.Command {
	short := fmt.Sprintf("clawcord %s - A fun and lightweight AI assistant\n\n", "🦔")
	cmd := &cobra.Command{
		Use:     "clawcord",
		Short:   short,
		Example: "clawcord onboard",
	}

	cmd.AddCommand(
		onboard.NewOnboardCommand(),
		agent.NewAgentCommand(),
		gateway.NewGatewayCommand(),
	)

	return cmd
}

func main() {
	if err := NewClawcordCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
