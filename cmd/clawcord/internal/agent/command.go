package agent

import "github.com/spf13/cobra"

func NewAgentCommand() *cobra.Command {
	var (
		message string
		session string
		model   string
		debug   bool
	)

	cmd := &cobra.Command{
		Use:     "agent",
		Aliases: []string{"a"},
		Short:   "Run the AI agent in one-shot or interactive mode",
		Example: "clawcord agent -m \"What is the weather?\"",
		Run: func(cmd *cobra.Command, args []string) {
			cobra.CheckErr(agentRun(agentFlags{
				Message: message,
				Session: session,
				Model:   model,
				Debug:   debug,
			}))
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Message for one-shot mode")
	cmd.Flags().StringVarP(&session, "session", "s", "", "Session key for conversation continuity")
	cmd.Flags().StringVar(&model, "model", "", "Override the default model")
	cmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")

	return cmd
}

type agentFlags struct {
	Message string
	Session string
	Model   string
	Debug   bool
}
