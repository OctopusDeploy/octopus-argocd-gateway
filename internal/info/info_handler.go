package info

import (
	"context"
	"log/slog"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/configuration"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopus"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/utilities"
)

const nonK8sFallback = "local-non-k8s"

type Handler struct {
	logger         *slog.Logger
	ctx            context.Context
	versionService argocd.VersionService
	config         *configuration.Configuration
	clock          utilities.Clock
}

func NewRunner(
	logger *slog.Logger,
	ctx context.Context,
	octopusClient octopus.Client,
	argoClient argocd.Client,
	config *configuration.Configuration,
	opts ...octopus.RunnerOption,
) octopus.Runner {
	handler := &Handler{
		logger:         logger,
		ctx:            ctx,
		versionService: argoClient.Version(),
		config:         config,
		clock:          utilities.NewClock(),
	}
	streamSource := octopusClient.InfoStreamSource()
	return runner.NewStreamRunner(streamSource, handler, logger, ctx, opts...)
}

func (h *Handler) ProcessRequest(request *pb.GetInfoRequest) (*pb.GetInfoResponse, error) {
	h.logger.InfoContext(h.ctx, "Received get info request")

	// Re-run the same auth-token validation we do at startup so operators see
	// expired / near-expiry tokens surfaced via the periodic info request,
	// not just in the boot log.
	report := argocd.ValidateAuthTokens(
		h.config.Gateway.ArgoCdAuthToken,
		h.config.Gateway.ArgoCdProjectAuthTokens,
		argocd.DefaultTokenExpiryWarningThreshold,
		h.clock,
	)
	argocd.LogValidationReport(h.logger, report)

	argoCDVersionMsg, err := h.versionService.GetVersion()

	canConnectToArgo := true
	argoCdError := ""
	argoCDVersion := ""

	if err != nil {
		h.logger.With(slog.Any("error", err)).Error("Error connecting to Argo CD instance")
		canConnectToArgo = false
		argoCdError = err.Error()
	}

	h.logger.InfoContext(h.ctx, "Sending get info response")

	if argoCDVersionMsg != nil {
		argoCDVersion = argoCDVersionMsg.Version
	}

	return &pb.GetInfoResponse{
		RequestId:             request.RequestId,
		GatewayVersion:        utilities.LookupEnvOrDefault("GATEWAY_VERSION", "0.0.1-local"),
		HelmChartVersion:      utilities.LookupEnvOrDefault("HELM_CHART_VERSION", "0.0.1-local"),
		HelmReleaseName:       utilities.LookupEnvOrDefault("HELM_RELEASE_NAME", "local"),
		KubernetesNamespace:   utilities.LookupEnvOrDefault("KUBERNETES_NAMESPACE", nonK8sFallback),
		CanConnectToArgoCd:    canConnectToArgo,
		ArgoCdConnectionError: argoCdError,
		ArgoCdVersion:         argoCDVersion,
	}, nil
}
