package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecorder_BuildShape(t *testing.T) {
	r := NewRecorder()
	r.RecordUpdate(5*time.Millisecond, "")
	r.RecordUpdate(10*time.Millisecond, "")
	r.RecordUpdate(20*time.Millisecond, "Unavailable")
	r.RecordSyncReply(1 * time.Millisecond)

	rep := r.Build("ingest", map[string]any{"target_rate": 100}, time.Now().Add(-2*time.Second), 4, 1)

	assert.Equal(t, "ingest", rep.Scenario)
	assert.Equal(t, 4, rep.Gateways)
	assert.Equal(t, 1, rep.DeadGateways)
	assert.Equal(t, int64(2), rep.UpdateSuccesses)
	assert.Equal(t, int64(1), rep.UpdateErrors)
	assert.InDelta(t, 1.0/3.0, rep.ErrorRate, 1e-6)
	assert.Equal(t, map[string]int64{"Unavailable": 1}, rep.ErrorsByCode)
	require.NotNil(t, rep.UpdateLatency)
	assert.Equal(t, int64(3), rep.UpdateLatency.Count)
	require.NotNil(t, rep.SyncReplyLatency)
	assert.Equal(t, int64(1), rep.SyncsReceived)
}

func TestRecorder_UpdateLatencyTail(t *testing.T) {
	r := NewRecorder()
	for i := range 10 {
		r.RecordUpdate(time.Duration(i)*time.Millisecond, "")
	}
	tail := r.UpdateLatencyTail(3)
	assert.Equal(t,
		[]time.Duration{7 * time.Millisecond, 8 * time.Millisecond, 9 * time.Millisecond},
		tail,
	)

	all := r.UpdateLatencyTail(0)
	assert.Len(t, all, 10)

	more := r.UpdateLatencyTail(100)
	assert.Len(t, more, 10)
}

func TestWrite_RoundTripsJSON(t *testing.T) {
	r := NewRecorder()
	r.RecordUpdate(2*time.Millisecond, "")
	rep := r.Build("sync-latency", map[string]any{"seed": 1}, time.Now(), 1, 0)

	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	require.NoError(t, Write(rep, path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	for _, key := range []string{
		"scenario", "config", "started_at", "duration_seconds",
		"gateways", "dead_gateways", "update_successes", "update_errors",
		"achieved_rate_per_s", "error_rate", "update_latency", "syncs_received",
	} {
		_, ok := decoded[key]
		assert.Truef(t, ok, "missing key %q in JSON: %s", key, raw)
	}

	assert.True(t, bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")))
}

func TestSummarise_PercentileOrdering(t *testing.T) {
	xs := make([]time.Duration, 100)
	for i := range xs {
		xs[i] = time.Duration(i+1) * time.Millisecond
	}
	stats := summarise(xs)
	require.NotNil(t, stats)
	assert.LessOrEqual(t, stats.MinMs, stats.P50Ms)
	assert.LessOrEqual(t, stats.P50Ms, stats.P95Ms)
	assert.LessOrEqual(t, stats.P95Ms, stats.P99Ms)
	assert.LessOrEqual(t, stats.P99Ms, stats.MaxMs)
}
