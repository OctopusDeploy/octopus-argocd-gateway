package scenario

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/payload"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/report"
)

func RunIngest(ctx context.Context, cfg Config, deps Deps) (*report.Report, error) {
	if cfg.TargetRate <= 0 {
		return nil, fmt.Errorf("ingest: target rate must be > 0, got %d", cfg.TargetRate)
	}
	if deps.Pool.Size() == 0 {
		return nil, fmt.Errorf("ingest: pool has no gateways")
	}
	if _, err := cfg.PayloadSize.ResourceCount(); err != nil {
		return nil, fmt.Errorf("ingest: %w", err)
	}

	cache, err := payload.NewCacheWithPool(cfg.Seed, resolvePoolSize(cfg.AppPoolSize), cfg.Scope, cfg.PayloadSize)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	deps.Logger.Info("ingest scenario started",
		slog.Int("target_rate", cfg.TargetRate),
		slog.String("payload", string(cfg.PayloadSize)),
		slog.Int("payload_pool", cache.PoolSize(cfg.PayloadSize)),
		slog.Duration("duration", cfg.Duration),
		slog.Int("gateways", deps.Pool.Size()))

	runCtx, cancel := cancelAfter(ctx, cfg.Duration)
	defer cancel()

	deps.Pool.Run(runCtx)
	deps.Metrics.TargetRate.Set(float64(cfg.TargetRate))

	driveIngest(runCtx, cfg.TargetRate, cache, cfg.PayloadSize, deps)

	deps.Pool.Wait()

	return deps.Rec.Build(
		string(NameIngest),
		configMap(cfg, nil),
		started,
		deps.Pool.Size(),
		deps.Pool.DeadCount(),
	), nil
}

func driveIngest(ctx context.Context, targetRate int, cache *payload.Cache, size payload.SizeClass, deps Deps) {
	workers := min(targetRate, 512)
	workers = max(workers, 32)

	jobs := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				sendOne(ctx, cache, size, deps)
			}
		}()
	}

	burst := max(targetRate/10, 1)
	limiter := rate.NewLimiter(rate.Limit(targetRate), burst)

	var sentInLastSecond atomic.Int64
	rateTicker := time.NewTicker(time.Second)
	defer rateTicker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-rateTicker.C:
				n := sentInLastSecond.Swap(0)
				if deps.Metrics != nil {
					deps.Metrics.AchievedRate.Set(float64(n))
				}
			}
		}
	}()

	for {
		if err := limiter.Wait(ctx); err != nil {
			close(jobs)
			wg.Wait()
			return
		}
		select {
		case jobs <- struct{}{}:
			sentInLastSecond.Add(1)
		default:
			// All workers busy; drop this token so saturation's stop
			// conditions can observe achieved < target.
		}
	}
}

func sendOne(ctx context.Context, cache *payload.Cache, size payload.SizeClass, deps Deps) {
	g := deps.Pool.Next()
	if g == nil {
		return
	}
	eventJSON, err := cache.Get(size)
	if err != nil {
		if deps.Metrics != nil {
			deps.Metrics.UpdatesTotal.WithLabelValues("err", "cache_miss").Inc()
		}
		return
	}
	req := &pb.ApplicationEventUpdateRequest{EventPayloadJson: eventJSON}
	start := time.Now()
	_, err = g.EventsClient().ApplicationEventUpdate(ctx, req)
	latency := time.Since(start)

	code := grpcCodeString(err)
	result := "ok"
	if err != nil {
		result = "err"
	}

	if deps.Metrics != nil {
		deps.Metrics.UpdateLatency.Observe(latency.Seconds())
		deps.Metrics.UpdatesTotal.WithLabelValues(result, code).Inc()
	}
	if deps.Rec != nil {
		deps.Rec.RecordUpdate(latency, code)
	}
}
