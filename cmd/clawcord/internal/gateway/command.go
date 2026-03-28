package gateway

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func NewGatewayCommand() *cobra.Command {
	var debug bool

	cmd := &cobra.Command{
		Use:     "gateway",
		Aliases: []string{"g"},
		Short:   "Run the long-lived Discord gateway runtime",
		Example: "clawcord gateway",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			cobra.CheckErr(gatewayRun(ctx, gatewayFlags{
				Debug: debug,
			}))
		},
	}

	cmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")

	return cmd
}

type gatewayFlags struct {
	Debug bool
}
