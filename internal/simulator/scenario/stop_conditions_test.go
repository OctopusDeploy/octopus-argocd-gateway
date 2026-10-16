package scenario

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStopTracker_FiresOnlyAfterFullWindow(t *testing.T) {
	cond := StopCondition{
		Name:   "err_gt_1pct",
		Window: 30 * time.Second,
		Eval:   func(s WindowSnapshot) bool { return s.ErrorRate > 0.01 },
	}
	tracker := newStopTracker()
	start := time.Unix(1_700_000_000, 0)

	// Condition is true, but not for long enough yet.
	for s := range 29 {
		now := start.Add(time.Duration(s) * time.Second)
		fired := tracker.Check(now, WindowSnapshot{ErrorRate: 0.05}, []StopCondition{cond})
		assert.Empty(t, fired, "fired too early at s=%d", s)
	}
	// At exactly the window length it fires.
	fired := tracker.Check(start.Add(30*time.Second), WindowSnapshot{ErrorRate: 0.05}, []StopCondition{cond})
	assert.Equal(t, "err_gt_1pct", fired)
}

func TestStopTracker_ResetsWhenConditionFalse(t *testing.T) {
	cond := StopCondition{
		Name:   "err_gt_1pct",
		Window: 30 * time.Second,
		Eval:   func(s WindowSnapshot) bool { return s.ErrorRate > 0.01 },
	}
	tracker := newStopTracker()
	start := time.Unix(1_700_000_000, 0)

	tracker.Check(start, WindowSnapshot{ErrorRate: 0.05}, []StopCondition{cond})
	tracker.Check(start.Add(15*time.Second), WindowSnapshot{ErrorRate: 0.05}, []StopCondition{cond})

	// Condition drops to false — streak should reset.
	tracker.Check(start.Add(16*time.Second), WindowSnapshot{ErrorRate: 0.0}, []StopCondition{cond})

	// Need another full 30s streak starting from the next true sample.
	tracker.Check(start.Add(20*time.Second), WindowSnapshot{ErrorRate: 0.05}, []StopCondition{cond})
	fired := tracker.Check(start.Add(40*time.Second), WindowSnapshot{ErrorRate: 0.05}, []StopCondition{cond})
	assert.Empty(t, fired, "must not fire on the old streak")
	fired = tracker.Check(start.Add(50*time.Second), WindowSnapshot{ErrorRate: 0.05}, []StopCondition{cond})
	assert.Equal(t, "err_gt_1pct", fired, "fires once new streak reaches 30s")
}

func TestStopTracker_ReturnsFirstFiring(t *testing.T) {
	c1 := StopCondition{
		Name: "err", Window: 10 * time.Second,
		Eval: func(s WindowSnapshot) bool { return s.ErrorRate > 0.01 },
	}
	c2 := StopCondition{
		Name: "p99", Window: 5 * time.Second,
		Eval: func(s WindowSnapshot) bool { return s.P99Latency > 10*time.Second },
	}
	tracker := newStopTracker()
	start := time.Unix(1_700_000_000, 0)

	snap := WindowSnapshot{ErrorRate: 0.1, P99Latency: 15 * time.Second}
	tracker.Check(start, snap, []StopCondition{c1, c2})
	// c2's 5s window fires first.
	fired := tracker.Check(start.Add(5*time.Second), snap, []StopCondition{c1, c2})
	assert.Equal(t, "p99", fired)
}

func TestSaturationStopConditions_MatchWhimsicalDoc(t *testing.T) {
	conds := SaturationStopConditions()
	assert.Len(t, conds, 3)

	byName := map[string]StopCondition{}
	for _, c := range conds {
		byName[c.Name] = c
	}

	errCond := byName["error_rate_above_1pct_for_30s"]
	assert.Equal(t, 30*time.Second, errCond.Window)
	assert.True(t, errCond.Eval(WindowSnapshot{ErrorRate: 0.02}))
	assert.False(t, errCond.Eval(WindowSnapshot{ErrorRate: 0.005}))

	achCond := byName["achieved_below_90pct_target_for_60s"]
	assert.Equal(t, 60*time.Second, achCond.Window)
	assert.True(t, achCond.Eval(WindowSnapshot{AchievedRate: 80, TargetRate: 100}))
	assert.False(t, achCond.Eval(WindowSnapshot{AchievedRate: 95, TargetRate: 100}))
	assert.False(t, achCond.Eval(WindowSnapshot{AchievedRate: 0, TargetRate: 0}),
		"zero target must not count as below target")

	p99Cond := byName["p99_latency_above_10s_for_30s"]
	assert.Equal(t, 30*time.Second, p99Cond.Window)
	assert.True(t, p99Cond.Eval(WindowSnapshot{P99Latency: 11 * time.Second}))
	assert.False(t, p99Cond.Eval(WindowSnapshot{P99Latency: 9 * time.Second}))
}

func TestComputeWindowP99_SimpleSequence(t *testing.T) {
	var xs []time.Duration
	for i := 1; i <= 100; i++ {
		xs = append(xs, time.Duration(i)*time.Millisecond)
	}
	// Full window: p99 of 1..100ms (nearest-rank) is 99ms.
	got := computeWindowP99(xs, 0)
	assert.Equal(t, 99*time.Millisecond, got)

	// Window starting at idx 50 (51..100ms): p99 is ~99ms still.
	got = computeWindowP99(xs, 50)
	assert.GreaterOrEqual(t, got, 99*time.Millisecond)

	// Empty window.
	assert.Equal(t, time.Duration(0), computeWindowP99(xs, len(xs)))
}
