package argocd

import (
	"context"
	"log/slog"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/version"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type VersionService interface {
	GetVersion() (*version.VersionMessage, error)
}

type versionServiceImpl struct {
	logger   *slog.Logger
	context  context.Context
	grpcConn *grpc.ClientConn
	client   version.VersionServiceClient
}

func newVersionService(
	ctx context.Context,
	logger *slog.Logger,
	grpcConn *grpc.ClientConn,
) VersionService {
	return &versionServiceImpl{
		logger:   logger,
		context:  ctx,
		grpcConn: grpcConn,
		client:   version.NewVersionServiceClient(grpcConn),
	}
}

func (v *versionServiceImpl) GetVersion() (*version.VersionMessage, error) {
	v.logger.Debug("Get Argo CD version")
	ctx := WithArgoProject(v.context, UnscopedProjectAuthToken)

	versionResponse, err := v.client.Version(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}

	return versionResponse, nil
}
