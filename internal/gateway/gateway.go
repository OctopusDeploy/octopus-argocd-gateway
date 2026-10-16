package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/OctopusDeploy/octopus-grpc/go/pkg/connection"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/applicationevents"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/applicationmanifests"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/applicationresources"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/applicationresourcetree"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/applications"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/applicationsync"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/communication"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/configuration"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/info"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/leaderelection"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/metrics"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopus"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/podlogs"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/resourceevents"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/utilities"
)

type Gateway struct {
	context       context.Context
	logger        *slog.Logger
	ErrorCh       *chan error
	argoClient    argocd.Client
	octopusClient octopus.Client
	config        *configuration.Configuration
	runWg         sync.WaitGroup
	metrics       metrics.Metrics
}

func New(ctx context.Context, config *configuration.Configuration, logger *slog.Logger) (*Gateway, error) {
	errCh := make(chan error)

	octopusClient, err := octopus.NewClient(ctx, logger, octopus.Config{
		ServerURL:   config.Gateway.OctopusGrpcUrl,
		ClientID:    config.Gateway.OctopusGrpcClientId,
		BearerToken: config.Gateway.OctopusGrpcAuthenticationToken,
		TLS: octopus.TLSConfig{
			Plaintext:      config.Gateway.OctopusGrpcTlsPlaintext,
			Thumbprint:     config.Gateway.OctopusGrpcThumbprint,
			PinCertificate: config.Gateway.OctopusGrpcPinCertificate,
		},
	})
	if err != nil {
		return nil, err
	}

	argoClient, err := argocd.NewClient(ctx, logger, argocd.Config{
		ServerURL:         config.Gateway.ArgoCdServerUrl,
		AuthToken:         config.Gateway.ArgoCdAuthToken,
		ProjectAuthTokens: config.Gateway.ArgoCdProjectAuthTokens,
		GRPCWeb:           config.Gateway.ArgoCdGrpcWeb,
		GRPCWebRootPath:   config.Gateway.ArgoCdGrpcWebRootPath,
		TLS: argocd.TLSConfig{
			Plaintext: config.Gateway.ArgoCdServerTlsPlainText,
			Insecure:  config.Gateway.ArgoCdServerTlsInsecure,
		},
	})
	if err != nil {
		return nil, err
	}

	return &Gateway{
		context:       ctx,
		logger:        logger.With(slog.String("component", "Gateway")),
		ErrorCh:       &errCh,
		argoClient:    argoClient,
		octopusClient: octopusClient,
		config:        config,
		metrics:       metrics.New(),
	}, nil
}

func (g *Gateway) Start() {
	g.startMetricsServer()
	g.startPprofServer()
	g.startUpChecks()

	if configuration.GetGatewayRuntimeEnvironment() == configuration.GatewayRuntimeInCluster {
		g.runWithLeaderElection()
	} else {
		g.runLoop(g.context)
	}
}

// runWithLeaderElection runs the gateway's active workload (subscribers) under
// a Kubernetes lease so that at most one replica processes events at a time.
// With replicas=1 the single pod trivially wins the lease; scaling the
// Deployment to >1 enables active-passive failover. On loss of leadership the
// process exits so Kubernetes restarts it and it rejoins the election as a
// candidate, avoiding the risk of inconsistent state from a partial shutdown.
func (g *Gateway) runWithLeaderElection() {
	g.metrics.SetLeader(false)
	err := leaderelection.Run(
		g.context,
		g.logger,
		func(ctx context.Context) {
			g.metrics.SetLeader(true)
			g.runLoop(ctx)
		},
		func() {
			g.metrics.SetLeader(false)
			*g.ErrorCh <- fmt.Errorf("leader election: lost leadership")
		},
	)
	if err != nil {
		*g.ErrorCh <- err
	}
}

func (g *Gateway) startPprofServer() {
	port := g.config.Gateway.PprofPort
	if port == 0 {
		g.logger.Info("pprof server disabled (pprof-port=0)")
		return
	}

	// Enable block and mutex profiling (off by default in the Go runtime).
	// These add small sampling overhead, justified because pprof is itself opt-in.
	runtime.SetBlockProfileRate(10_000_000) // sample blocking events ~every 10ms
	runtime.SetMutexProfileFraction(100)    // sample 1/100 mutex contention events

	addr := "127.0.0.1:" + strconv.Itoa(port)
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		<-g.context.Done()
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()

	go func() {
		g.logger.Info("pprof server listening", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			g.logger.Error("pprof server error", slog.Any("error", err))
		}
	}()
}

func (g *Gateway) startMetricsServer() {
	port := g.config.Gateway.MetricsPort
	if port == 0 {
		g.logger.Info("Metrics server disabled (metrics-port=0)")
		return
	}

	addr := ":" + strconv.Itoa(port)
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		<-g.context.Done()
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()

	go func() {
		g.logger.Info("Metrics server listening", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			g.logger.Error("Metrics server error", slog.Any("error", err))
		}
	}()
}

// runLoop blocks until ctx is cancelled (leadership lost or process shutdown).
// It is invoked from within the leader-election OnStartedLeading callback.
func (g *Gateway) runLoop(ctx context.Context) {
	if g.config.Gateway.OctopusHealthCheckInterval > 0 {
		healthCheck := g.SubscribeToOctopusHealthCheck(ctx)
		g.healthCheckLoop(ctx, healthCheck, g.StartSubscribers)
	} else {
		g.logger.Debug("Health check is disabled")
		g.StartSubscribers(ctx)
		<-ctx.Done()
	}
	g.waitForSubscribers()
}

func (g *Gateway) healthCheckLoop(
	ctx context.Context,
	healthCheck *connection.HealthCheck,
	startSubscribers func(ctx context.Context),
) {
	for {
		runCtx, cancel := context.WithCancel(ctx)
		startSubscribers(runCtx)

		select {
		// Leader/global context is done
		case <-ctx.Done():
			cancel()
			return
		case event := <-healthCheck.Events():
			if event == connection.Down {
				// kill existing subscribers
				cancel()
				g.waitForSubscribers()
				// wait for healthy again
				if !healthCheck.AwaitRecovery(ctx) {
					*g.ErrorCh <- fmt.Errorf("health check did not recover in time, killing process")
					return
				}
			}
		// Maximum retries reached
		case healthCheckErr := <-healthCheck.Errors():
			cancel()
			*g.ErrorCh <- healthCheckErr
			return
		}
	}
}

// waitForSubscribers waits for all subscriber goroutines to exit, with a timeout.
func (g *Gateway) waitForSubscribers() {
	done := make(chan struct{})
	go func() {
		g.runWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		g.logger.Warn("timed out waiting for subscribers to stop")
	}
}

func (g *Gateway) startUpChecks() {
	err := communication.EnsureValidTlsConfiguration(
		g.context,
		g.logger,
		g.config,
	)
	if err != nil {
		*g.ErrorCh <- err
		return
	}

	report := argocd.ValidateAuthTokens(
		g.config.Gateway.ArgoCdAuthToken,
		g.config.Gateway.ArgoCdProjectAuthTokens,
		argocd.DefaultTokenExpiryWarningThreshold,
		utilities.NewClock(),
	)
	argocd.LogValidationReport(g.logger, report)
}

func (g *Gateway) StartSubscribers(ctx context.Context) {
	g.SubscribeToInfoRequests(ctx)
	g.SubscribeToGetGatewayLogsRequests(ctx)

	timeoutCtx, cancelFunc := context.WithTimeout(ctx, 10*time.Minute)
	defer cancelFunc()

	grpcWebMode := g.config.Gateway.ArgoCdGrpcWeb || g.config.Gateway.ArgoCdGrpcWebRootPath != ""
	var argoCdConn *grpc.ClientConn
	if !grpcWebMode {
		argoCdConn = g.argoClient.RawConnection()
		g.logger.Info("Waiting for connections to Octopus and Argo CD to reach state of READY...")
	} else {
		g.logger.Info("Waiting for connection to Octopus to reach state of READY...")
	}

	err := communication.WaitForReadyConnections(
		timeoutCtx,
		g.logger,
		g.octopusClient.RawConnection(),
		argoCdConn,
	)
	if err != nil {
		*g.ErrorCh <- err
		return
	}

	err = g.PruneStaleApplications(ctx)
	grpcErr := status.Convert(err)
	if err != nil && grpcErr.Code() != codes.Unimplemented {
		g.logger.With(slog.Any("error", err)).Error("Error pruning stale applications from Octopus Server")
		*g.ErrorCh <- err
		return
	}

	g.SubscribeToApplicationEvents(ctx)

	err = g.SubscribeToResourceWatch(ctx)
	if err != nil {
		g.logger.With(slog.Any("error", err)).Error("Error subscribing to application resource watch")
		*g.ErrorCh <- err
		return
	}

	g.SubscribeToManifestRequests(ctx)
	g.SubscribeToResourceEventsRequests(ctx)
	g.SubscribeToPodLogsRequests(ctx)
	g.SubscribeToSyncRequests(ctx)
	g.SubscribeToApplicationResourceTreeRequests(ctx)
}

func (g *Gateway) startSynchronizedRunner(runner octopus.Runner) {
	go func() {
		select {
		case <-runner.Context().Done():
			return
		case err := <-runner.Errors():
			*g.ErrorCh <- err
		}
	}()

	// g.runWg.Go(runner.Start)
	// TODO Replace this with waitGroup.Go() when this project is updated to at least go1.25
	g.runWg.Add(1)
	go func() {
		defer g.runWg.Done()
		runner.Start()
	}()
}

func (g *Gateway) startRunner(runner octopus.Runner) {
	go func() {
		select {
		case <-runner.Context().Done():
			return
		case err := <-runner.Errors():
			*g.ErrorCh <- err
		}
	}()
	go runner.Start()
}

func (g *Gateway) runnerOpts() []octopus.RunnerOption {
	var opts []octopus.RunnerOption
	if g.config.Gateway.StreamRetryBackoff > 0 {
		opts = append(opts, octopus.WithRetryBackoff(time.Duration(g.config.Gateway.StreamRetryBackoff)*time.Second))
	}
	return opts
}

func (g *Gateway) SubscribeToOctopusHealthCheck(ctx context.Context) *connection.HealthCheck {
	logger := g.logger.With(slog.String("component", "HealthCheck"))
	runner := connection.NewHealthCheck(ctx, g.octopusClient.Connection(), g.healthCheckConfig(), logger)
	g.startRunner(runner)
	return runner
}

func (g *Gateway) healthCheckConfig() connection.HealthCheckConfig {
	return connection.HealthCheckConfig{
		Interval:    g.config.Gateway.OctopusHealthCheckInterval,
		GiveUpAfter: g.config.Gateway.OctopusHealthCheckGiveUpAfter,
	}
}

func (g *Gateway) SubscribeToApplicationEvents(ctx context.Context) {
	logger := g.logger.With(slog.String("component", "ApplicationsEventHandler"))
	opts := append(g.runnerOpts(), octopus.WithMetrics(g.metrics, "application_events"))

	dispatcherCfg := applicationevents.NewDispatcherConfig(
		applicationevents.WithThreadiness(g.config.Gateway.ApplicationEventWorkers),
		applicationevents.WithMaxRetries(g.config.Gateway.ApplicationEventMaxRetries),
		applicationevents.WithTombstoneTTL(
			time.Duration(g.config.Gateway.ApplicationEventTombstoneTTLSeconds)*time.Second,
		),
		applicationevents.WithDrainTimeout(
			time.Duration(g.config.Gateway.ApplicationEventDrainTimeoutSeconds)*time.Second,
		),
		applicationevents.WithBufferDuration(
			time.Duration(g.config.Gateway.ApplicationEventBufferSeconds)*time.Second,
		),
	)

	eventTimeout := time.Duration(g.config.Gateway.ApplicationEventTimeoutSeconds) * time.Second
	runner := applicationevents.NewRunner(
		logger, ctx, g.octopusClient, g.argoClient, g.metrics, dispatcherCfg, eventTimeout, opts...,
	)
	g.startSynchronizedRunner(runner)
}

func (g *Gateway) SubscribeToResourceWatch(ctx context.Context) error {
	logger := g.logger.With(slog.String("component", "ApplicationResourcesHandler"))
	runner, err := applicationresources.NewRunner(logger, ctx, g.octopusClient, g.argoClient, g.metrics)
	if err != nil {
		return err
	}
	g.startSynchronizedRunner(runner)
	return nil
}

func (g *Gateway) SubscribeToInfoRequests(ctx context.Context) {
	logger := g.logger.With(slog.String("component", "InfoHandler"))
	opts := append(g.runnerOpts(), octopus.WithSilentUnimplemented(), octopus.WithMetrics(g.metrics, "info"))
	runner := info.NewRunner(
		logger,
		ctx,
		g.octopusClient,
		g.argoClient,
		g.config,
		opts...,
	)
	g.startSynchronizedRunner(runner)
}

func (g *Gateway) SubscribeToGetGatewayLogsRequests(ctx context.Context) {
	logger := g.logger.With(slog.String("component", "GatewayPodLogsHandler"))
	opts := append(g.runnerOpts(), octopus.WithSilentUnimplemented(), octopus.WithMetrics(g.metrics, "gateway_logs"))
	runner := info.NewGatewayPodLogsRunner(logger, ctx, g.octopusClient, opts...)
	g.startSynchronizedRunner(runner)
}

func (g *Gateway) SubscribeToManifestRequests(ctx context.Context) {
	logger := g.logger.With(slog.String("component", "ApplicationManifestHandler"))
	opts := append(
		g.runnerOpts(),
		octopus.WithSilentUnimplemented(),
		octopus.WithMetrics(g.metrics, "application_manifests"),
	)
	runner := applicationmanifests.NewRunner(
		logger,
		ctx,
		g.octopusClient,
		g.argoClient,
		opts...,
	)
	g.startSynchronizedRunner(runner)
}

func (g *Gateway) SubscribeToResourceEventsRequests(ctx context.Context) {
	logger := g.logger.With(slog.String("component", "ResourceEventsHandler"))
	opts := append(g.runnerOpts(), octopus.WithSilentUnimplemented(), octopus.WithMetrics(g.metrics, "resource_events"))
	runner := resourceevents.NewRunner(
		logger,
		ctx,
		g.octopusClient,
		g.argoClient,
		opts...,
	)
	g.startSynchronizedRunner(runner)
}

func (g *Gateway) SubscribeToPodLogsRequests(ctx context.Context) {
	logger := g.logger.With(slog.String("component", "PodsLogsHandler"))
	opts := append(g.runnerOpts(), octopus.WithSilentUnimplemented(), octopus.WithMetrics(g.metrics, "pod_logs"))
	runner := podlogs.NewRunner(
		logger,
		ctx,
		g.octopusClient,
		g.argoClient,
		opts...,
	)
	g.startSynchronizedRunner(runner)
}

func (g *Gateway) SubscribeToSyncRequests(ctx context.Context) {
	logger := g.logger.With(slog.String("component", "ApplicationSyncHandler"))
	opts := append(
		g.runnerOpts(),
		octopus.WithSilentUnimplemented(),
		octopus.WithMetrics(g.metrics, "application_sync"),
	)
	runner := applicationsync.NewRunner(
		logger,
		ctx,
		g.octopusClient,
		g.argoClient,
		opts...,
	)
	g.startSynchronizedRunner(runner)
}

func (g *Gateway) SubscribeToApplicationResourceTreeRequests(ctx context.Context) {
	logger := g.logger.With(slog.String("component", "ApplicationResourceTreeHandler"))
	opts := append(g.runnerOpts(), octopus.WithSilentUnimplemented())
	runner := applicationresourcetree.NewRunner(
		logger,
		ctx,
		g.octopusClient,
		g.argoClient,
		opts...,
	)
	g.startSynchronizedRunner(runner)
}

func (g *Gateway) PruneStaleApplications(ctx context.Context) error {
	logger := g.logger.With(slog.String("component", "PruneStaleApplications"))

	pruneStaleApplicationsService, err := applications.NewPruneApplicationsService(
		logger,
		ctx,
		g.octopusClient,
		g.argoClient,
	)
	if err != nil {
		return err
	}

	_, err = pruneStaleApplicationsService.PruneApplications()
	if err != nil {
		return err
	}

	return nil
}

type ContextKey string
