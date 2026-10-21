package cmd

import (
	"github.com/spf13/cobra"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/configuration"
)

type RootCommander struct{}

func (c *RootCommander) GetCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "octopus-argocd-gateway",
		Short: "A brief description of your application",
		Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	}
	
	configManager := createConfigManager(rootCmd)

	// register each sub-command
	registerCommander := &RegisterCommander{
		ConfigManager: configManager,
	}
	rootCmd.AddCommand(registerCommander.GetCommand())

	runCommander := &RunCommander{
		ConfigManager: configManager,
	}
	rootCmd.AddCommand(runCommander.GetCommand())

	return rootCmd
}

func createConfigManager(rootCmd *cobra.Command) *configuration.ConfigManager {
	configManager := configuration.NewConfigManager()

	// add any global flags
	configManager.AddGlobalFlags(rootCmd.PersistentFlags())

	return configManager
}
