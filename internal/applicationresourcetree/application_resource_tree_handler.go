package applicationresourcetree

import (
	"context"
	"log/slog"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopus"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
)

type applicationService interface {
	GetApplicationResourceTree(project, name, namespace string) (*v1alpha1.ApplicationTree, error)
}

type Handler struct {
	logger             *slog.Logger
	applicationService applicationService
}

func NewRunner(
	logger *slog.Logger,
	ctx context.Context,
	octopusClient octopus.Client,
	argoClient argocd.Client,
	opts ...octopus.RunnerOption,
) octopus.Runner {
	handler := &Handler{
		logger:             logger,
		applicationService: argoClient.Applications(),
	}
	streamSource := octopusClient.ApplicationResourceTreeStreamSource()
	return runner.NewStreamRunner(streamSource, handler, logger, ctx, opts...)
}

func (h *Handler) ProcessRequest(
	request *pb.GetApplicationResourceTreeRequest,
) (*pb.GetApplicationResourceTreeResponse, error) {
	tree, err := h.applicationService.GetApplicationResourceTree(
		request.ApplicationProject,
		request.ApplicationName,
		request.ApplicationNamespace,
	)
	if err != nil {
		h.logger.With(slog.Any("error", err), slog.Any("request", request)).
			Error("Failed to get application resource tree")
		return &pb.GetApplicationResourceTreeResponse{
			RequestId: request.RequestId,
			RequestError: &pb.RequestError{
				Message: err.Error(),
				Code:    pb.ErrorCode_ERROR_CODE_UNEXPECTED,
			},
		}, nil
	}

	nodes := mapNodes(tree.Nodes)

	return &pb.GetApplicationResourceTreeResponse{
		RequestId: request.RequestId,
		Nodes:     nodes,
	}, nil
}

func mapNodes(nodes []v1alpha1.ResourceNode) []*pb.ResourceTreeNode {
	result := make([]*pb.ResourceTreeNode, 0, len(nodes))
	for _, node := range nodes {
		mapped := &pb.ResourceTreeNode{
			Uid:       node.UID,
			Name:      node.Name,
			Namespace: node.Namespace,
			Group:     node.Group,
			Version:   node.Version,
			Kind:      node.Kind,
			ParentUid: getParentUID(node),
		}
		if node.Health != nil {
			mapped.HealthStatusCode = string(node.Health.Status)
			mapped.HealthStatusMessage = node.Health.Message
		}
		result = append(result, mapped)
	}
	return result
}

// getParentUID returns a single parent UID for the node. Argo CD resource nodes
// can technically reference multiple parents, but we assume a tree structure here
// and report only the first non-empty parent reference.
func getParentUID(node v1alpha1.ResourceNode) string {
	for _, ref := range node.ParentRefs {
		if ref.UID != "" {
			return ref.UID
		}
	}
	return ""
}
