package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopus"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/credentials"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/metrics"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/report"
)

type Gateway struct {
	Name string

	entry   credentials.Entry
	tls     TLSOptions
	client  octopus.Client
	logger  *slog.Logger
	metrics *metrics.Metrics
	rec     *report.Recorder

	syncsReceived atomic.Int64
	dead          atomic.Bool
}

type TLSOptions struct {
	Plaintext      bool
	PinCertificate bool
}

func New(
	index int,
	e credentials.Entry,
	tls TLSOptions,
	logger *slog.Logger,
	m *metrics.Metrics,
	rec *report.Recorder,
) *Gateway {
	name := e.Name
	if name == "" {
		name = fmt.Sprintf("sim-gw-%d", index)
	}
	return &Gateway{
		Name:    name,
		entry:   e,
		tls:     tls,
		logger:  logger.With(slog.String("gateway", name)),
		metrics: m,
		rec:     rec,
	}
}

func (g *Gateway) Dial(ctx context.Context) error {
	client, err := octopus.NewClient(ctx, g.logger, octopus.Config{
		ServerURL:   g.entry.ServerURL,
		ClientID:    g.entry.ClientID,
		BearerToken: g.entry.BearerToken,
		TLS: octopus.TLSConfig{
			Plaintext:      g.tls.Plaintext,
			Thumbprint:     g.entry.Thumbprint,
			PinCertificate: g.tls.PinCertificate,
		},
	})
	if err != nil {
		return fmt.Errorf("gateway %s: new client: %w", g.Name, err)
	}
	g.client = client
	client.Connect()
	if err := client.WaitForReady(ctx); err != nil {
		_ = client.Close()
		return fmt.Errorf("gateway %s: wait for ready: %w", g.Name, err)
	}
	prewarmClients(client)
	return nil
}

func prewarmClients(c octopus.Client) {
	_ = c.ApplicationEvents()
	_ = c.ApplicationManifests()
	_ = c.ApplicationResources()
	_ = c.ApplicationSync()
	_ = c.Info()
	_ = c.PodLogs()
	_ = c.ResourceEvents()
}

func (g *Gateway) Close() error {
	if g.client == nil {
		return nil
	}
	return g.client.Close()
}

func (g *Gateway) EventsClient() pb.ApplicationEventServiceClient {
	return g.client.ApplicationEvents()
}

func (g *Gateway) MarkDead() {
	if g.dead.Swap(true) {
		return
	}
	if g.metrics != nil {
		g.metrics.DeadGateways.Inc()
		g.metrics.ConnectedGateways.Dec()
	}
}

func (g *Gateway) IsDead() bool { return g.dead.Load() }

func (g *Gateway) SyncsReceived() int64 { return g.syncsReceived.Load() }

func (g *Gateway) Start(ctx context.Context) error {
	if g.client == nil {
		return errors.New("gateway not dialled")
	}
	if g.metrics != nil {
		g.metrics.ConnectedGateways.Inc()
	}

	runners := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"sync", g.runSync},
		{"watch-resources", g.runWatchResources},
		{"manifests", g.runManifests},
		{"resource-events", g.runResourceEvents},
		{"pod-logs", g.runPodLogs},
		{"info", g.runInfo},
		{"gateway-logs", g.runGatewayLogs},
	}
	var wg sync.WaitGroup
	errCh := make(chan error, len(runners))

	for _, r := range runners {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.fn(ctx); err != nil && !isCtxErr(ctx, err) {
				g.logger.Debug("stream exited with error",
					slog.String("stream", r.name), slog.Any("error", err))
				g.MarkDead()
				select {
				case errCh <- fmt.Errorf("stream %s: %w", r.name, err):
				default:
				}
			}
		}()
	}

	<-ctx.Done()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		g.logger.Debug("timed out waiting for streams to stop")
	}

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func (g *Gateway) runSync(ctx context.Context) error {
	stream, err := g.client.ApplicationSync().SyncApplication(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = stream.CloseSend() }()

	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}
		recvAt := time.Now()
		if g.metrics != nil {
			g.metrics.SyncsReceivedTotal.WithLabelValues(g.Name).Inc()
		}
		g.syncsReceived.Add(1)

		resp := &pb.SyncApplicationResponse{RequestId: req.GetRequestId()}
		if err := stream.Send(resp); err != nil {
			return err
		}

		replyLat := time.Since(recvAt)
		if g.metrics != nil {
			g.metrics.SyncReplyLatency.Observe(replyLat.Seconds())
		}
		if g.rec != nil {
			g.rec.RecordSyncReply(replyLat)
		}
	}
}

func (g *Gateway) runWatchResources(ctx context.Context) error {
	stream, err := g.client.ApplicationResources().WatchApplicationResources(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = stream.CloseSend() }()
	return drainRecv(func() error {
		_, err := stream.Recv()
		return err
	})
}

func (g *Gateway) runManifests(ctx context.Context) error {
	stream, err := g.client.ApplicationManifests().GetResourceManifests(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = stream.CloseSend() }()
	return drainRecv(func() error {
		_, err := stream.Recv()
		return err
	})
}

func (g *Gateway) runResourceEvents(ctx context.Context) error {
	stream, err := g.client.ResourceEvents().GetResourceEvents(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = stream.CloseSend() }()
	return drainRecv(func() error {
		_, err := stream.Recv()
		return err
	})
}

func (g *Gateway) runPodLogs(ctx context.Context) error {
	stream, err := g.client.PodLogs().GetPodLogs(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = stream.CloseSend() }()
	return drainRecv(func() error {
		_, err := stream.Recv()
		return err
	})
}

func (g *Gateway) runInfo(ctx context.Context) error {
	stream, err := g.client.Info().GetInfo(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = stream.CloseSend() }()
	return drainRecv(func() error {
		_, err := stream.Recv()
		return err
	})
}

func (g *Gateway) runGatewayLogs(ctx context.Context) error {
	stream, err := g.client.Info().GetGatewayLogs(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = stream.CloseSend() }()
	return drainRecv(func() error {
		_, err := stream.Recv()
		return err
	})
}

func drainRecv(recv func() error) error {
	for {
		if err := recv(); err != nil {
			return err
		}
	}
}

func isCtxErr(ctx context.Context, err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unimplemented, codes.Canceled:
			return true
		}
	}
	return ctx.Err() != nil
}
