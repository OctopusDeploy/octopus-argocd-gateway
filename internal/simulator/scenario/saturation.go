package scenario

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/payload"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/report"
)

const (
	defaultSaturationStart   = 100
	defaultSaturationStepDur = 60 * time.Second
	defaultSaturationCeiling = 4000
	saturationSampleInterval = time.Second
	saturationStopWindow     = 60 * time.Second
)

func RunSaturation(ctx context.Context, cfg Config, deps Deps) (*report.Report, error) {
	if deps.Pool.Size() == 0 {
		return nil, fmt.Errorf("saturation: pool has no gateways")
	}
	if _, err := cfg.PayloadSize.ResourceCount(); err != nil {
		return nil, fmt.Errorf("saturation: %w", err)
	}

	cache, err := payload.NewCacheWithPool(cfg.Seed, resolvePoolSize(cfg.AppPoolSize), cfg.Scope, cfg.PayloadSize)
	if err != nil {
		return nil, err
	}

	start := cfg.TargetRate
	if start <= 0 {
		start = defaultSaturationStart
	}

	started := time.Now()
	deps.Logger.Info("saturation scenario started",
		slog.Int("start_rate", start),
		slog.String("payload", string(cfg.PayloadSize)),
		slog.Int("gateways", deps.Pool.Size()),
		slog.Int("ceiling", defaultSaturationCeiling))

	runCtx, cancel := cancelAfter(ctx, cfg.Duration)
	defer cancel()

	deps.Pool.Run(runCtx)

	result, lastStable, rampSteps, targetHistory := driveSaturation(runCtx, start, cache, cfg.PayloadSize, deps)

	cancel()
	deps.Pool.Wait()

	rep := deps.Rec.Build(
		string(NameSaturation),
		configMap(cfg, map[string]any{
			"start_rate":     start,
			"ceiling":        defaultSaturationCeiling,
			"step_duration":  defaultSaturationStepDur.Seconds(),
			"target_history": targetHistory,
		}),
		started,
		deps.Pool.Size(),
		deps.Pool.DeadCount(),
	)
	rep.Saturation = &report.SaturationResult{
		StopCondition:  result,
		LastStableRate: lastStable,
		LastTargetRate: float64(targetHistory[len(targetHistory)-1]),
		RampSteps:      rampSteps,
	}
	return rep, nil
}

func driveSaturation(
	parent context.Context,
	startRate int,
	cache *payload.Cache,
	size payload.SizeClass,
	deps Deps,
) (stopCondition string, lastStable float64, rampSteps int, history []int) {
	conds := SaturationStopConditions()
	tracker := newStopTracker()

	target := startRate
	history = append(history, target)
	lastStable = float64(target)

	for {
		if parent.Err() != nil {
			return "", lastStable, rampSteps, history
		}
		if target > defaultSaturationCeiling {
			return "ceiling_reached", lastStable, rampSteps, history
		}

		stepCtx, stepCancel := context.WithTimeout(parent, defaultSaturationStepDur)
		deps.Metrics.TargetRate.Set(float64(target))
		deps.Logger.Info("saturation step", slog.Int("target", target), slog.Int("step", rampSteps))

		var wg sync.WaitGroup
		wg.Add(1)
		dispatchDone := make(chan struct{})
		go func() {
			defer wg.Done()
			defer close(dispatchDone)
			driveIngest(stepCtx, target, cache, size, deps)
		}()

		fired := runStopEvaluator(stepCtx, tracker, conds, float64(target), deps)

		stepCancel()
		<-dispatchDone
		wg.Wait()

		if fired != "" {
			return fired, lastStable, rampSteps, history
		}

		rampSteps++
		lastStable = float64(target)
		target *= 2
		history = append(history, target)
	}
}

func runStopEvaluator(
	ctx context.Context,
	tracker *stopTracker,
	conds []StopCondition,
	target float64,
	deps Deps,
) string {
	windowStart := time.Now()
	baselineSuccess, baselineErr := deps.Rec.UpdateCounts()

	ticker := time.NewTicker(saturationSampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ""
		case now := <-ticker.C:
			if now.Sub(windowStart) > saturationStopWindow {
				baselineSuccess, baselineErr = deps.Rec.UpdateCounts()
				windowStart = now
			}
			s, e := deps.Rec.UpdateCounts()
			totalInWindow := (s - baselineSuccess) + (e - baselineErr)
			var errRate float64
			if totalInWindow > 0 {
				errRate = float64(e-baselineErr) / float64(totalInWindow)
			}
			tail := deps.Rec.UpdateLatencyTail(int(totalInWindow))
			p99 := computeWindowP99(tail, 0)

			elapsed := now.Sub(windowStart).Seconds()
			if elapsed < 1 {
				elapsed = 1
			}
			achieved := float64(totalInWindow) / elapsed
			snap := WindowSnapshot{
				AchievedRate: achieved,
				TargetRate:   target,
				ErrorRate:    errRate,
				P99Latency:   p99,
				SampleCount:  totalInWindow,
			}
			if fired := tracker.Check(now, snap, conds); fired != "" {
				return fired
			}
		}
	}
}
