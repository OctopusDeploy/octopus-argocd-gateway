package applicationevents

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/applications"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/metrics"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopus"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
)

// DefaultEventTimeout caps per-event platform I/O (cluster lookup + Octopus send).
// Without it, a server that accepts but never responds blocks a worker indefinitely — keepalive cannot detect this.
const DefaultEventTimeout = 30 * time.Second

type missingClusterKey struct {
	id     string
	idType argocd.ClusterIdType
}

type Handler struct {
	logger                *slog.Logger
	ctx                   context.Context
	client                pb.ApplicationEventServiceClient
	clusterService        argocd.ClusterService
	projectService        argocd.ProjectService
	metrics               metrics.Metrics
	eventTimeout          time.Duration
	loggedMissingClusters sync.Map
	loggedMissingProjects sync.Map
}

func NewRunner(
	logger *slog.Logger,
	ctx context.Context,
	octopusClient octopus.Client,
	argoClient argocd.Client,
	m metrics.Metrics,
	dispatcherCfg DispatcherConfig,
	eventTimeout time.Duration,
	opts ...octopus.RunnerOption,
) octopus.Runner {
	if eventTimeout <= 0 {
		eventTimeout = DefaultEventTimeout
	}

	// All project streams share one dispatcher; project is derived per-event from Spec.Project to pick the correct auth token.
	// ctx is detached (WithoutCancel) so drain-time sends are not cancelled when ctx is cancelled to trigger the drain.
	handler := &Handler{
		logger:         logger,
		ctx:            context.WithoutCancel(ctx),
		client:         octopusClient.ApplicationEvents(),
		clusterService: argoClient.Clusters(),
		projectService: argoClient.Projects(),
		metrics:        m,
		eventTimeout:   eventTimeout,
	}

	dispatcher := NewDispatchingHandler(handler, logger, m, "application_events", dispatcherCfg)
	dispatcher.Start(ctx)

	// Periodically refreshes the all applications by listing and pushing them through the event service and also runs an application prune
	// to ensure that Octopus always has the most up-to-date view of Argo CD applications
	if pruneService, err := applications.NewPruneApplicationsService(
		logger,
		ctx,
		octopusClient,
		argoClient,
	); err != nil {
		logger.Error("Failed to create prune service; application resync disabled", slog.Any("error", err))
	} else {
		go newDefaultResyncer(logger, argoClient, dispatcher, pruneService).run(ctx)
	}

	var inner runner.Runner
	projects := argoClient.AvailableProjects()
	if len(projects) == 0 {
		src := argoClient.ApplicationsWatchSource()
		inner = runner.NewWatchRunner(src, dispatcher, logger, ctx, opts...)
	} else {
		// Each project needs its own authenticated stream, but all feed the shared dispatcher.
		subs := make([]runner.Runner, 0, len(projects))
		for _, p := range projects {
			ctxWithProj := argocd.WithArgoProject(ctx, p)
			src := argoClient.ApplicationsWatchSource()
			subs = append(subs, runner.NewWatchRunner(src, dispatcher, logger, ctxWithProj, opts...))
		}
		inner = runner.NewMultiRunner(ctx, subs)
	}

	// Wrap so shutdown blocks on the dispatcher draining before returning to the gateway's wait group.
	return &drainingRunner{inner: inner, dispatcher: dispatcher}
}

// drainingRunner ensures the dispatcher drains its queue before the gateway's wait group unblocks on shutdown.
// Without it, the process could exit while workers are still sending events enqueued just before shutdown.
type drainingRunner struct {
	inner      runner.Runner
	dispatcher *DispatchingHandler
}

func (r *drainingRunner) Start() {
	r.inner.Start()
	r.dispatcher.WaitForDrain()
}

func (r *drainingRunner) Errors() <-chan error {
	return r.inner.Errors()
}

func (r *drainingRunner) Context() context.Context {
	return r.inner.Context()
}

func (h *Handler) ProcessMessage(eventMsg *v1alpha1.ApplicationWatchEvent) error {
	h.metrics.IncApplicationEventsByType(eventMsg.Type)

	// Project comes from the application (not context) to select the correct project-scoped Argo CD auth token.
	eventCtx := h.ctx
	if project := eventMsg.Application.Spec.Project; project != "" {
		eventCtx = argocd.WithArgoProject(h.ctx, project)
	}

	// eventTimeout is the only deadline here — the context is detached and the gRPC client has no per-call deadline.
	// The > 0 guard is for tests that construct Handler directly without going through NewRunner.
	if h.eventTimeout > 0 {
		var cancel context.CancelFunc
		eventCtx, cancel = context.WithTimeout(eventCtx, h.eventTimeout)
		defer cancel()
	}

	logger := h.logger.With(
		slog.Any("eventType", eventMsg.Type),
		slog.String("application", eventMsg.Application.QualifiedName()),
	)
	applicationDestination := eventMsg.Application.Spec.Destination
	clusterId := applicationDestination.Server
	clusterIdType := argocd.ClusterIdServer

	if applicationDestination.Name != "" {
		clusterId = applicationDestination.Name
		clusterIdType = argocd.ClusterIdName
	}

	cluster, err := h.clusterService.GetCluster(eventCtx, clusterId, clusterIdType)
	switch {
	case err == nil:
		eventMsg.Application = *argocd.InjectAnnotations(&eventMsg.Application, cluster.Annotations)
	case errors.Is(err, argocd.ErrPermissionDenied):
		fallthrough
	case errors.Is(err, argocd.ErrNotFound):
		key := missingClusterKey{id: clusterId, idType: clusterIdType}
		if _, alreadyLogged := h.loggedMissingClusters.LoadOrStore(key, struct{}{}); !alreadyLogged {
			project, ok := argocd.ArgoProjectFromContext(eventCtx)
			if !ok {
				project = "Global"
			}
			logger.Info(
				"No ArgoCD cluster matches application destination. This is likely due to missing permissions.",
				slog.String("clusterId", clusterId),
				slog.Any("clusterIdType", clusterIdType),
				slog.Any("Project", project),
			)
		}
	default:
		logger.Error("Failed to get cluster", slog.Any("error", err))
	}

	projectName := eventMsg.Application.Spec.GetProject()
	appProject, err := h.projectService.GetProject(eventCtx, projectName)
	switch {
	case err == nil:
		eventMsg.Application = *argocd.InjectAnnotations(&eventMsg.Application, appProject.Annotations)
	case errors.Is(err, argocd.ErrPermissionDenied):
		fallthrough
	case errors.Is(err, argocd.ErrNotFound):
		if _, alreadyLogged := h.loggedMissingProjects.LoadOrStore(projectName, struct{}{}); !alreadyLogged {
			logger.Info(
				"No ArgoCD project matches the application's project. This is likely due to missing permissions.",
				slog.String("project", projectName),
			)
		}
	default:
		logger.Error("Failed to get project", slog.Any("error", err))
	}

	logger.DebugContext(eventCtx, "Sending event to Octopus Deploy")
	eventJson, err := json.Marshal(eventMsg)
	if err != nil {
		logger.Error("Error marshalling application event to JSON", slog.Any("error", err))
		return nil
	}

	request := &pb.ApplicationEventUpdateRequest{
		EventPayloadJson: string(eventJson),
	}

	sendStart := time.Now()
	_, err = h.client.ApplicationEventUpdate(eventCtx, request)
	h.metrics.ObserveApplicationEventSendDuration(time.Since(sendStart).Seconds())
	if err != nil {
		h.metrics.IncApplicationEventsSent(eventMsg.Type, "error")
		logger.Error("Error sending application event to Octopus Deploy", slog.Any("error", err))
		// Return the error so the dispatcher retries with backoff rather than silently dropping the event.
		return err
	}

	h.metrics.IncApplicationEventsSent(eventMsg.Type, "success")
	logger.InfoContext(eventCtx, "Sent application event response")
	return nil
}
