package resourceevents

import (
	"context"
	"log/slog"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "k8s.io/api/core/v1"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopus"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
)

type applicationService interface {
	GetResourceEvents(query *application.ApplicationResourceEventsQuery) (*v1.EventList, error)
}

type Handler struct {
	logger             *slog.Logger
	ctx                context.Context
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
		ctx:                ctx,
		applicationService: argoClient.Applications(),
	}
	streamSource := octopusClient.ResourceEventsStreamSource()
	return runner.NewStreamRunner(streamSource, handler, logger, ctx, opts...)
}

func (h *Handler) ProcessRequest(request *pb.GetResourceEventsRequest) (*pb.GetResourceEventsResponse, error) {
	eventList, err := h.applicationService.GetResourceEvents(&application.ApplicationResourceEventsQuery{
		Name:         &request.ApplicationName,
		AppNamespace: &request.ApplicationNamespace,
		Project:      &request.ApplicationProject,

		ResourceNamespace: &request.ResourceNamespace,
		ResourceName:      &request.ResourceName,
		ResourceUID:       &request.ResourceUid,
	})
	if err != nil {
		h.logger.With(
			slog.String("application", request.QualifiedName()),
			slog.String("resource", request.ResourceName),
			slog.Any("error", err),
		).Error("Failed to get resource events entries")
		return &pb.GetResourceEventsResponse{
			RequestId: request.RequestId,
			RequestError: &pb.RequestError{
				Message: err.Error(),
				Code:    pb.ErrorCode_ERROR_CODE_UNEXPECTED,
			},
		}, nil
	}

	eventItems := mapEvents(eventList)
	return &pb.GetResourceEventsResponse{
		RequestId: request.RequestId,
		Items:     eventItems,
	}, nil
}

func mapEvents(eventList *v1.EventList) []*pb.EventItem {
	eventItems := make([]*pb.EventItem, len(eventList.Items))
	for i := 0; i < len(eventList.Items); i++ {
		event := eventList.Items[i]

		var firstTime, lastTime time.Time
		if event.FirstTimestamp.IsZero() {
			firstTime = event.EventTime.Time
		} else {
			firstTime = event.FirstTimestamp.Time
		}
		if event.LastTimestamp.IsZero() {
			lastTime = event.EventTime.Time
		} else {
			lastTime = event.LastTimestamp.Time
		}

		var count int32
		if event.Count == 0 {
			count = 1
		} else {
			count = event.Count
		}

		eventItems[i] = &pb.EventItem{
			FirstObservedTime:   timestamppb.New(firstTime),
			LastObservedTime:    timestamppb.New(lastTime),
			Count:               count,
			Action:              event.Action,
			Reason:              event.Reason,
			Note:                event.Message,
			ReportingController: event.ReportingController,
			ReportingInstance:   event.ReportingInstance,
			Type:                event.Type,
		}
	}

	return eventItems
}
