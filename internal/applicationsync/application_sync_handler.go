package applicationsync

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
	GetApplication(project, name, namespace string) (*v1alpha1.Application, error)
	SyncApplication(project, name, namespace string, opts *argocd.SyncOptions) (*v1alpha1.Application, error)
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
	streamSource := octopusClient.SyncStreamSource()
	return runner.NewStreamRunner(streamSource, handler, logger, ctx, opts...)
}

func (h *Handler) ProcessRequest(request *pb.SyncApplicationRequest) (*pb.SyncApplicationResponse, error) {
	logger := h.logger.With(
		slog.String("application", request.QualifiedName()),
		slog.String("project", request.ApplicationProject),
	)
	logger.Info("Processing sync request")

	app, err := h.applicationService.GetApplication(
		request.ApplicationProject,
		request.ApplicationName,
		request.ApplicationNamespace,
	)
	if err != nil {
		logger.Error("Failed to get application", slog.Any("error", err))
		return &pb.SyncApplicationResponse{
			RequestId: request.RequestId,
			RequestError: &pb.RequestError{
				Message: err.Error(),
				Code:    pb.ErrorCode_ERROR_CODE_UNEXPECTED,
			},
		}, nil
	}

	syncOpts := extractSyncOptions(app)

	_, err = h.applicationService.SyncApplication(
		request.ApplicationProject,
		request.ApplicationName,
		request.ApplicationNamespace,
		syncOpts,
	)
	if err != nil {
		logger.Error("Failed to sync application", slog.Any("error", err))
		return &pb.SyncApplicationResponse{
			RequestId: request.RequestId,
			RequestError: &pb.RequestError{
				Message: err.Error(),
				Code:    pb.ErrorCode_ERROR_CODE_UNEXPECTED,
			},
		}, nil
	}

	return buildSyncResponse(request.RequestId), nil
}
