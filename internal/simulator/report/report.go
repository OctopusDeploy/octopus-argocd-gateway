package report

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"sync"
	"time"
)

type Recorder struct {
	mu              sync.Mutex
	updateLatencies []time.Duration
	syncLatencies   []time.Duration
	errorsByCode    map[string]int64
	successes       int64
}

func NewRecorder() *Recorder {
	return &Recorder{errorsByCode: make(map[string]int64)}
}

func (r *Recorder) RecordUpdate(latency time.Duration, grpcCode string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updateLatencies = append(r.updateLatencies, latency)
	if grpcCode == "" {
		r.successes++
	} else {
		r.errorsByCode[grpcCode]++
	}
}

func (r *Recorder) RecordSyncReply(latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.syncLatencies = append(r.syncLatencies, latency)
}

func (r *Recorder) UpdateLatencyTail(n int) []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := len(r.updateLatencies)
	if n <= 0 || n > total {
		n = total
	}
	out := make([]time.Duration, n)
	copy(out, r.updateLatencies[total-n:])
	return out
}

func (r *Recorder) UpdateCounts() (success int64, errors int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var e int64
	for _, v := range r.errorsByCode {
		e += v
	}
	return r.successes, e
}

type LatencyStats struct {
	Count  int64   `json:"count"`
	MinMs  float64 `json:"min_ms"`
	MaxMs  float64 `json:"max_ms"`
	MeanMs float64 `json:"mean_ms"`
	P50Ms  float64 `json:"p50_ms"`
	P95Ms  float64 `json:"p95_ms"`
	P99Ms  float64 `json:"p99_ms"`
}

type SaturationResult struct {
	StopCondition  string  `json:"stop_condition,omitempty"`
	LastStableRate float64 `json:"last_stable_rate,omitempty"`
	LastTargetRate float64 `json:"last_target_rate,omitempty"`
	RampSteps      int     `json:"ramp_steps,omitempty"`
}

type Report struct {
	Scenario         string            `json:"scenario"`
	Config           map[string]any    `json:"config"`
	StartedAt        time.Time         `json:"started_at"`
	DurationSeconds  float64           `json:"duration_seconds"`
	Gateways         int               `json:"gateways"`
	DeadGateways     int               `json:"dead_gateways"`
	UpdateSuccesses  int64             `json:"update_successes"`
	UpdateErrors     int64             `json:"update_errors"`
	ErrorsByCode     map[string]int64  `json:"errors_by_code,omitempty"`
	AchievedRate     float64           `json:"achieved_rate_per_s"`
	ErrorRate        float64           `json:"error_rate"`
	UpdateLatency    *LatencyStats     `json:"update_latency,omitempty"`
	SyncsReceived    int64             `json:"syncs_received"`
	SyncReplyLatency *LatencyStats     `json:"sync_reply_latency,omitempty"`
	Saturation       *SaturationResult `json:"saturation,omitempty"`
}

func (r *Recorder) Build(scenario string, config map[string]any, startedAt time.Time, gateways, dead int) *Report {
	r.mu.Lock()
	defer r.mu.Unlock()

	duration := time.Since(startedAt).Seconds()
	errors := int64(0)
	for _, v := range r.errorsByCode {
		errors += v
	}
	total := r.successes + errors
	errorRate := 0.0
	if total > 0 {
		errorRate = float64(errors) / float64(total)
	}
	achieved := 0.0
	if duration > 0 {
		achieved = float64(total) / duration
	}

	rep := &Report{
		Scenario:        scenario,
		Config:          config,
		StartedAt:       startedAt,
		DurationSeconds: duration,
		Gateways:        gateways,
		DeadGateways:    dead,
		UpdateSuccesses: r.successes,
		UpdateErrors:    errors,
		AchievedRate:    achieved,
		ErrorRate:       errorRate,
	}
	if len(r.errorsByCode) > 0 {
		rep.ErrorsByCode = maps.Clone(r.errorsByCode)
	}
	if len(r.updateLatencies) > 0 {
		rep.UpdateLatency = summarise(r.updateLatencies)
	}
	if len(r.syncLatencies) > 0 {
		rep.SyncReplyLatency = summarise(r.syncLatencies)
		rep.SyncsReceived = int64(len(r.syncLatencies))
	}
	return rep
}

func Write(rep *Report, path string) error {
	var w io.Writer = os.Stdout
	if path != "" && path != "-" {
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("create report file %q: %w", path, err)
		}
		defer f.Close()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func summarise(samples []time.Duration) *LatencyStats {
	if len(samples) == 0 {
		return nil
	}
	ss := slices.Clone(samples)
	slices.Sort(ss)

	var sum time.Duration
	for _, v := range ss {
		sum += v
	}
	toMs := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	return &LatencyStats{
		Count:  int64(len(ss)),
		MinMs:  toMs(ss[0]),
		MaxMs:  toMs(ss[len(ss)-1]),
		MeanMs: toMs(sum / time.Duration(len(ss))),
		P50Ms:  toMs(percentile(ss, 0.50)),
		P95Ms:  toMs(percentile(ss, 0.95)),
		P99Ms:  toMs(percentile(ss, 0.99)),
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	idx = max(idx, 0)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
