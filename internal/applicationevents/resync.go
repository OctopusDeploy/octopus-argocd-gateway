package applicationevents

import (
	"context"
	"log/slog"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
)

const applicationResyncInterval = 5 * time.Minute

type applicationPruner interface {
	PruneApplications() (*pb.PruneApplicationsResponse, error)
}

// resyncer periodically re-lists every Argo CD application, forwards each to Octopus through the
// shared dispatcher, then prunes applications that no longer exist in Argo CD.
type resyncer struct {
	logger     *slog.Logger
	interval   time.Duration
	argoClient argocd.Client
	sink       runner.MessageHandler[*v1alpha1.ApplicationWatchEvent]
	pruner     applicationPruner
}

// Used for tests as it provides an interval param will later be removed once that value is pulled from configuration
func newResyncer(
	logger *slog.Logger,
	interval time.Duration,
	argoClient argocd.Client,
	sink runner.MessageHandler[*v1alpha1.ApplicationWatchEvent],
	pruner applicationPruner,
) *resyncer {
	return &resyncer{
		logger:     logger.With(slog.String("component", "ApplicationResync")),
		interval:   interval,
		argoClient: argoClient,
		sink:       sink,
		pruner:     pruner,
	}
}

func newDefaultResyncer(
	logger *slog.Logger,
	argoClient argocd.Client,
	sink runner.MessageHandler[*v1alpha1.ApplicationWatchEvent],
	pruner applicationPruner,
) *resyncer {
	return newResyncer(logger, applicationResyncInterval, argoClient, sink, pruner)
}

func (r *resyncer) run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *resyncer) reconcile(ctx context.Context) {
	r.logger.Debug("Starting application resync")

	projects := r.argoClient.AvailableProjects()
	if len(projects) == 0 {
		r.forwardProject(ctx, "")
	} else {
		for _, project := range projects {
			if ctx.Err() != nil {
				return
			}
			r.forwardProject(ctx, project)
		}
	}

	if ctx.Err() != nil {
		return
	}

	if _, err := r.pruner.PruneApplications(); err != nil {
		if status.Code(err) == codes.Unimplemented {
			// Older Octopus Servers don't implement pruning; not an error worth surfacing each cycle.
			r.logger.Debug("Octopus Server does not support application pruning; skipping")
			return
		}
		r.logger.Error("Failed to prune stale applications during resync", slog.Any("error", err))
	}
}

func (r *resyncer) forwardProject(ctx context.Context, project string) {
	appList, err := r.argoClient.Applications().ListApplications(project)
	if err != nil {
		r.logger.Error("Failed to list applications during resync",
			slog.String("project", project), slog.Any("error", err))
		return
	}

	for i := range appList.Items {
		if ctx.Err() != nil {
			r.logger.Error("Failed to resync applications", slog.Any("error", err))
			return
		}
		_ = r.sink.ProcessMessage(&v1alpha1.ApplicationWatchEvent{
			Type:        watch.Modified,
			Application: appList.Items[i],
		})
	}

	r.logger.Debug("Resynced applications for project",
		slog.String("project", project), slog.Int("count", len(appList.Items)))
}
