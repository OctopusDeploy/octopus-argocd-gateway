package scenario

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/payload"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/report"
)

func RunSyncLatency(ctx context.Context, cfg Config, deps Deps) (*report.Report, error) {
	if deps.Pool.Size() == 0 {
		return nil, fmt.Errorf("sync-latency: pool has no gateways")
	}

	started := time.Now()
	poolSize := resolvePoolSize(cfg.AppPoolSize)
	deps.Logger.Info("sync-latency scenario started",
		slog.Duration("duration", cfg.Duration),
		slog.Int("gateways", deps.Pool.Size()),
		slog.Int("app_pool_size", poolSize))

	runCtx, cancel := cancelAfter(ctx, cfg.Duration)
	defer cancel()

	deps.Pool.Run(runCtx)

	// Without pre-ingest the server has no record of these apps on this
	// gateway and the sync endpoint returns "application not found".
	if err := preIngestForSync(runCtx, cfg.Seed, poolSize, cfg.Scope, deps); err != nil {
		return nil, fmt.Errorf("sync-latency: pre-ingest: %w", err)
	}
	deps.Logger.Info("sync-latency pre-ingest complete", slog.Int("apps", poolSize))

	if cfg.AppsOutputPath != "" {
		names := advertisedAppNames(cfg.Seed, poolSize)
		if err := writeAppsCatalogue(cfg.AppsOutputPath, names); err != nil {
			return nil, fmt.Errorf("sync-latency: write apps file: %w", err)
		}
		deps.Logger.Info("wrote advertised apps catalogue",
			slog.String("path", cfg.AppsOutputPath),
			slog.Int("apps", len(names)))
	}

	<-runCtx.Done()
	deps.Pool.Wait()

	rep := deps.Rec.Build(
		string(NameSyncLatency),
		configMap(cfg, nil),
		started,
		deps.Pool.Size(),
		deps.Pool.DeadCount(),
	)

	var totalSyncs int64
	for _, g := range deps.Pool.Gateways() {
		totalSyncs += g.SyncsReceived()
	}
	rep.SyncsReceived = totalSyncs

	return rep, nil
}

// Keep in sync with payload.BuildApplication's naming rule.
func advertisedAppNames(seed uint64, poolSize int) []string {
	names := make([]string, poolSize)
	for i := 0; i < poolSize; i++ {
		names[i] = fmt.Sprintf("sim-app-%d", seed+uint64(i))
	}
	return names
}

func preIngestForSync(ctx context.Context, seed uint64, poolSize int, scope *payload.ScopingPool, deps Deps) error {
	// Sync-latency only needs the server to observe each app exists.
	cache, err := payload.NewCacheWithPool(seed, poolSize, scope, payload.SizeSmall)
	if err != nil {
		return err
	}
	for i := 0; i < poolSize; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		sendOne(ctx, cache, payload.SizeSmall, deps)
	}
	return nil
}

func writeAppsCatalogue(path string, apps []string) error {
	data, err := json.Marshal(apps)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}
