package podlogs

import (
	"context"
	"log/slog"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopus"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/utilities"
)

type applicationService interface {
	GetPodLogs(query *application.ApplicationPodLogsQuery) (application.ApplicationService_PodLogsClient, error)
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
	streamSource := octopusClient.PodLogsStreamSource()
	return runner.NewStreamRunner(streamSource, handler, logger, ctx, opts...)
}

func (h *Handler) ProcessRequest(request *pb.GetPodLogsRequest) (*pb.GetPodLogsResponse, error) {
	podLogsService, err := h.applicationService.GetPodLogs(&application.ApplicationPodLogsQuery{
		Name:         &request.ApplicationName,
		AppNamespace: &request.ApplicationNamespace,
		Project:      &request.ApplicationProject,

		Namespace: &request.ResourceNamespace,
		PodName:   &request.PodName,
		Container: &request.ContainerName,

		SinceSeconds: utilities.Pointer(int64(0)),
		Previous:     &request.PreviousContainer,
		TailLines:    utilities.Pointer(int64(1000)),
		Follow:       utilities.Pointer(false),
	})
	if err != nil {
		h.logger.With(slog.Any("err", err), slog.Any("request", request)).
			Error("Failed to get pod logs client")
		return &pb.GetPodLogsResponse{
			RequestId: request.RequestId,
			RequestError: &pb.RequestError{
				Message: err.Error(),
				Code:    pb.ErrorCode_ERROR_CODE_UNEXPECTED,
			},
		}, nil
	}

	logs, receivedLogCount := h.processPodLogs(podLogsService)

	if receivedLogCount != len(logs) {
		logParseFailureMessage := "Failed to parse log lines"
		h.logger.With(slog.Int("expectedLogCount", receivedLogCount), slog.Int("actualLogCount", len(logs))).
			Error(logParseFailureMessage)
		return &pb.GetPodLogsResponse{
			RequestId: request.RequestId,
			RequestError: &pb.RequestError{
				Message: logParseFailureMessage,
				Code:    pb.ErrorCode_ERROR_CODE_UNEXPECTED,
			},
		}, nil
	}

	return &pb.GetPodLogsResponse{
		RequestId: request.RequestId,
		Entries:   logs,
	}, nil
}

func (h *Handler) processPodLogs(podLogsService application.ApplicationService_PodLogsClient) ([]*pb.PodLogEntry, int) {
	var logs []*pb.PodLogEntry
	receivedLogCount := 0

	for {
		logMsg, logErr := podLogsService.Recv()

		if logErr != nil {
			h.logger.With(slog.Any("err", logErr)).Error("Failed to get pod log entries")
			break
		}

		if logMsg.GetLast() {
			break
		}
		receivedLogCount += 1

		logTimestamp, err := time.Parse(time.RFC3339Nano, logMsg.GetTimeStampStr())
		if err != nil {
			h.logger.With(slog.Any("err", err), slog.Any("logTimestamp", logMsg.GetTimeStampStr())).
				Error("Failed to parse pod log timestamp")
			continue
		}

		logs = append(logs, &pb.PodLogEntry{
			Message:   logMsg.GetContent(),
			Timestamp: timestamppb.New(logTimestamp),
		})
	}

	return logs, receivedLogCount
}
