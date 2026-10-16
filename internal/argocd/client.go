package argocd

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"net"
	"net/http"
	"slices"
	"sync"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/certificate"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/communication"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
)

type rpcAuth struct {
	bearerToken string
}

func (r rpcAuth) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", r.bearerToken),
	}, nil
}

func (r rpcAuth) RequireTransportSecurity() bool {
	return true
}

type plaintextRpcAuth struct {
	bearerToken string
}

func (r plaintextRpcAuth) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", r.bearerToken),
	}, nil
}

func (r plaintextRpcAuth) RequireTransportSecurity() bool {
	return false
}

type Config struct {
	ServerURL         string
	AuthToken         string
	ProjectAuthTokens map[string]string
	TLS               TLSConfig
	GRPCWeb           bool
	GRPCWebRootPath   string
}

type TLSConfig struct {
	Plaintext bool
	Insecure  bool
}

type Client interface {
	Connect()
	Close() error
	State() connectivity.State
	WaitForReady(ctx context.Context) error
	RawConnection() *grpc.ClientConn

	Applications() ApplicationService
	Clusters() ClusterService
	Projects() ProjectService
	Version() VersionService
	AvailableProjects() []string

	ApplicationsWatchSource() runner.StreamSource[*v1alpha1.ApplicationWatchEvent]
}

type client struct {
	conn   *grpc.ClientConn
	ctx    context.Context
	logger *slog.Logger
	cfg    Config

	// GRPCWeb proxy fields
	plainText       bool
	serverAddr      string
	grpcWebRootPath string
	httpClient      *http.Client
	proxyMutex      sync.Mutex
	proxyListener   net.Listener
	proxyServer     *grpc.Server
	proxyUsersCount int
	proxyCloser     io.Closer

	applicationService ApplicationService
	clusterService     ClusterService
	projectService     ProjectService
	versionService     VersionService
}

func NewClient(ctx context.Context, logger *slog.Logger, cfg Config) (Client, error) {
	rootCertificates, err := certificate.GetRootCertificatePool()
	if err != nil {
		logger.Warn("Failed to load root certificate pool", slog.Any("error", err))
	}
	tlsConfig := &tls.Config{
		RootCAs:            rootCertificates,
		InsecureSkipVerify: cfg.TLS.Insecure,
	}

	c := &client{
		ctx:             ctx,
		logger:          logger,
		cfg:             cfg,
		plainText:       cfg.TLS.Plaintext,
		grpcWebRootPath: cfg.GRPCWebRootPath,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
	}

	grpcDialOptions := []grpc.DialOption{
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(math.MaxInt32),
			grpc.MaxCallRecvMsgSize(math.MaxInt32),
		),
	}

	var sanitizedServerURL string

	if cfg.GRPCWeb || cfg.GRPCWebRootPath != "" {
		// Start a local gRPC server which proxies requests using the grpc-web protocol.
		c.serverAddr = sanitizeServerAddr(cfg.ServerURL)
		addr, proxyCloser, err := c.useGRPCProxy(ctx)
		if err != nil {
			return nil, err
		}
		c.proxyCloser = proxyCloser
		sanitizedServerURL = "unix:" + addr.String()
		grpcDialOptions = append(grpcDialOptions,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	} else {
		sanitizedServerURL = communication.SanitizeGrpcUrl(cfg.ServerURL)
		if !cfg.TLS.Plaintext {
			grpcDialOptions = append(grpcDialOptions,
				grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
			)
		} else {
			grpcDialOptions = append(grpcDialOptions,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
		}
	}

	if cfg.AuthToken == "" && len(cfg.ProjectAuthTokens) == 0 {
		return nil, errors.New(
			"argo cd auth misconfigured: no AuthToken or ProjectAuthTokens provided",
		)
	}

	var tokenResolver TokenResolver
	// Project auth tokens take priority
	if len(cfg.ProjectAuthTokens) > 0 {
		tokenResolver = NewProjectTokenResolver(cfg.ProjectAuthTokens)
	} else {
		tokenResolver = NewGlobalTokenResolver(cfg.AuthToken)
	}

	interceptor := newAuthInterceptor(cfg, tokenResolver)
	grpcDialOptions = append(grpcDialOptions,
		grpc.WithChainUnaryInterceptor(interceptor.Unary()),
		grpc.WithChainStreamInterceptor(interceptor.Stream()),
	)

	conn, err := grpc.NewClient(sanitizedServerURL, grpcDialOptions...)
	if err != nil {
		if c.proxyCloser != nil {
			_ = c.proxyCloser.Close()
		}
		return nil, err
	}

	c.conn = conn
	return c, nil
}

func (c *client) Connect() {
	c.conn.Connect()
}

func (c *client) Close() error {
	err := c.conn.Close()
	if c.proxyCloser != nil {
		_ = c.proxyCloser.Close()
	}
	return err
}

func (c *client) State() connectivity.State {
	return c.conn.GetState()
}

func (c *client) WaitForReady(ctx context.Context) error {
	return communication.WaitForReady(ctx, c.conn)
}

func (c *client) RawConnection() *grpc.ClientConn {
	return c.conn
}

func (c *client) Applications() ApplicationService {
	if c.applicationService == nil {
		c.applicationService = newApplicationService(c.ctx, c.logger, c.conn)
	}
	return c.applicationService
}

func (c *client) Clusters() ClusterService {
	if c.clusterService == nil {
		c.clusterService = newClusterService(c.ctx, c.logger, c.conn)
	}
	return c.clusterService
}

func (c *client) Projects() ProjectService {
	if c.projectService == nil {
		c.projectService = newProjectService(c.ctx, c.logger, c.conn)
	}
	return c.projectService
}

func (c *client) Version() VersionService {
	if c.versionService == nil {
		c.versionService = newVersionService(c.ctx, c.logger, c.conn)
	}
	return c.versionService
}

func (c *client) AvailableProjects() []string {
	return slices.Collect(maps.Keys(c.cfg.ProjectAuthTokens))
}

func (c *client) ApplicationsWatchSource() runner.StreamSource[*v1alpha1.ApplicationWatchEvent] {
	return &applicationsWatchSource{
		appService:  c.Applications(),
		errorPolicy: DefaultErrorPolicy,
	}
}

type applicationsWatchSource struct {
	appService  ApplicationService
	errorPolicy *ErrorPolicy
}

func (s *applicationsWatchSource) CreateStream(
	ctx context.Context,
) (runner.RecvStream[*v1alpha1.ApplicationWatchEvent], error) {
	return s.appService.WatchApplications(ctx)
}

func (s *applicationsWatchSource) ErrorPolicy() runner.ErrorPolicy {
	return s.errorPolicy
}
