package octopus

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/OctopusDeploy/octopus-grpc/go/pkg/connection"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"

	certpool "github.com/OctopusDeploy/octopus-argocd-gateway/internal/certificate"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/communication"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/metrics"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
)

type Config struct {
	ServerURL       string
	ClientID        string
	BearerToken     string
	TLS             TLSConfig
	CertificatePath string
}

type TLSConfig struct {
	Plaintext      bool
	Thumbprint     string
	PinCertificate bool
}

type Client interface {
	Connect()
	Close() error
	State() connectivity.State
	WaitForReady(ctx context.Context) error
	RawConnection() *grpc.ClientConn

	ApplicationEvents() pb.ApplicationEventServiceClient
	ApplicationManifests() pb.ApplicationManifestsServiceClient
	ApplicationResources() pb.ApplicationResourceServiceClient
	ApplicationResourceTree() pb.ApplicationResourceTreeServiceClient
	ApplicationSync() pb.ApplicationSyncServiceClient
	Info() pb.InfoServiceClient
	PodLogs() pb.PodLogsServiceClient
	PruneApplications() pb.PruneApplicationsServiceClient
	ResourceEvents() pb.ResourceEventsServiceClient
	Connection() connection.Connection

	ManifestsStreamSource() runner.BidirectionalStreamSource[*pb.GetResourceManifestsRequest, *pb.GetResourceManifestsResponse]
	ResourceEventsStreamSource() runner.BidirectionalStreamSource[*pb.GetResourceEventsRequest, *pb.GetResourceEventsResponse]
	PodLogsStreamSource() runner.BidirectionalStreamSource[*pb.GetPodLogsRequest, *pb.GetPodLogsResponse]
	InfoStreamSource() runner.BidirectionalStreamSource[*pb.GetInfoRequest, *pb.GetInfoResponse]
	GatewayLogsStreamSource() runner.BidirectionalStreamSource[*pb.GetGatewayLogsRequest, *pb.GetGatewayLogsResponse]
	ApplicationResourcesStreamSource() runner.BidirectionalStreamSource[*pb.WatchApplicationResourcesRequest, *pb.WatchApplicationResourcesResponse]
	SyncStreamSource() runner.BidirectionalStreamSource[*pb.SyncApplicationRequest, *pb.SyncApplicationResponse]
	ApplicationResourceTreeStreamSource() runner.BidirectionalStreamSource[*pb.GetApplicationResourceTreeRequest, *pb.GetApplicationResourceTreeResponse]
}

// client builds its service stubs per call rather than holding them. A stub
// captures the ClientConn it was built with, so one kept across a rebuild would go
// on talking to a connection that has already been closed.
type client struct {
	conn connection.Connection
}

func NewClient(ctx context.Context, logger *slog.Logger, cfg Config) (Client, error) {
	rootCertificates, err := certpool.GetRootCertificatePool()
	if err != nil {
		logger.Warn("Failed to load root certificate pool", slog.Any("error", err))
	}

	conn, err := connection.New(connection.Config{
		ServerURL:   communication.SanitizeGrpcUrl(cfg.ServerURL),
		Credentials: bearerCredentials(cfg),
		TLS: connection.TLSConfig{
			Plaintext:      cfg.TLS.Plaintext,
			Thumbprint:     cfg.TLS.Thumbprint,
			PinCertificate: cfg.TLS.PinCertificate,
			RootCAs:        rootCertificates,
		},
		CallOptions: []grpc.CallOption{
			grpc.MaxCallSendMsgSize(math.MaxInt32),
			grpc.MaxCallRecvMsgSize(math.MaxInt32),
		},
	}, logger)
	if err != nil {
		return nil, err
	}

	return &client{
		conn: conn,
	}, nil
}

func (c *client) Connect() {
	c.conn.Get().Connect()
}

func (c *client) Close() error {
	return c.conn.Close()
}

func (c *client) State() connectivity.State {
	return c.conn.Get().GetState()
}

func (c *client) WaitForReady(ctx context.Context) error {
	return communication.WaitForReady(ctx, c.conn.Get())
}

func (c *client) RawConnection() *grpc.ClientConn {
	return c.conn.Get()
}

func (c *client) ApplicationEvents() pb.ApplicationEventServiceClient {
	return pb.NewApplicationEventServiceClient(c.conn.Get())
}

func (c *client) ApplicationManifests() pb.ApplicationManifestsServiceClient {
	return pb.NewApplicationManifestsServiceClient(c.conn.Get())
}

func (c *client) ApplicationResources() pb.ApplicationResourceServiceClient {
	return pb.NewApplicationResourceServiceClient(c.conn.Get())
}

func (c *client) ApplicationResourceTree() pb.ApplicationResourceTreeServiceClient {
	return pb.NewApplicationResourceTreeServiceClient(c.conn.Get())
}

func (c *client) Info() pb.InfoServiceClient {
	return pb.NewInfoServiceClient(c.conn.Get())
}

func (c *client) PodLogs() pb.PodLogsServiceClient {
	return pb.NewPodLogsServiceClient(c.conn.Get())
}

func (c *client) PruneApplications() pb.PruneApplicationsServiceClient {
	return pb.NewPruneApplicationsServiceClient(c.conn.Get())
}

func (c *client) ResourceEvents() pb.ResourceEventsServiceClient {
	return pb.NewResourceEventsServiceClient(c.conn.Get())
}

func (c *client) ApplicationSync() pb.ApplicationSyncServiceClient {
	return pb.NewApplicationSyncServiceClient(c.conn.Get())
}

func (c *client) Connection() connection.Connection {
	return c.conn
}

func (c *client) ManifestsStreamSource() runner.BidirectionalStreamSource[*pb.GetResourceManifestsRequest, *pb.GetResourceManifestsResponse] {
	return &manifestsStreamSource{
		client:      c.ApplicationManifests(),
		errorPolicy: DefaultErrorPolicy,
	}
}

func (c *client) ResourceEventsStreamSource() runner.BidirectionalStreamSource[*pb.GetResourceEventsRequest, *pb.GetResourceEventsResponse] {
	return &resourceEventsStreamSource{
		client:      c.ResourceEvents(),
		errorPolicy: DefaultErrorPolicy,
	}
}

func (c *client) PodLogsStreamSource() runner.BidirectionalStreamSource[*pb.GetPodLogsRequest, *pb.GetPodLogsResponse] {
	return &podLogsStreamSource{
		client:      c.PodLogs(),
		errorPolicy: DefaultErrorPolicy,
	}
}

func (c *client) InfoStreamSource() runner.BidirectionalStreamSource[*pb.GetInfoRequest, *pb.GetInfoResponse] {
	return &infoStreamSource{
		client:      c.Info(),
		errorPolicy: DefaultErrorPolicy,
	}
}

func (c *client) GatewayLogsStreamSource() runner.BidirectionalStreamSource[*pb.GetGatewayLogsRequest, *pb.GetGatewayLogsResponse] {
	return &gatewayLogsStreamSource{
		client:      c.Info(),
		errorPolicy: DefaultErrorPolicy,
	}
}

func (c *client) ApplicationResourcesStreamSource() runner.BidirectionalStreamSource[*pb.WatchApplicationResourcesRequest, *pb.WatchApplicationResourcesResponse] {
	return &applicationResourcesStreamSource{
		client:      c.ApplicationResources(),
		errorPolicy: DefaultErrorPolicy,
	}
}

func (c *client) SyncStreamSource() runner.BidirectionalStreamSource[*pb.SyncApplicationRequest, *pb.SyncApplicationResponse] {
	return &syncStreamSource{
		client:      c.ApplicationSync(),
		errorPolicy: DefaultErrorPolicy,
	}
}

type (
	Runner       = runner.Runner
	RunnerOption = runner.RunnerOption
)

func WithSilentUnimplemented() RunnerOption {
	return runner.WithSilentUnimplemented()
}

func WithRetryBackoff(d time.Duration) RunnerOption {
	return runner.WithRetryBackoff(d)
}

func WithMetrics(m metrics.Metrics, handlerName string) RunnerOption {
	return runner.WithMetrics(m, handlerName)
}

type manifestsStreamSource struct {
	client      pb.ApplicationManifestsServiceClient
	errorPolicy *ErrorPolicy
}

func (s *manifestsStreamSource) CreateStream(
	ctx context.Context,
) (runner.BidirectionalStream[*pb.GetResourceManifestsRequest, *pb.GetResourceManifestsResponse], error) {
	return s.client.GetResourceManifests(ctx)
}

func (s *manifestsStreamSource) ErrorPolicy() runner.ErrorPolicy {
	return s.errorPolicy
}

type resourceEventsStreamSource struct {
	client      pb.ResourceEventsServiceClient
	errorPolicy *ErrorPolicy
}

func (s *resourceEventsStreamSource) CreateStream(
	ctx context.Context,
) (runner.BidirectionalStream[*pb.GetResourceEventsRequest, *pb.GetResourceEventsResponse], error) {
	return s.client.GetResourceEvents(ctx)
}

func (s *resourceEventsStreamSource) ErrorPolicy() runner.ErrorPolicy {
	return s.errorPolicy
}

type podLogsStreamSource struct {
	client      pb.PodLogsServiceClient
	errorPolicy *ErrorPolicy
}

func (s *podLogsStreamSource) CreateStream(
	ctx context.Context,
) (runner.BidirectionalStream[*pb.GetPodLogsRequest, *pb.GetPodLogsResponse], error) {
	return s.client.GetPodLogs(ctx)
}

func (s *podLogsStreamSource) ErrorPolicy() runner.ErrorPolicy {
	return s.errorPolicy
}

type infoStreamSource struct {
	client      pb.InfoServiceClient
	errorPolicy *ErrorPolicy
}

func (s *infoStreamSource) CreateStream(
	ctx context.Context,
) (runner.BidirectionalStream[*pb.GetInfoRequest, *pb.GetInfoResponse], error) {
	return s.client.GetInfo(ctx)
}

func (s *infoStreamSource) ErrorPolicy() runner.ErrorPolicy {
	return s.errorPolicy
}

type gatewayLogsStreamSource struct {
	client      pb.InfoServiceClient
	errorPolicy *ErrorPolicy
}

func (s *gatewayLogsStreamSource) CreateStream(
	ctx context.Context,
) (runner.BidirectionalStream[*pb.GetGatewayLogsRequest, *pb.GetGatewayLogsResponse], error) {
	return s.client.GetGatewayLogs(ctx)
}

func (s *gatewayLogsStreamSource) ErrorPolicy() runner.ErrorPolicy {
	return s.errorPolicy
}

type applicationResourcesStreamSource struct {
	client      pb.ApplicationResourceServiceClient
	errorPolicy *ErrorPolicy
}

func (s *applicationResourcesStreamSource) CreateStream(
	ctx context.Context,
) (runner.BidirectionalStream[*pb.WatchApplicationResourcesRequest, *pb.WatchApplicationResourcesResponse], error) {
	return s.client.WatchApplicationResources(ctx)
}

func (s *applicationResourcesStreamSource) ErrorPolicy() runner.ErrorPolicy {
	return s.errorPolicy
}

type syncStreamSource struct {
	client      pb.ApplicationSyncServiceClient
	errorPolicy *ErrorPolicy
}

func (s *syncStreamSource) CreateStream(
	ctx context.Context,
) (runner.BidirectionalStream[*pb.SyncApplicationRequest, *pb.SyncApplicationResponse], error) {
	return s.client.SyncApplication(ctx)
}

func (s *syncStreamSource) ErrorPolicy() runner.ErrorPolicy {
	return s.errorPolicy
}

func (c *client) ApplicationResourceTreeStreamSource() runner.BidirectionalStreamSource[*pb.GetApplicationResourceTreeRequest, *pb.GetApplicationResourceTreeResponse] {
	return &applicationResourceTreeStreamSource{
		client:      c.ApplicationResourceTree(),
		errorPolicy: DefaultErrorPolicy,
	}
}

type applicationResourceTreeStreamSource struct {
	client      pb.ApplicationResourceTreeServiceClient
	errorPolicy *ErrorPolicy
}

func (s *applicationResourceTreeStreamSource) CreateStream(
	ctx context.Context,
) (runner.BidirectionalStream[*pb.GetApplicationResourceTreeRequest, *pb.GetApplicationResourceTreeResponse], error) {
	return s.client.GetApplicationResourceTree(ctx)
}

func (s *applicationResourceTreeStreamSource) ErrorPolicy() runner.ErrorPolicy {
	return s.errorPolicy
}

func bearerCredentials(cfg Config) credentials.PerRPCCredentials {
	if cfg.TLS.Plaintext {
		return connection.PlaintextBearerCredentials{ClientID: cfg.ClientID, Token: cfg.BearerToken}
	}

	return connection.BearerCredentials{ClientID: cfg.ClientID, Token: cfg.BearerToken}
}
