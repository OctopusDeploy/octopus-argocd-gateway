package scenario

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"google.golang.org/grpc/status"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/metrics"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/payload"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/pool"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/report"
)

type Name string

const (
	NameSyncLatency Name = "sync-latency"
	NameIngest      Name = "ingest"
	NameSaturation  Name = "saturation"
)

func ParseName(s string) (Name, error) {
	switch Name(s) {
	case NameSyncLatency, NameIngest, NameSaturation:
		return Name(s), nil
	default:
		return "", fmt.Errorf("unknown scenario %q (want sync-latency|ingest|saturation)", s)
	}
}

type Config struct {
	Duration    time.Duration
	TargetRate  int
	PayloadSize payload.SizeClass
	Seed        uint64
	// AppPoolSize is the number of distinct application UIDs the payload
	// generator pre-marshals per size class. 0 falls back to payload.DefaultPoolSize.
	AppPoolSize int
	// Scope, when non-nil, drives Octopus scoping annotations on a deterministic
	// percentage of generated applications. Nil leaves payloads unannotated.
	Scope *payload.ScopingPool
	// AppsOutputPath, when non-empty, writes the sync-latency catalogue here.
	AppsOutputPath string
}

type Deps struct {
	Logger  *slog.Logger
	Pool    *pool.Pool
	Metrics *metrics.Metrics
	Rec     *report.Recorder
}

func grpcCodeString(err error) string {
	if err == nil {
		return ""
	}
	if st, ok := status.FromError(err); ok {
		return st.Code().String()
	}
	return "unknown"
}

func configMap(cfg Config, extra map[string]any) map[string]any {
	out := map[string]any{
		"duration_s":    cfg.Duration.Seconds(),
		"target_rate":   cfg.TargetRate,
		"payload_size":  string(cfg.PayloadSize),
		"seed":          cfg.Seed,
		"app_pool_size": resolvePoolSize(cfg.AppPoolSize),
	}
	maps.Copy(out, extra)
	return out
}

func resolvePoolSize(requested int) int {
	if requested <= 0 {
		return payload.DefaultPoolSize
	}
	return requested
}

func cancelAfter(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, d)
}
