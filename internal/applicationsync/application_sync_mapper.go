package applicationsync

import (
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
)

func extractSyncOptions(app *v1alpha1.Application) *argocd.SyncOptions {
	if app == nil || app.Spec.SyncPolicy == nil {
		return &argocd.SyncOptions{}
	}

	opts := &argocd.SyncOptions{}

	if app.Spec.SyncPolicy.Automated != nil {
		opts.Prune = app.Spec.SyncPolicy.Automated.Prune
	}

	return opts
}

func buildSyncResponse(requestID string) *pb.SyncApplicationResponse {
	return &pb.SyncApplicationResponse{
		RequestId: requestID,
	}
}
