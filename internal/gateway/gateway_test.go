package gateway

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OctopusDeploy/octopus-grpc/go/pkg/connection"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/configuration"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/logger"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/testutilities"
)

func TestNewGateway_ReturnsNewGateway(t *testing.T) {
	config := &configuration.Configuration{
		Gateway: configuration.GatewayConfig{
			ArgoCdAuthToken: "test-token",
		},
	}
	actual, err := New(t.Context(), config, logger.New(false))

	if err != nil || actual == nil {
		t.Error("Unexpected error while creating new gateway instance:", err)
	}
}

// testServer is an in-process health server reached through a real
// connection.Connection, so the supervision tests drive the health check over the
// same connection the gateway builds in production.
type testServer struct {
	conn   connection.Connection
	health *health.Server
	checks atomic.Int32
}

// serving flips what the server answers. A NOT_SERVING response is a failed probe
// as far as the health check is concerned.
func (s *testServer) serving(serving bool) {
	status := grpc_health_v1.HealthCheckResponse_NOT_SERVING
	if serving {
		status = grpc_health_v1.HealthCheckResponse_SERVING
	}

	s.health.SetServingStatus("", status)
}

func (s *testServer) awaitChecks(count int32) {
	for s.checks.Load() < count {
		time.Sleep(time.Millisecond)
	}
}

// awaitReady connects before the first probe. grpc.NewClient is lazy, and a probe
// is bounded by the check interval, so a test that let the dial happen under the
// first probe would be racing it.
func (s *testServer) awaitReady(t *testing.T) {
	t.Helper()

	client := s.conn.Get()
	client.Connect()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	for client.GetState() != connectivity.Ready {
		if !client.WaitForStateChange(ctx, client.GetState()) {
			t.Fatal("Expected the connection to become ready")
		}
	}
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()

	s := &testServer{health: health.NewServer()}
	s.serving(true)

	count := func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		s.checks.Add(1)

		return handler(ctx, req)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Expected to listen, got %v", err)
	}

	server := grpc.NewServer(grpc.UnaryInterceptor(count))
	grpc_health_v1.RegisterHealthServer(server, s.health)

	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	s.conn, err = connection.New(connection.Config{
		ServerURL: listener.Addr().String(),
		TLS:       connection.TLSConfig{Plaintext: true},
	}, testutilities.CreateTestLogger())
	if err != nil {
		t.Fatalf("Expected to build a connection, got %v", err)
	}

	t.Cleanup(func() { _ = s.conn.Close() })
	s.awaitReady(t)

	return s
}

func newTestGateway(ctx context.Context) (*Gateway, chan error) {
	errCh := make(chan error, 1)
	g := &Gateway{
		context: ctx,
		logger:  testutilities.CreateTestLogger(),
		ErrorCh: &errCh,
		config:  &configuration.Configuration{},
	}
	return g, errCh
}

func TestHealthCheckLoop_Down_RestartsSubscribersAfterRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	// First probe fails (triggers Down); all subsequent probes succeed (triggers Up).
	server := newTestServer(t)
	server.serving(false)
	go func() {
		server.awaitChecks(1)
		server.serving(true)
	}()

	loop := startSupervision(t, ctx, server, time.Second)

	firstCtx := loop.awaitSubscriberStart(t, ctx)
	secondCtx := loop.awaitSubscriberStart(t, ctx)

	if firstCtx == secondCtx {
		t.Error("Expected a new context for restarted subscribers, got the same one")
	}

	if firstCtx.Err() == nil {
		t.Error("Expected the first subscriber context to be cancelled after Down")
	}

	cancel()
	loop.awaitExit(t)
	loop.expectNoError(t)
}

// The health check's own error does not reach ErrorCh. Down is sent on the first
// failed probe, so the loop is inside AwaitRecovery by the time the health check
// gives up -- and AwaitRecovery consumes the error and reports only that recovery
// did not happen. The main select's Errors() branch is unreachable this way.
func TestHealthCheckLoop_HealthCheckGivesUp_EndsSupervision(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	server := newTestServer(t)
	server.serving(false)

	// A budget of a nanosecond has it give up on the very first failed probe.
	loop := startSupervision(t, ctx, server, time.Nanosecond)

	if err := loop.awaitError(t, ctx); !strings.Contains(err.Error(), "did not recover in time") {
		t.Errorf("Expected the loop to abandon recovery, got %v", err)
	}
}

func TestHealthCheckLoop_Down_ContextCancelledDuringRecovery(t *testing.T) {
	// A separate outer context, so the test controls when the gateway shuts down
	// independently of the test-wide timeout.
	testCtx, testCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer testCancel()
	outerCtx, outerCancel := context.WithCancel(testCtx)

	server := newTestServer(t)
	server.serving(false)

	// Long enough that the health check will not give up on its own.
	loop := startSupervision(t, outerCtx, server, time.Minute)

	firstCtx := loop.awaitSubscriberStart(t, testCtx)

	// Waiting for the run context to be cancelled confirms the loop has taken Down
	// and is inside AwaitRecovery, so the cancellation below is picked up there
	// rather than racing the main select.
	select {
	case <-firstCtx.Done():
	case <-testCtx.Done():
		t.Fatal("Expected the subscriber context to be cancelled after Down")
	}

	outerCancel()

	if err := loop.awaitError(t, testCtx); !strings.Contains(err.Error(), "did not recover in time") {
		t.Errorf("Expected the loop to abandon recovery, got %v", err)
	}
}

// supervision is a healthCheckLoop running in the background over a health check
// fed by server, with the channels a test needs to observe it.
type supervision struct {
	errCh             chan error
	subscriberStarted chan context.Context
	done              chan struct{}
}

func startSupervision(
	t *testing.T,
	ctx context.Context,
	server *testServer,
	giveUpAfter time.Duration,
) *supervision {
	t.Helper()

	g, errCh := newTestGateway(ctx)
	loop := &supervision{
		errCh:             errCh,
		subscriberStarted: make(chan context.Context, 5),
		done:              make(chan struct{}),
	}

	healthCheck := connection.NewHealthCheck(
		ctx,
		server.conn,
		connection.HealthCheckConfig{Interval: 50 * time.Millisecond, GiveUpAfter: giveUpAfter},
		g.logger,
	)
	go healthCheck.Start()

	go func() {
		g.healthCheckLoop(ctx, healthCheck, func(runCtx context.Context) {
			loop.subscriberStarted <- runCtx
		})
		close(loop.done)
	}()

	return loop
}

func (s *supervision) awaitSubscriberStart(t *testing.T, ctx context.Context) context.Context {
	t.Helper()

	select {
	case runCtx := <-s.subscriberStarted:
		return runCtx
	case <-ctx.Done():
		t.Fatal("Expected the subscribers to start")

		return nil
	}
}

func (s *supervision) awaitError(t *testing.T, ctx context.Context) error {
	t.Helper()

	select {
	case err := <-s.errCh:
		if err == nil {
			t.Fatal("Expected an error on ErrorCh, got nil")
		}

		return err
	case <-ctx.Done():
		t.Fatal("Expected an error on ErrorCh, got none")

		return nil
	}
}

func (s *supervision) awaitExit(t *testing.T) {
	t.Helper()

	select {
	case <-s.done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Expected healthCheckLoop to exit after its context was cancelled")
	}
}

func (s *supervision) expectNoError(t *testing.T) {
	t.Helper()

	select {
	case err := <-s.errCh:
		t.Errorf("Expected no error on ErrorCh, got %v", err)
	default:
	}
}
