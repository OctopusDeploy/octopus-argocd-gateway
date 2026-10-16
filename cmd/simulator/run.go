package main

import (
	"time"

	"github.com/spf13/cobra"
)

type runFlags struct {
	scenario        string
	credentialsPath string
	gateways        int
	duration        time.Duration
	targetRate      int
	payloadSize     string
	seed            uint64
	reportPath      string
	metricsAddr     string
	debug           bool
	dialTimeout     time.Duration
	dialConcurrency int
	plaintext       bool
	pinCertificate  bool
	appPoolSize     int
	scopingPoolPath string
	appsOutputPath  string
}

func newRunCommand() *cobra.Command {
	flags := &runFlags{}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one performance-test scenario",
		Long: `Run one of three scenarios: sync-latency, ingest, saturation.

  sync-latency  holds all virtual gateways open and responds to server-
                initiated sync requests as fast as possible.
  ingest        fires ApplicationEventUpdate at --target-rate for --duration
                with --payload={small,medium,large}.
  saturation    ramps the update rate (double every 60s) until a stop
                condition fires or the 4000 rps ceiling is reached.

The simulator consumes a pre-seeded credentials JSON file via --credentials.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScenario(cmd.Context(), flags)
		},
	}

	f := cmd.Flags()
	f.StringVar(&flags.scenario, "scenario", "", "one of: sync-latency, ingest, saturation (required)")
	f.StringVar(&flags.credentialsPath, "credentials", "", "path to pre-seeded credentials JSON (required)")
	f.IntVar(&flags.gateways, "gateways", 0, "cap on virtual gateways (0 = use all entries in credentials file)")
	f.DurationVar(&flags.duration, "duration", 60*time.Second, "scenario duration (0 = run until cancelled)")
	f.IntVar(
		&flags.targetRate,
		"target-rate",
		0,
		"updates/s (ingest: required; saturation: starting rate, default 100)",
	)
	f.StringVar(&flags.payloadSize, "payload", "small", "small|medium|large (ingest/saturation)")
	f.Uint64Var(&flags.seed, "seed", 1, "deterministic payload seed")
	f.StringVar(&flags.reportPath, "report", "-", "JSON report output path (- or empty for stdout)")
	f.StringVar(
		&flags.metricsAddr,
		"metrics-addr",
		":9090",
		"address for the Prometheus /metrics endpoint (empty to disable)",
	)
	f.BoolVar(&flags.debug, "debug", false, "enable debug logging")
	f.DurationVar(&flags.dialTimeout, "connect-timeout", 30*time.Second, "per-gateway gRPC dial timeout")
	f.IntVar(&flags.dialConcurrency, "dial-concurrency", 32, "parallel dials during pool setup")
	f.BoolVar(&flags.plaintext, "plaintext", false, "skip TLS (dev only)")
	f.BoolVar(
		&flags.pinCertificate,
		"pin-certificate",
		false,
		"enforce certificate pinning using each entry's thumbprint",
	)
	f.IntVar(
		&flags.appPoolSize,
		"app-pool-size",
		0,
		"distinct application UIDs the payload generator pre-marshals per size class (0 uses the simulator's default)",
	)
	f.StringVar(
		&flags.scopingPoolPath,
		"scoping-pool",
		"",
		"path to a JSON file describing Octopus project/environment slugs to stamp as argo.octopus.com/* annotations on a configurable percentage of generated applications (empty disables annotations)",
	)
	f.StringVar(
		&flags.appsOutputPath,
		"apps-out",
		"",
		"sync-latency only: path to write a JSON array of advertised application names once pre-ingest completes (empty disables)",
	)

	return cmd
}
