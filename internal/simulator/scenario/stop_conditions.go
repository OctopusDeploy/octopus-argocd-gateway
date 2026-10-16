package scenario

import (
	"slices"
	"time"
)

type StopCondition struct {
	Name   string
	Window time.Duration
	Eval   func(snap WindowSnapshot) bool
}

type WindowSnapshot struct {
	AchievedRate float64
	TargetRate   float64
	ErrorRate    float64
	P99Latency   time.Duration
	SampleCount  int64
}

func SaturationStopConditions() []StopCondition {
	return []StopCondition{
		{
			Name:   "error_rate_above_1pct_for_30s",
			Window: 30 * time.Second,
			Eval:   func(s WindowSnapshot) bool { return s.ErrorRate > 0.01 },
		},
		{
			Name:   "achieved_below_90pct_target_for_60s",
			Window: 60 * time.Second,
			Eval: func(s WindowSnapshot) bool {
				if s.TargetRate <= 0 {
					return false
				}
				return s.AchievedRate < 0.90*s.TargetRate
			},
		},
		{
			Name:   "p99_latency_above_10s_for_30s",
			Window: 30 * time.Second,
			Eval:   func(s WindowSnapshot) bool { return s.P99Latency > 10*time.Second },
		},
	}
}

type stopTracker struct {
	firstTrueAt map[string]time.Time
}

func newStopTracker() *stopTracker {
	return &stopTracker{firstTrueAt: make(map[string]time.Time)}
}

func (t *stopTracker) Check(now time.Time, snap WindowSnapshot, conds []StopCondition) string {
	for _, c := range conds {
		if c.Eval(snap) {
			if start, ok := t.firstTrueAt[c.Name]; ok {
				if now.Sub(start) >= c.Window {
					return c.Name
				}
			} else {
				t.firstTrueAt[c.Name] = now
			}
		} else {
			delete(t.firstTrueAt, c.Name)
		}
	}
	return ""
}

func computeWindowP99(all []time.Duration, fromIdx int) time.Duration {
	if fromIdx >= len(all) {
		return 0
	}
	slice := slices.Clone(all[fromIdx:])
	slices.Sort(slice)
	if len(slice) == 0 {
		return 0
	}
	idx := int(float64(len(slice)-1) * 0.99)
	idx = max(idx, 0)
	if idx >= len(slice) {
		idx = len(slice) - 1
	}
	return slice[idx]
}
