package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/logger"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/credentials"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/gateway"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/metrics"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/payload"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/pool"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/report"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/scenario"
)

type scenarioRuntime struct {
	name    scenario.Name
	log     *slog.Logger
	pool    *pool.Pool
	metrics *metrics.Metrics
	rec     *report.Recorder
}

func runScenario(parentCtx context.Context, flags *runFlags) error {
	ctx, cancel := signal.NotifyContext(parentCtx, os.Interrupt)
	defer cancel()

	rt, cleanup, err := initScenario(ctx, flags)
	if err != nil {
		return err
	}
	defer cleanup()

	scope, err := loadScopingPool(flags.scopingPoolPath)
	if err != nil {
		return err
	}
	if scope != nil {
		rt.log.Info("loaded scoping pool",
			slog.String("path", flags.scopingPoolPath),
			slog.Int("managed_percent", scope.ManagedPercent),
			slog.Int("projects", len(scope.Projects)),
			slog.Int("environments", len(scope.Environments)))
	}

	cfg := scenario.Config{
		Duration:       flags.duration,
		TargetRate:     flags.targetRate,
		PayloadSize:    payload.SizeClass(flags.payloadSize),
		Seed:           flags.seed,
		AppPoolSize:    flags.appPoolSize,
		Scope:          scope,
		AppsOutputPath: flags.appsOutputPath,
	}
	deps := scenario.Deps{
		Logger:  rt.log,
		Pool:    rt.pool,
		Metrics: rt.metrics,
		Rec:     rt.rec,
	}

	var rep *report.Report
	switch rt.name {
	case scenario.NameSyncLatency:
		rep, err = scenario.RunSyncLatency(ctx, cfg, deps)
	case scenario.NameIngest:
		rep, err = scenario.RunIngest(ctx, cfg, deps)
	case scenario.NameSaturation:
		rep, err = scenario.RunSaturation(ctx, cfg, deps)
	}
	if err != nil {
		return err
	}

	if err := report.Write(rep, flags.reportPath); err != nil {
		return err
	}
	rt.log.Info("scenario complete",
		slog.String("scenario", string(rt.name)),
		slog.Int64("update_successes", rep.UpdateSuccesses),
		slog.Int64("update_errors", rep.UpdateErrors),
		slog.Int64("syncs_received", rep.SyncsReceived),
		slog.Float64("achieved_rate", rep.AchievedRate))
	return nil
}

func initScenario(ctx context.Context, flags *runFlags) (*scenarioRuntime, func(), error) {
	if flags.scenario == "" {
		return nil, nil, fmt.Errorf("--scenario is required")
	}
	if flags.credentialsPath == "" {
		return nil, nil, fmt.Errorf("--credentials is required")
	}
	name, err := scenario.ParseName(flags.scenario)
	if err != nil {
		return nil, nil, err
	}

	log := logger.New(flags.debug)

	creds, err := credentials.Load(flags.credentialsPath)
	if err != nil {
		return nil, nil, err
	}
	entries := creds.Select(flags.gateways)
	log.Info("loaded credentials",
		slog.String("path", flags.credentialsPath),
		slog.Int("entries", len(entries)))

	m := metrics.New()
	var metricsShutdown func()
	if flags.metricsAddr != "" {
		lis, shutdown, err := m.Serve(flags.metricsAddr)
		if err != nil {
			return nil, nil, err
		}
		metricsShutdown = shutdown
		log.Info("metrics listening", slog.String("addr", lis.Addr().String()))
	}

	rec := report.NewRecorder()

	p := pool.New(pool.Config{
		Entries: entries,
		TLS: gateway.TLSOptions{
			Plaintext:      flags.plaintext || creds.Plaintext,
			PinCertificate: flags.pinCertificate || creds.PinCertificate,
		},
		DialTimeout:     flags.dialTimeout,
		DialConcurrency: flags.dialConcurrency,
	}, log, m, rec)

	if err := p.Dial(ctx); err != nil {
		if metricsShutdown != nil {
			metricsShutdown()
		}
		return nil, nil, err
	}

	cleanup := func() {
		p.Close()
		if metricsShutdown != nil {
			metricsShutdown()
		}
	}

	return &scenarioRuntime{
		name:    name,
		log:     log,
		pool:    p,
		metrics: m,
		rec:     rec,
	}, cleanup, nil
}

func loadScopingPool(path string) (*payload.ScopingPool, error) {
	if path == "" {
		return nil, nil
	}
	return payload.LoadScopingPool(path)
}
