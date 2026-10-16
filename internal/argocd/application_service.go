package argocd

import (
	"context"
	"log/slog"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"google.golang.org/grpc"

	v1 "k8s.io/api/core/v1"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/utilities"
)

type ApplicationService interface {
	ListApplications(applicationProject string) (*v1alpha1.ApplicationList, error)
	WatchApplications(ctx context.Context) (application.ApplicationService_WatchClient, error)
	WatchApplicationResourceTree(
		ctx context.Context,
		applicationProject,
		applicationName,
		applicationNamespace string,
	) (application.ApplicationService_WatchResourceTreeClient, error)
	GetApplication(
		applicationProject,
		applicationName,
		applicationNamespace string,
	) (*v1alpha1.Application, error)
	GetApplicationResourceTree(
		applicationProject,
		applicationName,
		applicationNamespace string,
	) (*v1alpha1.ApplicationTree, error)
	GetManagedResourceManifests(
		query *application.ResourcesQuery,
	) (*application.ManagedResourcesResponse, error)
	GetResourceLiveManifest(
		query *application.ApplicationResourceRequest,
	) (*application.ApplicationResourceResponse, error)
	GetResourceEvents(query *application.ApplicationResourceEventsQuery) (*v1.EventList, error)
	GetPodLogs(query *application.ApplicationPodLogsQuery) (application.ApplicationService_PodLogsClient, error)
	SyncApplication(
		applicationProject,
		applicationName,
		applicationNamespace string,
		opts *SyncOptions,
	) (*v1alpha1.Application, error)
}

type SyncOptions struct {
	Prune    bool
	Revision string
}

type applicationServiceImpl struct {
	logger   *slog.Logger
	context  context.Context
	grpcConn *grpc.ClientConn
	client   application.ApplicationServiceClient
}

func newApplicationService(
	ctx context.Context,
	logger *slog.Logger,
	grpcConn *grpc.ClientConn,
) ApplicationService {
	return &applicationServiceImpl{
		logger:   logger,
		context:  ctx,
		grpcConn: grpcConn,
		client:   application.NewApplicationServiceClient(grpcConn),
	}
}

func (a *applicationServiceImpl) ListApplications(applicationProject string) (*v1alpha1.ApplicationList, error) {
	a.logger.Debug("Listing Argo CD Applications")

	ctx := WithArgoProject(a.context, applicationProject)
	applicationList, err := a.client.List(ctx, &application.ApplicationQuery{})
	if err != nil {
		return nil, err
	}

	return applicationList, nil
}

func (a *applicationServiceImpl) WatchApplications(
	ctx context.Context,
) (application.ApplicationService_WatchClient, error) {
	a.logger.Debug("Watching Argo CD Applications")

	watch, err := a.client.Watch(ctx, &application.ApplicationQuery{})
	if err != nil {
		return nil, err
	}

	return watch, nil
}

func (a *applicationServiceImpl) GetApplication(
	applicationProject,
	applicationName,
	applicationNamespace string,
) (*v1alpha1.Application, error) {
	a.logger.Debug("Getting Argo CD Application")

	ctx := WithArgoProject(a.context, applicationProject)
	app, err := a.client.Get(ctx, &application.ApplicationQuery{
		Name:         &applicationName,
		AppNamespace: &applicationNamespace,
		Project:      []string{applicationProject},
	})
	if err != nil {
		return nil, err
	}

	return app, nil
}

func (a *applicationServiceImpl) WatchApplicationResourceTree(
	ctx context.Context,
	applicationProject,
	applicationName,
	applicationNamespace string,
) (application.ApplicationService_WatchResourceTreeClient, error) {
	a.logger.Debug("Watching Argo CD Application Resources")

	ctxWithProj := WithArgoProject(ctx, applicationProject)
	watch, err := a.client.WatchResourceTree(ctxWithProj, &application.ResourcesQuery{
		ApplicationName: &applicationName,
		Project:         &applicationProject,
		AppNamespace:    &applicationNamespace,
	})
	if err != nil {
		return nil, err
	}

	return watch, nil
}

func (a *applicationServiceImpl) GetApplicationResourceTree(
	applicationProject,
	applicationName,
	applicationNamespace string,
) (*v1alpha1.ApplicationTree, error) {
	a.logger.With(
		slog.String("application", utilities.QualifiedName(applicationName, applicationNamespace)),
	).Debug("Getting Argo CD Application Resources")

	ctx := WithArgoProject(a.context, applicationProject)
	res, err := a.client.ResourceTree(ctx, &application.ResourcesQuery{
		ApplicationName: &applicationName,
		Project:         &applicationProject,
		AppNamespace:    &applicationNamespace,
	})
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (a *applicationServiceImpl) GetManagedResourceManifests(
	query *application.ResourcesQuery,
) (*application.ManagedResourcesResponse, error) {
	a.logger.With(
		slog.String("application", utilities.QualifiedName(*query.ApplicationName, *query.AppNamespace)),
		slog.String("resource", *query.Name),
	).Debug("Getting Argo CD application resource manifests")

	ctx := WithArgoProject(a.context, *query.Project)
	res, err := a.client.ManagedResources(ctx, query)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (a *applicationServiceImpl) GetResourceLiveManifest(
	query *application.ApplicationResourceRequest,
) (*application.ApplicationResourceResponse, error) {
	a.logger.With(
		slog.String("application", utilities.QualifiedName(*query.Name, *query.AppNamespace)),
		slog.String("resource", *query.ResourceName),
	).Debug("Getting Argo CD application resource live manifests")

	ctx := WithArgoProject(a.context, *query.Project)
	res, err := a.client.GetResource(ctx, query)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (a *applicationServiceImpl) GetResourceEvents(
	query *application.ApplicationResourceEventsQuery,
) (*v1.EventList, error) {
	a.logger.With(
		slog.String("application", utilities.QualifiedName(*query.Name, *query.AppNamespace)),
		slog.String("resource", *query.ResourceName),
	).Debug("Getting Argo CD resource events")

	ctx := WithArgoProject(a.context, *query.Project)
	res, err := a.client.ListResourceEvents(ctx, query)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (a *applicationServiceImpl) GetPodLogs(
	query *application.ApplicationPodLogsQuery,
) (application.ApplicationService_PodLogsClient, error) {
	a.logger.With(
		slog.String("application", utilities.QualifiedName(*query.Name, *query.AppNamespace)),
		slog.String("pod", *query.PodName),
	).Debug("Getting Argo CD pod logs")

	ctx := WithArgoProject(a.context, *query.Project)
	res, err := a.client.PodLogs(ctx, query)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (a *applicationServiceImpl) SyncApplication(
	applicationProject,
	applicationName,
	applicationNamespace string,
	opts *SyncOptions,
) (*v1alpha1.Application, error) {
	a.logger.With(
		slog.String("application", utilities.QualifiedName(applicationName, applicationNamespace)),
		slog.String("project", applicationProject),
	).Debug("Syncing Argo CD Application")
	req := &application.ApplicationSyncRequest{
		Name:         &applicationName,
		AppNamespace: &applicationNamespace,
		Project:      &applicationProject,
	}

	if opts != nil {
		req.Prune = &opts.Prune
		if opts.Revision != "" {
			req.Revision = &opts.Revision
		}
	}

	ctx := WithArgoProject(a.context, applicationProject)
	return a.client.Sync(ctx, req)
}
