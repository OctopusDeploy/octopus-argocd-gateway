package argocd

import (
	"context"
	"log/slog"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"google.golang.org/grpc"
)

type ProjectService interface {
	// GetProject looks up an AppProject by name. The caller must supply a context
	// wrapped via WithArgoProject when project-scoped tokens are in use, otherwise
	// the auth interceptor cannot pick the right token.
	GetProject(ctx context.Context, name string) (*v1alpha1.AppProject, error)
}

type projectServiceImpl struct {
	logger   *slog.Logger
	context  context.Context
	grpcConn *grpc.ClientConn
	client   project.ProjectServiceClient
}

func newProjectService(
	ctx context.Context,
	logger *slog.Logger,
	grpcConn *grpc.ClientConn,
) ProjectService {
	return &projectServiceImpl{
		logger:   logger,
		context:  ctx,
		grpcConn: grpcConn,
		client:   project.NewProjectServiceClient(grpcConn),
	}
}

func (p *projectServiceImpl) GetProject(
	ctx context.Context,
	name string,
) (*v1alpha1.AppProject, error) {
	p.logger.With(slog.String("project", name)).Debug("Getting Argo CD Project")

	result, err := p.client.Get(ctx, &project.ProjectQuery{Name: name})
	if err != nil {
		return nil, WrapRequestError(err, "get project")
	}

	return result, nil
}
