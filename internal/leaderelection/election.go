package leaderelection

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"k8s.io/client-go/tools/leaderelection"

	rl "k8s.io/client-go/tools/leaderelection/resourcelock"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/kubernetes"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/utilities"
)

const (
	defaultLockName = "octopus-argocd-gateway-leader"

	leaseDuration = 15 * time.Second
	renewDeadline = 10 * time.Second
	retryPeriod   = 2 * time.Second
)

// resolveLockName derives the lease name from the Helm release name so that
// two gateway releases in the same namespace hold separate leases. Falls back
// to the historical fixed name when HELM_RELEASE_NAME is unset.
func resolveLockName() string {
	releaseName := utilities.LookupEnvOrDefault("HELM_RELEASE_NAME", "")
	if releaseName == "" {
		return defaultLockName
	}
	return fmt.Sprintf("%s-leader", releaseName)
}

func resolveIdentity() (string, error) {
	if podName := os.Getenv("POD_NAME"); podName != "" {
		return podName, nil
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to determine identity: %w", err)
	}
	return hostname, nil
}

func resolveNamespace() (string, error) {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns, nil
	}
	if ns := os.Getenv("KUBERNETES_NAMESPACE"); ns != "" {
		return ns, nil
	}
	return "", fmt.Errorf("namespace not set: expected POD_NAMESPACE or KUBERNETES_NAMESPACE env var")
}

// Run starts leader election and blocks until ctx is cancelled. onStartedLeading
// runs in a goroutine when this replica acquires the lease and should block
// until the passed context is done; onStoppedLeading runs when leadership is
// lost and is expected to signal the caller to shut down. Namespace and
// identity are resolved from the pod's downward-API env vars.
func Run(
	ctx context.Context,
	logger *slog.Logger,
	onStartedLeading func(ctx context.Context),
	onStoppedLeading func(),
) error {
	namespace, err := resolveNamespace()
	if err != nil {
		return err
	}
	identity, err := resolveIdentity()
	if err != nil {
		return err
	}

	lockName := resolveLockName()
	logger = logger.With(
		slog.String("component", "LeaderElection"),
		slog.String("identity", identity),
		slog.String("namespace", namespace),
		slog.String("lockName", lockName),
	)

	cfg, err := kubernetes.NewKubernetesRestConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubernetes config for leader election: %w", err)
	}

	lock, err := rl.NewFromKubeconfig(
		rl.LeasesResourceLock,
		namespace,
		lockName,
		rl.ResourceLockConfig{Identity: identity},
		cfg,
		renewDeadline,
	)
	if err != nil {
		return fmt.Errorf("failed to create resource lock: %w", err)
	}

	elector, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: leaseDuration,
		RenewDeadline: renewDeadline,
		RetryPeriod:   retryPeriod,
		Name:          lockName,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				logger.Info("acquired leadership, starting active workload")
				onStartedLeading(ctx)
			},
			OnStoppedLeading: func() {
				logger.Warn("lost leadership, signalling shutdown")
				onStoppedLeading()
			},
			OnNewLeader: func(newLeader string) {
				if newLeader != identity {
					logger.Info("new leader observed", slog.String("leader", newLeader))
				}
			},
		},
		ReleaseOnCancel: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create leader elector: %w", err)
	}

	logger.Info("starting leader election")
	elector.Run(ctx)
	return nil
}
