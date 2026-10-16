package argocd

import (
	"context"
	"log/slog"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/cluster"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"google.golang.org/grpc"
)

type ClusterService interface {
	// GetCluster looks up a cluster. The caller must supply a context wrapped
	// via WithArgoProject when project-scoped tokens are in use, otherwise
	// the auth interceptor cannot pick the right token.
	GetCluster(ctx context.Context, clusterId string, idType ClusterIdType) (*v1alpha1.Cluster, error)
}

type clusterServiceImpl struct {
	logger   *slog.Logger
	context  context.Context
	grpcConn *grpc.ClientConn
	client   cluster.ClusterServiceClient
}

type ClusterIdType string

const (
	ClusterIdServer ClusterIdType = "server"
	ClusterIdName   ClusterIdType = "name"
)

func newClusterService(
	ctx context.Context,
	logger *slog.Logger,
	grpcConn *grpc.ClientConn,
) ClusterService {
	return &clusterServiceImpl{
		logger:   logger,
		context:  ctx,
		grpcConn: grpcConn,
		client:   cluster.NewClusterServiceClient(grpcConn),
	}
}

func (c *clusterServiceImpl) GetCluster(
	ctx context.Context,
	clusterId string,
	idType ClusterIdType,
) (*v1alpha1.Cluster, error) {
	c.logger.With(
		slog.String("clusterId", clusterId),
		slog.Any("clusterType", idType),
	).Debug("Getting Argo CD Cluster")

	result, err := c.client.Get(ctx, &cluster.ClusterQuery{
		Id: &cluster.ClusterID{
			Type:  string(idType),
			Value: clusterId,
		},
	})
	if err != nil {
		return nil, WrapRequestError(err, "get cluster")
	}

	return result, nil
}
