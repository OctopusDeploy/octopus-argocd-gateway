package applicationresources

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OctopusDeploy/octopus-grpc/go/pkg/connection"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/metrics"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
)

// --- Octopus + Argo fakes (minimal surface for Handler.Start recv loop)

type handlerTestOctopusClient struct {
	appResources pb.ApplicationResourceServiceClient
}

func (c *handlerTestOctopusClient) Connect() {
}

func (c *handlerTestOctopusClient) Close() error {
	return nil
}

func (c *handlerTestOctopusClient) State() connectivity.State {
	return connectivity.Ready
}

func (c *handlerTestOctopusClient) WaitForReady(context.Context) error {
	return nil
}

func (c *handlerTestOctopusClient) RawConnection() *grpc.ClientConn {
	return nil
}

func (c *handlerTestOctopusClient) ApplicationEvents() pb.ApplicationEventServiceClient {
	return nil
}

func (c *handlerTestOctopusClient) ApplicationManifests() pb.ApplicationManifestsServiceClient {
	return nil
}

func (c *handlerTestOctopusClient) ApplicationResources() pb.ApplicationResourceServiceClient {
	return c.appResources
}

func (c *handlerTestOctopusClient) ApplicationResourceTree() pb.ApplicationResourceTreeServiceClient {
	return nil
}

func (c *handlerTestOctopusClient) ApplicationSync() pb.ApplicationSyncServiceClient {
	return nil
}

func (c *handlerTestOctopusClient) Info() pb.InfoServiceClient {
	return nil
}

func (c *handlerTestOctopusClient) PodLogs() pb.PodLogsServiceClient {
	return nil
}

func (c *handlerTestOctopusClient) PruneApplications() pb.PruneApplicationsServiceClient {
	return nil
}

func (c *handlerTestOctopusClient) ResourceEvents() pb.ResourceEventsServiceClient {
	return nil
}

func (c *handlerTestOctopusClient) Connection() connection.Connection {
	return nil
}

func (c *handlerTestOctopusClient) ManifestsStreamSource() runner.BidirectionalStreamSource[*pb.GetResourceManifestsRequest, *pb.GetResourceManifestsResponse] {
	return nil
}

func (c *handlerTestOctopusClient) ResourceEventsStreamSource() runner.BidirectionalStreamSource[*pb.GetResourceEventsRequest, *pb.GetResourceEventsResponse] {
	return nil
}

func (c *handlerTestOctopusClient) PodLogsStreamSource() runner.BidirectionalStreamSource[*pb.GetPodLogsRequest, *pb.GetPodLogsResponse] {
	return nil
}

func (c *handlerTestOctopusClient) InfoStreamSource() runner.BidirectionalStreamSource[*pb.GetInfoRequest, *pb.GetInfoResponse] {
	return nil
}

func (c *handlerTestOctopusClient) GatewayLogsStreamSource() runner.BidirectionalStreamSource[*pb.GetGatewayLogsRequest, *pb.GetGatewayLogsResponse] {
	return nil
}

func (c *handlerTestOctopusClient) ApplicationResourcesStreamSource() runner.BidirectionalStreamSource[*pb.WatchApplicationResourcesRequest, *pb.WatchApplicationResourcesResponse] {
	return nil
}

func (c *handlerTestOctopusClient) SyncStreamSource() runner.BidirectionalStreamSource[*pb.SyncApplicationRequest, *pb.SyncApplicationResponse] {
	return nil
}

func (c *handlerTestOctopusClient) ApplicationResourceTreeStreamSource() runner.BidirectionalStreamSource[*pb.GetApplicationResourceTreeRequest, *pb.GetApplicationResourceTreeResponse] {
	return nil
}

type handlerTestArgoClient struct{}

func (c handlerTestArgoClient) AvailableProjects() []string {
	return nil
}

func (handlerTestArgoClient) Connect() {
}

func (handlerTestArgoClient) Close() error {
	return nil
}

func (handlerTestArgoClient) State() connectivity.State {
	return connectivity.Ready
}

func (handlerTestArgoClient) WaitForReady(context.Context) error {
	return nil
}

func (handlerTestArgoClient) RawConnection() *grpc.ClientConn {
	return nil
}

func (handlerTestArgoClient) Applications() argocd.ApplicationService {
	return nil
}

func (handlerTestArgoClient) Clusters() argocd.ClusterService {
	return nil
}

func (handlerTestArgoClient) Projects() argocd.ProjectService {
	return nil
}

func (handlerTestArgoClient) Version() argocd.VersionService {
	return nil
}

func (handlerTestArgoClient) ApplicationsWatchSource() runner.StreamSource[*v1alpha1.ApplicationWatchEvent] {
	return nil
}

type unavailableRecvStream struct {
	ctx context.Context
}

func (f *unavailableRecvStream) Recv() (*pb.WatchApplicationResourcesRequest, error) {
	return nil, status.Error(codes.Unavailable, "EOF")
}

func (f *unavailableRecvStream) Send(*pb.WatchApplicationResourcesResponse) error {
	return nil
}

func (f *unavailableRecvStream) Header() (metadata.MD, error) {
	return nil, nil
}

func (f *unavailableRecvStream) Trailer() metadata.MD {
	return nil
}

func (f *unavailableRecvStream) CloseSend() error {
	return nil
}

func (f *unavailableRecvStream) Context() context.Context {
	if f.ctx != nil {
		return f.ctx
	}
	return context.Background()
}

func (f *unavailableRecvStream) SendMsg(m any) error {
	return nil
}

func (f *unavailableRecvStream) RecvMsg(m any) error {
	return status.Error(codes.Unavailable, "EOF")
}

type appResourceServiceFake struct {
	watchFn func(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[pb.WatchApplicationResourcesResponse, pb.WatchApplicationResourcesRequest], error)
}

func (s *appResourceServiceFake) WatchApplicationResources(
	ctx context.Context,
	opts ...grpc.CallOption,
) (grpc.BidiStreamingClient[pb.WatchApplicationResourcesResponse, pb.WatchApplicationResourcesRequest], error) {
	if s.watchFn != nil {
		return s.watchFn(ctx, opts...)
	}
	return &unavailableRecvStream{ctx: ctx}, nil
}

// countingMsgHook counts records with a given message (e.g. "Recoverable stream error").
type countingMsgHook struct {
	inner   slog.Handler
	target  string
	counter *atomic.Int32
}

func (h *countingMsgHook) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}

func (h *countingMsgHook) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == h.target {
		h.counter.Add(1)
	}
	return h.inner.Handle(ctx, r)
}

func (h *countingMsgHook) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &countingMsgHook{inner: h.inner.WithAttrs(attrs), target: h.target, counter: h.counter}
}

func (h *countingMsgHook) WithGroup(name string) slog.Handler {
	return &countingMsgHook{inner: h.inner.WithGroup(name), target: h.target, counter: h.counter}
}

// TestHandler_RecoverableRecvCancelExits exercises recoverable Recv errors, backoff, and clean exit on cancel.
func TestHandler_RecoverableRecvCancelExits(t *testing.T) {
	var recoverableN atomic.Int32
	hook := &countingMsgHook{
		inner:   slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}),
		target:  "Recoverable stream error",
		counter: &recoverableN,
	}
	logger := slog.New(hook)

	ctx, cancel := context.WithCancel(context.Background())
	octo := &handlerTestOctopusClient{
		appResources: &appResourceServiceFake{},
	}

	h, err := New(logger, ctx, octo, handlerTestArgoClient{}, metrics.NoopMetrics{})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Start()
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}

	if recoverableN.Load() < 1 {
		t.Fatal("expected at least one recoverable stream error log")
	}
}

// TestHandler_InitialConnectFailure_SendsErrorAndExits when WatchApplicationResources fails immediately.
func TestHandler_InitialConnectFailure_SendsErrorAndExits(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ctx := context.Background()

	octo := &handlerTestOctopusClient{
		appResources: &appResourceServiceFake{
			watchFn: func(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[pb.WatchApplicationResourcesResponse, pb.WatchApplicationResourcesRequest], error) {
				return nil, status.Error(codes.Unavailable, "no upstream")
			},
		},
	}

	h, err := New(logger, ctx, octo, handlerTestArgoClient{}, metrics.NoopMetrics{})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Start()
	}()

	select {
	case err := <-h.Errors():
		if err == nil {
			t.Fatal("expected connect error on Errors()")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for connect error")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not exit after connect failure")
	}
}

// TestHandler_UnrecoverableRecvError_OnErrorsChannel when Recv returns a non-recoverable error.
func TestHandler_UnrecoverableRecvError_OnErrorsChannel(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := &invalidArgRecvStream{ctx: ctx}
	octo := &handlerTestOctopusClient{
		appResources: &appResourceServiceFake{
			watchFn: func(c context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[pb.WatchApplicationResourcesResponse, pb.WatchApplicationResourcesRequest], error) {
				return stream, nil
			},
		},
	}

	h, err := New(logger, ctx, octo, handlerTestArgoClient{}, metrics.NoopMetrics{})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Start()
	}()

	select {
	case err := <-h.Errors():
		if err == nil {
			t.Fatal("expected error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for error")
	}

	<-done
}

type invalidArgRecvStream struct {
	ctx context.Context
}

func (f *invalidArgRecvStream) Recv() (*pb.WatchApplicationResourcesRequest, error) {
	return nil, status.Error(codes.InvalidArgument, "bad")
}

func (f *invalidArgRecvStream) Send(*pb.WatchApplicationResourcesResponse) error {
	return nil
}

func (f *invalidArgRecvStream) Header() (metadata.MD, error) {
	return nil, nil
}

func (f *invalidArgRecvStream) Trailer() metadata.MD {
	return nil
}

func (f *invalidArgRecvStream) CloseSend() error {
	return nil
}

func (f *invalidArgRecvStream) Context() context.Context {
	if f.ctx != nil {
		return f.ctx
	}
	return context.Background()
}

func (f *invalidArgRecvStream) SendMsg(m any) error {
	return nil
}

func (f *invalidArgRecvStream) RecvMsg(m any) error {
	return status.Error(codes.InvalidArgument, "bad")
}

// trackingRecvStream records CloseSend calls so tests can assert the old stream is
// closed before a new connection is opened.
type trackingRecvStream struct {
	ctx         context.Context
	onCloseSend func()
}

func (f *trackingRecvStream) Recv() (*pb.WatchApplicationResourcesRequest, error) {
	return nil, status.Error(codes.Unavailable, "EOF")
}

func (f *trackingRecvStream) Send(*pb.WatchApplicationResourcesResponse) error { return nil }

func (f *trackingRecvStream) Header() (metadata.MD, error) { return nil, nil }

func (f *trackingRecvStream) Trailer() metadata.MD { return nil }

func (f *trackingRecvStream) CloseSend() error {
	if f.onCloseSend != nil {
		f.onCloseSend()
	}
	return nil
}

func (f *trackingRecvStream) Context() context.Context {
	if f.ctx != nil {
		return f.ctx
	}
	return context.Background()
}

func (f *trackingRecvStream) SendMsg(m any) error { return nil }

func (f *trackingRecvStream) RecvMsg(m any) error {
	return status.Error(codes.Unavailable, "EOF")
}

// TestHandler_Connect_ClosesStreamBeforeReconnect verifies that CloseSend is called on the
// existing stream before WatchApplicationResources is called again after a recoverable error.
func TestHandler_Connect_ClosesStreamBeforeReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var closeSendCalled atomic.Bool
	var connectCount atomic.Int32

	firstStream := &trackingRecvStream{
		ctx: ctx,
		onCloseSend: func() {
			closeSendCalled.Store(true)
		},
	}

	octo := &handlerTestOctopusClient{
		appResources: &appResourceServiceFake{
			watchFn: func(c context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[pb.WatchApplicationResourcesResponse, pb.WatchApplicationResourcesRequest], error) {
				n := connectCount.Add(1)
				if n == 1 {
					return firstStream, nil
				}
				// CloseSend must have been called on the first stream before we get here.
				if !closeSendCalled.Load() {
					t.Error("CloseSend was not called on the old stream before the new WatchApplicationResources call")
				}
				cancel()
				return &unavailableRecvStream{ctx: c}, nil
			},
		},
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h, err := New(logger, ctx, octo, handlerTestArgoClient{}, metrics.NoopMetrics{})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Start()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return")
	}

	if connectCount.Load() < 2 {
		t.Fatal("expected at least 2 connects (initial + after recoverable error)")
	}
	if !closeSendCalled.Load() {
		t.Fatal("expected CloseSend to be called on the first stream before reconnecting")
	}
}

var (
	_ grpc.BidiStreamingClient[pb.WatchApplicationResourcesResponse, pb.WatchApplicationResourcesRequest] = (*unavailableRecvStream)(
		nil,
	)
	_ grpc.BidiStreamingClient[pb.WatchApplicationResourcesResponse, pb.WatchApplicationResourcesRequest] = (*invalidArgRecvStream)(
		nil,
	)
	_ grpc.BidiStreamingClient[pb.WatchApplicationResourcesResponse, pb.WatchApplicationResourcesRequest] = (*trackingRecvStream)(
		nil,
	)
)
