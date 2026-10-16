package simulator_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/credentials"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/gateway"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/metrics"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/payload"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/pool"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/report"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/scenario"
)

type fakeServer struct {
	pb.UnimplementedApplicationEventServiceServer
	pb.UnimplementedApplicationSyncServiceServer
	pb.UnimplementedApplicationResourceServiceServer
	pb.UnimplementedApplicationManifestsServiceServer
	pb.UnimplementedResourceEventsServiceServer
	pb.UnimplementedPodLogsServiceServer
	pb.UnimplementedInfoServiceServer
	pb.UnimplementedPruneApplicationsServiceServer

	syncsToSend   chan *pb.SyncApplicationRequest
	syncStreamReg chan struct{}
	eventsRecv    atomic.Int64
	syncsReplied  atomic.Int64
}

func newFakeServer() *fakeServer {
	return &fakeServer{
		syncsToSend:   make(chan *pb.SyncApplicationRequest, 1024),
		syncStreamReg: make(chan struct{}, 1),
	}
}

func (s *fakeServer) ApplicationEventUpdate(
	_ context.Context,
	_ *pb.ApplicationEventUpdateRequest,
) (*pb.ApplicationEventUpdateResponse, error) {
	s.eventsRecv.Add(1)
	return &pb.ApplicationEventUpdateResponse{}, nil
}

func (s *fakeServer) SyncApplication(
	stream grpc.BidiStreamingServer[pb.SyncApplicationResponse, pb.SyncApplicationRequest],
) error {
	ctx := stream.Context()

	select {
	case s.syncStreamReg <- struct{}{}:
	default:
	}

	errCh := make(chan error, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				errCh <- nil
				return
			case req := <-s.syncsToSend:
				if err := stream.Send(req); err != nil {
					errCh <- err
					return
				}
			}
		}
	}()

	for {
		_, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		s.syncsReplied.Add(1)
	}
}

func (s *fakeServer) WatchApplicationResources(
	stream grpc.BidiStreamingServer[pb.WatchApplicationResourcesResponse, pb.WatchApplicationResourcesRequest],
) error {
	return drainStream(stream)
}

func (s *fakeServer) GetResourceManifests(
	stream grpc.BidiStreamingServer[pb.GetResourceManifestsResponse, pb.GetResourceManifestsRequest],
) error {
	return drainStream(stream)
}

func (s *fakeServer) GetResourceEvents(
	stream grpc.BidiStreamingServer[pb.GetResourceEventsResponse, pb.GetResourceEventsRequest],
) error {
	return drainStream(stream)
}

func (s *fakeServer) GetPodLogs(stream grpc.BidiStreamingServer[pb.GetPodLogsResponse, pb.GetPodLogsRequest]) error {
	return drainStream(stream)
}

func (s *fakeServer) GetInfo(stream grpc.BidiStreamingServer[pb.GetInfoResponse, pb.GetInfoRequest]) error {
	return drainStream(stream)
}

func (s *fakeServer) GetGatewayLogs(
	stream grpc.BidiStreamingServer[pb.GetGatewayLogsResponse, pb.GetGatewayLogsRequest],
) error {
	return drainStream(stream)
}

func drainStream[Req any, Resp any](stream grpc.BidiStreamingServer[Resp, Req]) error {
	for {
		if _, err := stream.Recv(); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func startFakeServer(t *testing.T) (*fakeServer, string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	fake := newFakeServer()
	pb.RegisterApplicationEventServiceServer(srv, fake)
	pb.RegisterApplicationSyncServiceServer(srv, fake)
	pb.RegisterApplicationResourceServiceServer(srv, fake)
	pb.RegisterApplicationManifestsServiceServer(srv, fake)
	pb.RegisterResourceEventsServiceServer(srv, fake)
	pb.RegisterPodLogsServiceServer(srv, fake)
	pb.RegisterInfoServiceServer(srv, fake)
	pb.RegisterPruneApplicationsServiceServer(srv, fake)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = srv.Serve(lis)
	}()

	return fake, lis.Addr().String(), func() {
		srv.GracefulStop()
		wg.Wait()
	}
}

func TestSimulator_IngestReachesServerAndProducesReport(t *testing.T) {
	fake, addr, shutdown := startFakeServer(t)
	defer shutdown()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New()
	rec := report.NewRecorder()

	p := pool.New(pool.Config{
		Entries: []credentials.Entry{
			{Name: "sim-a", ClientID: "cid-a", BearerToken: "tok-a", ServerURL: addr},
			{Name: "sim-b", ClientID: "cid-b", BearerToken: "tok-b", ServerURL: addr},
		},
		TLS:             gateway.TLSOptions{Plaintext: true},
		DialTimeout:     5 * time.Second,
		DialConcurrency: 8,
	}, log, m, rec)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, p.Dial(ctx))
	defer p.Close()

	rep, err := scenario.RunIngest(ctx, scenario.Config{
		Duration:    2 * time.Second,
		TargetRate:  50,
		PayloadSize: payload.SizeSmall,
		Seed:        1,
	}, scenario.Deps{Logger: log, Pool: p, Metrics: m, Rec: rec})
	require.NoError(t, err)

	assert.Equal(t, "ingest", rep.Scenario)
	assert.Greater(t, rep.UpdateSuccesses, int64(0))
	// In-flight calls cancelled at ctx end can race with shutdown and surface
	// as Canceled; tolerate a small number.
	assert.LessOrEqual(t, rep.UpdateErrors, int64(5), "errors_by_code=%v", rep.ErrorsByCode)
	assert.NotNil(t, rep.UpdateLatency)
	assert.Greater(t, rep.UpdateLatency.Count, int64(0))
	assert.GreaterOrEqual(t, fake.eventsRecv.Load(), rep.UpdateSuccesses)
}

func TestSimulator_SyncLatencyRespondsToServerPushes(t *testing.T) {
	fake, addr, shutdown := startFakeServer(t)
	defer shutdown()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New()
	rec := report.NewRecorder()

	p := pool.New(pool.Config{
		Entries: []credentials.Entry{
			{Name: "sim-a", ClientID: "cid-a", BearerToken: "tok-a", ServerURL: addr},
		},
		TLS:             gateway.TLSOptions{Plaintext: true},
		DialTimeout:     5 * time.Second,
		DialConcurrency: 4,
	}, log, m, rec)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, p.Dial(ctx))
	defer p.Close()

	go func() {
		select {
		case <-fake.syncStreamReg:
		case <-ctx.Done():
			return
		}
		for i := range 20 {
			fake.syncsToSend <- &pb.SyncApplicationRequest{
				RequestId:            "req-" + strconv.Itoa(i),
				ApplicationName:      "app",
				ApplicationProject:   "default",
				ApplicationNamespace: "ns",
			}
		}
	}()

	rep, err := scenario.RunSyncLatency(ctx, scenario.Config{
		Duration: 2 * time.Second,
	}, scenario.Deps{Logger: log, Pool: p, Metrics: m, Rec: rec})
	require.NoError(t, err)

	assert.Equal(t, "sync-latency", rep.Scenario)
	assert.Equal(t, int64(20), rep.SyncsReceived)
	assert.Equal(t, int64(20), fake.syncsReplied.Load())
	assert.NotNil(t, rep.SyncReplyLatency)
	assert.Equal(t, int64(20), rep.SyncReplyLatency.Count)
}
