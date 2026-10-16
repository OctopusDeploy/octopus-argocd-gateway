package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/applicationevents"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/certificate"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/configuration"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/gateway"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/logger"
)

type RunCommander struct {
	ConfigManager *configuration.ConfigManager
}

func (c *RunCommander) GetCommand() *cobra.Command {
	// runCmd represents the run command
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Launches the Octopus Deploy ArgoCD Gateway process with the provided configuration, requires the Gateway to be registered with the Octopus Deploy server",
		Long: `Launches the Octopus Deploy ArgoCD Gateway process with the provided configuration, requires the Gateway to be registered with the Octopus Deploy server.

Example: 
octopus-argocd-gateway run --argocd-auth-token <token> --argocd-server-url <url>
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle SIGINT (CTRL+C) gracefully.
			ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)

			config := c.ConfigManager.GetConfiguration()
			log := logger.New(config.Debug)

			err := certificate.InitializeCertificatePool(config.Gateway.CertificatePath, log)
			if err != nil {
				log.Warn(
					"Failed to initialize certificate pool during startup, not all certificates were successfully added to the pool",
					slog.Any("error", err),
				)
			}

			gatewayProcess, err := gateway.New(ctx, config, log)
			if err != nil {
				return err
			}

			go gatewayProcess.Start()

			select {
			case <-ctx.Done():
				log.InfoContext(ctx, "Shutting Down...")
				return nil

			case chanErr := <-*gatewayProcess.ErrorCh:
				logger.LogMultiError(log, "Unrecoverable error(s) have been encountered", chanErr)
				return nil
			}
		},
	}

	c.ConfigManager.AddStringFlag(
		runCmd.Flags(),
		"argocd-auth-token",
		"",
		"Sets the ArgoCD token used for authenticating with the ArgoCD API",
	)

	c.ConfigManager.AddStringFlag(runCmd.Flags(), "argocd-server-url", "", "ArgoCD instance URL")
	c.ConfigManager.AddBoolFlag(
		runCmd.Flags(),
		"argocd-server-grpcweb",
		false,
		"Use the gRPC-Web protocol to connect to the ArgoCD API",
	)
	c.ConfigManager.AddStringFlag(
		runCmd.Flags(),
		"argocd-server-grpcweb-root-path",
		"",
		"Enables gRPC-Web protocol and sets the path to rewrite in the gRPC-Web request",
	)

	c.ConfigManager.AddIntFlag(
		runCmd.Flags(),
		"metrics-port",
		9090,
		"Port to expose Prometheus metrics on (0 to disable)",
	)

	c.ConfigManager.AddIntFlag(
		runCmd.Flags(),
		"pprof-port",
		0,
		"Port to expose net/http/pprof on, bound to 127.0.0.1 (0 to disable)",
	)

	c.ConfigManager.AddIntFlag(
		runCmd.Flags(),
		"application-event-workers",
		applicationevents.DefaultThreadiness,
		"Number of worker goroutines processing ArgoCD application events concurrently",
	)

	c.ConfigManager.AddIntFlag(
		runCmd.Flags(),
		"application-event-timeout-seconds",
		int(applicationevents.DefaultEventTimeout.Seconds()),
		"Maximum seconds to resolve the destination cluster and send a single ArgoCD application "+
			"event before it times out and is retried",
	)

	c.ConfigManager.AddIntFlag(
		runCmd.Flags(),
		"application-event-max-retries",
		applicationevents.DefaultMaxRetries,
		"Number of times a failing ArgoCD application event is requeued before it is dropped",
	)

	c.ConfigManager.AddIntFlag(
		runCmd.Flags(),
		"application-event-tombstone-ttl-seconds",
		int(applicationevents.DefaultTombstoneTTL.Seconds()),
		"How long, in seconds, a delete tombstone is retained to reject stale updates to an "+
			"already-deleted ArgoCD application",
	)

	c.ConfigManager.AddIntFlag(
		runCmd.Flags(),
		"application-event-drain-timeout-seconds",
		int(applicationevents.DefaultDrainTimeout.Seconds()),
		"Maximum seconds a graceful shutdown waits for in-flight and queued ArgoCD application "+
			"events to be sent before the process exits",
	)

	c.ConfigManager.AddIntFlag(
		runCmd.Flags(),
		"application-event-buffer-seconds",
		int(applicationevents.DefaultBufferDuration.Seconds()),
		"Number of seconds to buffer incoming ArgoCD application events before dispatching to Octopus Server. "+
			"Received events for the same ArgoCD Application in a buffer window will result in only the latest event being dispatched. "+
			"Deleted and Added events are never buffered. (0 disables buffering)",
	)

	return runCmd
}
