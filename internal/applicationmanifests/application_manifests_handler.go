package applicationmanifests

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopus"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
)

type applicationService interface {
	GetManagedResourceManifests(query *application.ResourcesQuery) (*application.ManagedResourcesResponse, error)
	GetResourceLiveManifest(
		query *application.ApplicationResourceRequest,
	) (*application.ApplicationResourceResponse, error)
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
	streamSource := octopusClient.ManifestsStreamSource()
	return runner.NewStreamRunner(streamSource, handler, logger, ctx, opts...)
}

func (h *Handler) ProcessRequest(request *pb.GetResourceManifestsRequest) (*pb.GetResourceManifestsResponse, error) {
	var response *pb.GetResourceManifestsResponse
	var err error

	logger := h.logger.With(
		slog.Any("resourceType", request.ResourceType),
		slog.String("resourceName", request.ResourceName),
		slog.String("applicationName", request.QualifiedName()),
	)
	switch request.ResourceType {
	case pb.ResourceType_RESOURCE_TYPE_APPLIED:
		response, err = h.handleAppliedResource(request)
	case pb.ResourceType_RESOURCE_TYPE_CHILD:
		response, err = h.handleChildResource(request)
	default:
		logger.Info("Unsupported resource type, attempting to get manifest")
		response, err = h.handleAppliedResource(request)

		if response != nil {
			return response, nil
		}

		if err != nil {
			logger.With(slog.Any("error", err)).Error("Error getting manifests for unknown resource type")
		}
		response, err = h.handleChildResource(request)
	}

	if err != nil {
		h.logger.With(slog.Any("error", err), slog.Any("request", request)).
			Error("Failed to get resource manifests")
		return &pb.GetResourceManifestsResponse{
			RequestId: request.RequestId,
			RequestError: &pb.RequestError{
				Message: err.Error(),
				Code:    pb.ErrorCode_ERROR_CODE_UNEXPECTED,
			},
		}, nil
	}

	return response, nil
}

func (h *Handler) handleAppliedResource(
	request *pb.GetResourceManifestsRequest,
) (*pb.GetResourceManifestsResponse, error) {
	res, err := h.applicationService.GetManagedResourceManifests(&application.ResourcesQuery{
		ApplicationName: &request.ApplicationName,
		AppNamespace:    &request.ApplicationNamespace,
		Project:         &request.ApplicationProject,
		Namespace:       &request.ResourceNamespace,
		Name:            &request.ResourceName,
		Version:         &request.ResourceVersion,
		Group:           &request.ResourceGroup,
		Kind:            &request.ResourceKind,
	})
	if err != nil {
		return nil, err
	}

	if len(res.Items) != 1 {
		return nil, fmt.Errorf("expected 1 resource manifest, got %d", len(res.Items))
	}

	resourceManifests := res.Items[0]
	return MapManagedManifests(request.RequestId, resourceManifests)
}

func (h *Handler) handleChildResource(
	request *pb.GetResourceManifestsRequest,
) (*pb.GetResourceManifestsResponse, error) {
	resourceManifest, err := h.applicationService.GetResourceLiveManifest(&application.ApplicationResourceRequest{
		AppNamespace: &request.ApplicationNamespace,
		Name:         &request.ApplicationName,
		Project:      &request.ApplicationProject,
		Namespace:    &request.ResourceNamespace,
		ResourceName: &request.ResourceName,
		Version:      &request.ResourceVersion,
		Group:        &request.ResourceGroup,
		Kind:         &request.ResourceKind,
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetResourceManifestsResponse{
		RequestId:    request.RequestId,
		LiveManifest: *resourceManifest.Manifest,
	}, nil
}
