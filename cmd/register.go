package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/certificate"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/configuration"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/logger"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/registration"
)

type RegisterCommander struct {
	ConfigManager *configuration.ConfigManager
}

func (c *RegisterCommander) GetCommand() *cobra.Command {
	// registerCmd represents the register command
	registerCmd := &cobra.Command{
		Use:   "register",
		Short: "A brief description of your command",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		Run: func(cmd *cobra.Command, args []string) {
			// Handle SIGINT (CTRL+C) gracefully.
			ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)

			config := c.ConfigManager.GetConfiguration()
			logger := logger.New(config.Debug)

			err := certificate.InitializeCertificatePool(config.Gateway.CertificatePath, logger)
			if err != nil {
				logger.Warn(
					"Failed to initialize certificate pool during registration, not all certificates were successfully added to the pool",
					slog.Any("error", err),
				)
			}

			err = registration.Run(ctx, config, logger)
			cobra.CheckErr(err)
		},
	}

	c.ConfigManager.AddStringFlag(
		registerCmd.Flags(),
		"server-api-url",
		"",
		"Address of the Octopus Server API. e.g. https://myinstance.octopus.app")

	c.ConfigManager.AddSecretStringFlag(
		registerCmd.Flags(),
		"server-access-token",
		"",
		"Access token to use when connecting to the server, can be a long lived API key or short lived bearer token.")

	c.ConfigManager.AddStringFlag(
		registerCmd.Flags(),
		"space-id",
		"",
		"ID of the space where the gateway will be registered with the server")

	c.ConfigManager.AddStringArrayFlag(
		registerCmd.Flags(),
		"space-ids",
		[]string{},
		"IDs of spaces to register the gateway with, merged with space-id")

	c.ConfigManager.AddStringFlag(
		registerCmd.Flags(),
		"gateway-name",
		"",
		"The name of the gateway")

	c.ConfigManager.AddStringFlag(
		registerCmd.Flags(),
		"argocd-web-ui-url",
		"",
		"The URL for the Argo CD Web UI")

	c.ConfigManager.AddStringArrayFlag(
		registerCmd.Flags(),
		"environment",
		[]string{},
		"The ID or slug of an environment to associate the gateway with")

	c.ConfigManager.AddStringArrayFlag(
		registerCmd.Flags(),
		"tenant",
		[]string{},
		"The ID or slug of a tenant to associate the gateway with")

	c.ConfigManager.AddStringFlag(
		registerCmd.Flags(),
		"secret-name",
		"",
		"Name of the secret where the gateway configuration will be stored")

	c.ConfigManager.AddStringFlag(
		registerCmd.Flags(),
		"namespace",
		"",
		"The name of the namespace where the gateway is installed")

	return registerCmd
}
