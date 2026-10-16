package info

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopus"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/utilities"
)

type GatewayPodLogsHandler struct {
	logger *slog.Logger
	ctx    context.Context
}

func NewGatewayPodLogsRunner(
	logger *slog.Logger,
	ctx context.Context,
	octopusClient octopus.Client,
	opts ...octopus.RunnerOption,
) octopus.Runner {
	handler := &GatewayPodLogsHandler{
		logger: logger,
		ctx:    ctx,
	}
	streamSource := octopusClient.GatewayLogsStreamSource()
	return runner.NewStreamRunner(streamSource, handler, logger, ctx, opts...)
}

func (h *GatewayPodLogsHandler) ProcessRequest(request *pb.GetGatewayLogsRequest) (*pb.GetGatewayLogsResponse, error) {
	h.logger.InfoContext(h.ctx, "Received get gateway logs request")

	namespace := utilities.LookupEnvOrDefault("KUBERNETES_NAMESPACE", nonK8sFallback)
	podName := utilities.LookupEnvOrDefault("POD_NAME", nonK8sFallback)

	var logLines []LogLine
	var err error
	if namespace == nonK8sFallback {
		logLines = make([]LogLine, 1)
		logLines[0] = LogLine{
			Message:   "The gateway is running outside of Kubernetes, so no logs can be returned.",
			Timestamp: time.Now(),
		}
	} else {
		logLines, err = getGatewayPodLogs(h.ctx, h.logger, podName, namespace, request.PreviousContainer)
	}

	if err != nil {
		h.logger.With(slog.Any("error", err)).Error("Failed to load gateway logs")
		return &pb.GetGatewayLogsResponse{
			RequestId: request.RequestId,
			RequestError: &pb.RequestError{
				Message: err.Error(),
				Code:    pb.ErrorCode_ERROR_CODE_UNEXPECTED,
			},
		}, nil
	}

	podLogEntries := mapLogLines(logLines)
	h.logger.InfoContext(h.ctx, "Sent get gateway logs response")
	return &pb.GetGatewayLogsResponse{
		RequestId: request.RequestId,
		Entries:   podLogEntries,
	}, nil
}

func mapLogLines(logLines []LogLine) []*pb.PodLogEntry {
	podLogs := make([]*pb.PodLogEntry, len(logLines))

	for i, logLine := range logLines {
		podLogs[i] = &pb.PodLogEntry{
			Message:   logLine.Message,
			Timestamp: timestamppb.New(logLine.Timestamp),
		}
	}

	return podLogs
}
