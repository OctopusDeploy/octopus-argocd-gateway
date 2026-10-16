package applicationevents

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testingclock "k8s.io/utils/clock/testing"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/metrics"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/testutilities"
)

// fakeHandler is a configurable inner handler for exercising the dispatcher.
type fakeHandler struct {
	fn func(*v1alpha1.ApplicationWatchEvent) error
}

func (f *fakeHandler) ProcessMessage(evt *v1alpha1.ApplicationWatchEvent) error {
	return f.fn(evt)
}

// countingMetrics counts dropped events by reason. Embeds NoopMetrics for the rest.
type countingMetrics struct {
	metrics.NoopMetrics
	mu       sync.Mutex
	dropped  map[string]int
	buffered int
}

func newCountingMetrics() *countingMetrics {
	return &countingMetrics{
		dropped:  map[string]int{},
		buffered: 0,
	}
}

func (c *countingMetrics) IncDispatcherEventsDropped(_, reason string) {
	c.mu.Lock()
	c.dropped[reason]++
	c.mu.Unlock()
}

func (c *countingMetrics) droppedCount(reason string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dropped[reason]
}

func (c *countingMetrics) IncDispatcherEventsBuffered(_ string) {
	c.mu.Lock()
	c.buffered++
	c.mu.Unlock()
}

func (c *countingMetrics) bufferedCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buffered
}

func appEvent(eventType watch.EventType, namespace, name, uid string) *v1alpha1.ApplicationWatchEvent {
	return &v1alpha1.ApplicationWatchEvent{
		Type: eventType,
		Application: v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       types.UID(uid),
			},
		},
	}
}

func newTestDispatcher(t *testing.T, inner *fakeHandler, m metrics.Metrics, cfg DispatcherConfig) *DispatchingHandler {
	t.Helper()
	return NewDispatchingHandler(inner, testutilities.CreateTestLogger(), m, "test", cfg)
}

// waitFor polls cond until it returns true or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func (d *DispatchingHandler) tombstoneCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.tombstones)
}

// coalesce is a test helper that drives coalesceLocked with the lock held.
func (d *DispatchingHandler) coalesce(key string, evt *v1alpha1.ApplicationWatchEvent) coalesceResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.coalesceLocked(key, evt)
}

// TestDispatcher_TombstoneGate exercises the coalesce/tombstone logic directly, without workers.
func TestDispatcher_TombstoneGate(t *testing.T) {
	d := newTestDispatcher(t, &fakeHandler{}, metrics.NoopMetrics{}, NewDispatcherConfig())
	key := "argocd/app"

	// An add with no tombstone is admitted.
	if res := d.coalesce(key, appEvent(watch.Added, "argocd", "app", "uid-A")); !res.enqueue {
		t.Fatal("expected add to be admitted when no tombstone exists")
	}

	// A delete records a tombstone immediately (at ingestion) and is admitted.
	deleted := appEvent(watch.Deleted, "argocd", "app", "uid-A")
	if res := d.coalesce(key, deleted); !res.enqueue {
		t.Fatal("expected delete to be admitted")
	}
	if d.tombstoneCount() != 1 {
		t.Fatalf("expected delete to record a tombstone, got %d", d.tombstoneCount())
	}

	// A stale update to the same instance (uid-A) must be dropped.
	staleUpdate := appEvent(watch.Modified, "argocd", "app", "uid-A")
	if res := d.coalesce(key, staleUpdate); res.enqueue {
		t.Error("expected stale update to deleted instance (uid-A) to be dropped")
	}

	// A delete is always admitted, even with a tombstone present.
	if res := d.coalesce(key, deleted); !res.enqueue {
		t.Error("expected delete to always be admitted")
	}

	// A recreate (new uid) must be admitted and clear the tombstone.
	recreated := appEvent(watch.Added, "argocd", "app", "uid-B")
	if res := d.coalesce(key, recreated); !res.enqueue {
		t.Fatal("expected recreate (uid-B) to be admitted")
	}
	if d.tombstoneCount() != 0 {
		t.Errorf("expected tombstone cleared after recreate, got %d", d.tombstoneCount())
	}

	// After clearing, the previously-stale uid-A update would now be admitted again.
	if res := d.coalesce(key, staleUpdate); !res.enqueue {
		t.Error("expected update to be admitted after tombstone cleared")
	}
}

// TestDispatcher_StaleUpdateDoesNotEvictPendingDelete verifies that a stale same-UID update arriving
// while a delete is still pending is dropped without replacing the pending delete, so the delete is
// still the event that gets processed. This closes the coalesce-away window.
func TestDispatcher_StaleUpdateDoesNotEvictPendingDelete(t *testing.T) {
	d := newTestDispatcher(t, &fakeHandler{}, metrics.NoopMetrics{}, NewDispatcherConfig())
	key := "argocd/app"

	if res := d.coalesce(key, appEvent(watch.Deleted, "argocd", "app", "uid-A")); !res.enqueue {
		t.Fatal("expected delete to be admitted")
	}

	if res := d.coalesce(key, appEvent(watch.Modified, "argocd", "app", "uid-A")); res.enqueue {
		t.Fatal("expected stale update to be dropped, not enqueued")
	}

	got := d.takePending(key)
	if got == nil || got.Type != watch.Deleted {
		t.Fatalf("expected pending event to remain the delete, got %+v", got)
	}
}

// TestDispatcher_StaleUpdateDuringDeleteRetryIsDropped verifies that while a delete is waiting out a
// retry backoff (restored to pending after a failed send), a stale same-UID update is dropped rather
// than evicting the pending delete.
func TestDispatcher_StaleUpdateDuringDeleteRetryIsDropped(t *testing.T) {
	d := newTestDispatcher(t, &fakeHandler{}, metrics.NoopMetrics{}, NewDispatcherConfig())
	key := "argocd/app"

	deleted := appEvent(watch.Deleted, "argocd", "app", "uid-A")
	_ = d.ProcessMessage(deleted)

	// Simulate a worker taking the delete and its send failing, so it is restored for retry.
	if got := d.takePending(key); got == nil || got.Type != watch.Deleted {
		t.Fatalf("expected to take the pending delete, got %+v", got)
	}
	d.restorePending(key, deleted)

	// A stale update for the deleted instance arrives during the backoff.
	if err := d.ProcessMessage(appEvent(watch.Modified, "argocd", "app", "uid-A")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The pending event is still the delete; the stale update did not evict it.
	if got := d.takePending(key); got == nil || got.Type != watch.Deleted {
		t.Fatalf("expected pending delete to survive the stale update, got %+v", got)
	}
}

// TestDispatcher_DropsUpdateAfterDelete is the headline test: an update for an already-deleted
// instance is dropped end-to-end through the worker pool.
func TestDispatcher_DropsUpdateAfterDelete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var recorded []*v1alpha1.ApplicationWatchEvent
	inner := &fakeHandler{fn: func(evt *v1alpha1.ApplicationWatchEvent) error {
		mu.Lock()
		recorded = append(recorded, evt)
		mu.Unlock()
		return nil
	}}
	recordedLen := func() int { mu.Lock(); defer mu.Unlock(); return len(recorded) }

	cm := newCountingMetrics()
	d := newTestDispatcher(t, inner, cm, NewDispatcherConfig(WithThreadiness(1)))
	d.Start(ctx)

	// Process a delete; wait until its tombstone is recorded.
	_ = d.ProcessMessage(appEvent(watch.Deleted, "argocd", "app", "uid-A"))
	waitFor(t, time.Second, func() bool { return recordedLen() == 1 && d.tombstoneCount() == 1 })

	// A stale update to the deleted instance must be dropped.
	_ = d.ProcessMessage(appEvent(watch.Modified, "argocd", "app", "uid-A"))
	waitFor(t, time.Second, func() bool { return cm.droppedCount("update_after_delete") == 1 })

	if got := recordedLen(); got != 1 {
		t.Errorf("expected only the delete to be processed, got %d events", got)
	}
}

// TestDispatcher_AdmitsRecreateAfterDelete verifies a recreate (new uid) is processed normally.
func TestDispatcher_AdmitsRecreateAfterDelete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var recorded []*v1alpha1.ApplicationWatchEvent
	inner := &fakeHandler{fn: func(evt *v1alpha1.ApplicationWatchEvent) error {
		mu.Lock()
		recorded = append(recorded, evt)
		mu.Unlock()
		return nil
	}}
	lastUID := func() types.UID {
		mu.Lock()
		defer mu.Unlock()
		return recorded[len(recorded)-1].Application.UID
	}
	recordedLen := func() int { mu.Lock(); defer mu.Unlock(); return len(recorded) }

	d := newTestDispatcher(t, inner, metrics.NoopMetrics{}, NewDispatcherConfig(WithThreadiness(1)))
	d.Start(ctx)

	// Wait until the delete is actually processed (not merely tombstoned at ingestion) before sending
	// the recreate, otherwise the recreate would legitimately coalesce the not-yet-processed delete.
	_ = d.ProcessMessage(appEvent(watch.Deleted, "argocd", "app", "uid-A"))
	waitFor(t, time.Second, func() bool { return recordedLen() == 1 })

	_ = d.ProcessMessage(appEvent(watch.Added, "argocd", "app", "uid-B"))
	waitFor(t, time.Second, func() bool { return recordedLen() == 2 })

	if lastUID() != types.UID("uid-B") {
		t.Errorf("expected recreate (uid-B) to be processed, last processed uid was %q", lastUID())
	}
	if d.tombstoneCount() != 0 {
		t.Errorf("expected tombstone cleared after recreate, got %d", d.tombstoneCount())
	}
}

// TestDispatcher_RetriesThenSucceeds verifies a transient send failure is retried.
func TestDispatcher_RetriesThenSucceeds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var attempts int32
	var mu sync.Mutex
	var recorded []*v1alpha1.ApplicationWatchEvent
	inner := &fakeHandler{fn: func(evt *v1alpha1.ApplicationWatchEvent) error {
		if atomic.AddInt32(&attempts, 1) == 1 {
			return errors.New("transient failure")
		}
		mu.Lock()
		recorded = append(recorded, evt)
		mu.Unlock()
		return nil
	}}
	recordedLen := func() int { mu.Lock(); defer mu.Unlock(); return len(recorded) }

	d := newTestDispatcher(t, inner, metrics.NoopMetrics{}, NewDispatcherConfig(WithThreadiness(1), WithMaxRetries(3)))
	d.Start(ctx)

	_ = d.ProcessMessage(appEvent(watch.Modified, "argocd", "app", "uid-A"))
	waitFor(t, 2*time.Second, func() bool { return recordedLen() == 1 })

	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Errorf("expected 2 attempts (1 fail + 1 success), got %d", got)
	}
}

// TestDispatcher_DropsAfterMaxRetries verifies an event is dropped once retries are exhausted.
func TestDispatcher_DropsAfterMaxRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inner := &fakeHandler{fn: func(*v1alpha1.ApplicationWatchEvent) error {
		return errors.New("permanent failure")
	}}

	cm := newCountingMetrics()
	d := newTestDispatcher(t, inner, cm, NewDispatcherConfig(WithThreadiness(1), WithMaxRetries(2)))
	d.Start(ctx)

	_ = d.ProcessMessage(appEvent(watch.Modified, "argocd", "app", "uid-A"))
	waitFor(t, 2*time.Second, func() bool { return cm.droppedCount("max_retries") == 1 })
}

// TestDispatcher_CoalescesBurst verifies that events piling up for one application while it is being
// processed collapse to the latest, preserving order around the in-flight event.
func TestDispatcher_CoalescesBurst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var recorded []*v1alpha1.ApplicationWatchEvent
	firstSeen := make(chan struct{})
	release := make(chan struct{})
	var calls int32
	inner := &fakeHandler{fn: func(evt *v1alpha1.ApplicationWatchEvent) error {
		mu.Lock()
		recorded = append(recorded, evt)
		mu.Unlock()
		if atomic.AddInt32(&calls, 1) == 1 {
			close(firstSeen)
			<-release // hold the first event in-flight
		}
		return nil
	}}
	recordedNames := func() []string {
		mu.Lock()
		defer mu.Unlock()
		names := make([]string, len(recorded))
		for i, e := range recorded {
			names[i] = e.Application.ResourceVersion
		}
		return names
	}

	d := newTestDispatcher(t, inner, metrics.NoopMetrics{}, NewDispatcherConfig(WithThreadiness(1)))
	d.Start(ctx)

	v1 := appEvent(watch.Modified, "argocd", "app", "uid-A")
	v1.Application.ResourceVersion = "1"
	_ = d.ProcessMessage(v1)
	<-firstSeen // worker is now blocked processing v1

	// Pile up two more updates for the same app while v1 is in-flight; they coalesce to v3.
	v2 := appEvent(watch.Modified, "argocd", "app", "uid-A")
	v2.Application.ResourceVersion = "2"
	v3 := appEvent(watch.Modified, "argocd", "app", "uid-A")
	v3.Application.ResourceVersion = "3"
	_ = d.ProcessMessage(v2)
	_ = d.ProcessMessage(v3)

	close(release)
	waitFor(t, time.Second, func() bool { return atomic.LoadInt32(&calls) == 2 })

	got := recordedNames()
	if len(got) != 2 || got[0] != "1" || got[1] != "3" {
		t.Errorf("expected coalesced processing [1 3], got %v", got)
	}
}

// TestDispatcher_DrainsOnShutdown verifies an enqueued event is processed during graceful shutdown.
func TestDispatcher_DrainsOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var processed int32
	inner := &fakeHandler{fn: func(*v1alpha1.ApplicationWatchEvent) error {
		atomic.AddInt32(&processed, 1)
		return nil
	}}

	d := newTestDispatcher(t, inner, metrics.NoopMetrics{}, NewDispatcherConfig(WithThreadiness(1)))
	d.Start(ctx)

	_ = d.ProcessMessage(appEvent(watch.Deleted, "argocd", "app", "uid-A"))
	cancel()
	d.WaitForDrain()

	if atomic.LoadInt32(&processed) != 1 {
		t.Errorf("expected enqueued event to be drained on shutdown, processed=%d", processed)
	}
}

// TestDispatcher_WaitForDrain_ReturnsWhenDrained verifies the graceful-shutdown wait returns once
// the enqueued event has been delivered and the workers have exited.
func TestDispatcher_WaitForDrain_ReturnsWhenDrained(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var processed int32
	inner := &fakeHandler{fn: func(*v1alpha1.ApplicationWatchEvent) error {
		atomic.AddInt32(&processed, 1)
		return nil
	}}

	// Generous timeout: the drain should complete well before it, exercising the happy path.
	d := newTestDispatcher(
		t,
		inner,
		metrics.NoopMetrics{},
		NewDispatcherConfig(WithThreadiness(1), WithDrainTimeout(5*time.Second)),
	)
	d.Start(ctx)

	_ = d.ProcessMessage(appEvent(watch.Deleted, "argocd", "app", "uid-A"))
	cancel()

	returned := make(chan struct{})
	go func() {
		d.WaitForDrain()
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForDrain did not return after the queue drained")
	}

	if atomic.LoadInt32(&processed) != 1 {
		t.Errorf("expected enqueued event to be drained on shutdown, processed=%d", processed)
	}
}

// TestDispatcher_WaitForDrain_TimesOutOnStuckWorker verifies the wait is bounded: a worker stuck in
// a never-returning send must not block shutdown past drainTimeout.
func TestDispatcher_WaitForDrain_TimesOutOnStuckWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inFlight := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	var once sync.Once
	inner := &fakeHandler{fn: func(*v1alpha1.ApplicationWatchEvent) error {
		once.Do(func() { close(inFlight) })
		<-release // never released within the test window: simulate a hung send
		return nil
	}}

	d := newTestDispatcher(
		t,
		inner,
		metrics.NoopMetrics{},
		NewDispatcherConfig(WithThreadiness(1), WithDrainTimeout(50*time.Millisecond)),
	)
	d.Start(ctx)

	_ = d.ProcessMessage(appEvent(watch.Modified, "argocd", "app", "uid-A"))
	<-inFlight // worker is now blocked inside the inner handler
	cancel()

	start := time.Now()
	returned := make(chan struct{})
	go func() {
		d.WaitForDrain()
		close(returned)
	}()

	select {
	case <-returned:
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("WaitForDrain took %v, expected it to be bounded near drainTimeout", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForDrain did not return despite a stuck worker; the timeout did not bound it")
	}
}

// stubRunner is a minimal runner.Runner used to drive drainingRunner without a real watch stream.
type stubRunner struct {
	ctx     context.Context
	errCh   chan error
	started chan struct{}
}

func (s *stubRunner) Start() {
	close(s.started)
	<-s.ctx.Done() // mimic a watch runner: Start returns when the context is cancelled
}
func (s *stubRunner) Errors() <-chan error     { return s.errCh }
func (s *stubRunner) Context() context.Context { return s.ctx }

// TestDrainingRunner_BlocksUntilDispatcherDrains verifies the wrapper does not return from Start
// until the dispatcher has finished draining, so the gateway's wait group blocks on the drain.
func TestDrainingRunner_BlocksUntilDispatcherDrains(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	processStarted := make(chan struct{})
	finishProcessing := make(chan struct{})
	var processed int32
	var once sync.Once
	inner := &fakeHandler{fn: func(*v1alpha1.ApplicationWatchEvent) error {
		once.Do(func() { close(processStarted) })
		<-finishProcessing
		atomic.AddInt32(&processed, 1)
		return nil
	}}

	d := newTestDispatcher(
		t,
		inner,
		metrics.NoopMetrics{},
		NewDispatcherConfig(WithThreadiness(1), WithDrainTimeout(5*time.Second)),
	)
	d.Start(ctx)

	stub := &stubRunner{ctx: ctx, errCh: make(chan error), started: make(chan struct{})}
	dr := &drainingRunner{inner: stub, dispatcher: d}

	_ = d.ProcessMessage(appEvent(watch.Deleted, "argocd", "app", "uid-A"))
	<-processStarted // an event is mid-flight in the worker

	startReturned := make(chan struct{})
	go func() {
		dr.Start()
		close(startReturned)
	}()

	<-stub.started
	cancel() // stub.Start returns, but the worker is still holding the in-flight event

	select {
	case <-startReturned:
		t.Fatal("drainingRunner.Start returned before the dispatcher finished draining")
	case <-time.After(100 * time.Millisecond):
		// Expected: still draining because the worker is blocked.
	}

	close(finishProcessing) // let the in-flight event complete

	select {
	case <-startReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("drainingRunner.Start did not return after the dispatcher drained")
	}

	if atomic.LoadInt32(&processed) != 1 {
		t.Errorf("expected the in-flight event to be delivered before shutdown completed, processed=%d", processed)
	}
}

// TestDispatcher_SweepEvictsExpiredTombstones verifies TTL-based cleanup.
func TestDispatcher_SweepEvictsExpiredTombstones(t *testing.T) {
	d := newTestDispatcher(t, &fakeHandler{}, metrics.NoopMetrics{}, NewDispatcherConfig(WithTombstoneTTL(time.Minute)))

	now := time.Unix(1_000_000, 0)
	d.now = func() time.Time { return now }

	d.coalesce("argocd/app", appEvent(watch.Deleted, "argocd", "app", "uid-A"))
	if d.tombstoneCount() != 1 {
		t.Fatalf("expected 1 tombstone, got %d", d.tombstoneCount())
	}

	// Not yet expired.
	now = now.Add(30 * time.Second)
	d.sweepTombstones()
	if d.tombstoneCount() != 1 {
		t.Errorf("expected tombstone retained before TTL, got %d", d.tombstoneCount())
	}

	// Past TTL.
	now = now.Add(time.Minute)
	d.sweepTombstones()
	if d.tombstoneCount() != 0 {
		t.Errorf("expected tombstone evicted after TTL, got %d", d.tombstoneCount())
	}
}

func TestDispatcher_BufferingDispatchesLastReceivedEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recordedChan := make(chan *v1alpha1.ApplicationWatchEvent, 100)
	inner := &fakeHandler{fn: func(evt *v1alpha1.ApplicationWatchEvent) error {
		recordedChan <- evt
		return nil
	}}

	fakeClock := testingclock.NewFakeClock(time.Now())

	cfg := NewDispatcherConfig(
		WithBufferDuration(2*time.Second),
		WithClock(fakeClock),
	)

	cm := newCountingMetrics()
	d := newTestDispatcher(t, inner, cm, cfg)
	d.Start(ctx)

	// Modified 1
	modEvent1 := appEvent(watch.Modified, "argocd", "app", "uid-A")
	modEvent1.Application.Generation = 1
	_ = d.ProcessMessage(modEvent1)

	// wait for 0.5s
	fakeClock.Step(500 * time.Millisecond)

	// Modified 2
	modEvent2 := appEvent(watch.Modified, "argocd", "app", "uid-A")
	modEvent2.Application.Generation = 2
	_ = d.ProcessMessage(modEvent2)

	// wait for 1.4 sec
	fakeClock.Step(1400 * time.Millisecond)

	// Modified
	modEvent3 := appEvent(watch.Modified, "argocd", "app", "uid-A")
	modEvent3.Application.Generation = 3
	_ = d.ProcessMessage(modEvent3)

	// wait 101 milliseconds (to take us over 2s total)
	fakeClock.Step(101 * time.Millisecond)

	recordedEvent := <-recordedChan

	if recordedEvent.Application.Generation != modEvent3.Application.Generation {
		t.Errorf(
			"The received event was not the last added generation, got event generation %d",
			recordedEvent.Application.Generation)
	}

	if bufferedCount := cm.bufferedCount(); bufferedCount != 2 {
		t.Errorf("Should have recorded 2 buffered events, recorded %d", bufferedCount)
	}
}

func TestDispatcher_BufferingDisabledAllEventsImmediatelyDispatched(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recordedChan := make(chan *v1alpha1.ApplicationWatchEvent, 100)
	inner := &fakeHandler{fn: func(evt *v1alpha1.ApplicationWatchEvent) error {
		recordedChan <- evt
		return nil
	}}

	fakeClock := testingclock.NewFakeClock(time.Now())

	cfg := NewDispatcherConfig(
		WithBufferDuration(0),
		WithClock(fakeClock),
	)

	cm := newCountingMetrics()
	d := newTestDispatcher(t, inner, cm, cfg)
	d.Start(ctx)

	// Modified 1
	modEvent1 := appEvent(watch.Modified, "argocd", "app", "uid-A")
	modEvent1.Application.Generation = 1
	_ = d.ProcessMessage(modEvent1)

	// wait for 0.5s
	fakeClock.Step(500 * time.Millisecond)
	recordedEvent := <-recordedChan
	if recordedEvent.Application.Generation != modEvent1.Application.Generation {
		t.Error("The event was not immediately dispatched")
	}

	// Modified 2
	modEvent2 := appEvent(watch.Modified, "argocd", "app", "uid-A")
	modEvent2.Application.Generation = 2
	_ = d.ProcessMessage(modEvent2)

	// wait for 1.4 sec
	fakeClock.Step(1400 * time.Millisecond)
	recordedEvent = <-recordedChan
	if recordedEvent.Application.Generation != modEvent2.Application.Generation {
		t.Error("The event was not immediately dispatched")
	}

	// Modified
	modEvent3 := appEvent(watch.Modified, "argocd", "app", "uid-A")
	modEvent3.Application.Generation = 3
	_ = d.ProcessMessage(modEvent3)

	// wait 101 milliseconds (to take us over 2s total)
	fakeClock.Step(101 * time.Millisecond)
	recordedEvent = <-recordedChan
	if recordedEvent.Application.Generation != modEvent3.Application.Generation {
		t.Error("The event was not immediately dispatched")
	}

	if bufferedCount := cm.bufferedCount(); bufferedCount != 0 {
		t.Errorf("Should have recorded 0 buffered events, recorded %d", bufferedCount)
	}
}

func TestDispatcher_OutOfOrderBufferedEventIsDropped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recordedChan := make(chan *v1alpha1.ApplicationWatchEvent, 100)
	inner := &fakeHandler{fn: func(evt *v1alpha1.ApplicationWatchEvent) error {
		recordedChan <- evt
		return nil
	}}

	fakeClock := testingclock.NewFakeClock(time.Now())

	cfg := NewDispatcherConfig(
		WithBufferDuration(2*time.Second),
		WithClock(fakeClock),
	)

	cm := newCountingMetrics()
	d := newTestDispatcher(t, inner, cm, cfg)
	d.Start(ctx)

	// Modified 1
	modEvent1 := appEvent(watch.Modified, "argocd", "app", "uid-A")
	modEvent1.Application.Generation = 5
	_ = d.ProcessMessage(modEvent1)

	// wait for 0.5s
	fakeClock.Step(500 * time.Millisecond)

	// Modified 2
	modEvent2 := appEvent(watch.Modified, "argocd", "app", "uid-A")
	modEvent2.Application.Generation = 4
	_ = d.ProcessMessage(modEvent2)

	// wait for 1.4 sec
	fakeClock.Step(1400 * time.Millisecond)

	// Modified
	modEvent3 := appEvent(watch.Modified, "argocd", "app", "uid-A")
	modEvent3.Application.Generation = 6
	_ = d.ProcessMessage(modEvent3)

	// wait 101 milliseconds (to take us over 2s total)
	fakeClock.Step(101 * time.Millisecond)

	recordedEvent := <-recordedChan
	if recordedEvent.Application.Generation != modEvent3.Application.Generation {
		t.Errorf(
			"The received event was not the last added generation, got event generation %d",
			recordedEvent.Application.Generation,
		)
	}

	bufferedCount := cm.bufferedCount()
	if bufferedCount != 1 {
		t.Errorf("Should have recorded 1 buffered events, recorded %d", bufferedCount)
	}

	droppedCount := cm.droppedCount("out_of_order")
	if droppedCount != 1 {
		t.Errorf("Should have recorded 1 dropped event for out_of_order, recorded %d", droppedCount)
	}
}

func TestDispatcher_AddedEventsAreImmediatelyDispatched(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recordedChan := make(chan *v1alpha1.ApplicationWatchEvent, 100)
	inner := &fakeHandler{fn: func(evt *v1alpha1.ApplicationWatchEvent) error {
		recordedChan <- evt
		return nil
	}}

	fakeClock := testingclock.NewFakeClock(time.Now())

	cfg := NewDispatcherConfig(
		WithBufferDuration(2*time.Second),
		WithClock(fakeClock),
	)

	cm := newCountingMetrics()
	d := newTestDispatcher(t, inner, cm, cfg)
	d.Start(ctx)

	// Added
	addEvent := appEvent(watch.Added, "argocd", "app", "uid-A")
	addEvent.Application.Generation = 1
	_ = d.ProcessMessage(addEvent)

	// wait for 1 sec
	fakeClock.Step(time.Second)

	recordedEvent := <-recordedChan

	if recordedEvent.Application.Generation != addEvent.Application.Generation {
		t.Errorf(
			"The received event was not the last added generation, got event generation %d",
			recordedEvent.Application.Generation)
	}

	if bufferedCount := cm.bufferedCount(); bufferedCount != 0 {
		t.Errorf("Should have recorded 1 buffered events, recorded %d", bufferedCount)
	}
}

func TestDispatcher_DeletedEventsAreImmediatelyDispatched(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recordedChan := make(chan *v1alpha1.ApplicationWatchEvent, 100)
	inner := &fakeHandler{fn: func(evt *v1alpha1.ApplicationWatchEvent) error {
		recordedChan <- evt
		return nil
	}}

	fakeClock := testingclock.NewFakeClock(time.Now())

	cfg := NewDispatcherConfig(
		WithBufferDuration(2*time.Second),
		WithClock(fakeClock),
	)

	cm := newCountingMetrics()
	d := newTestDispatcher(t, inner, cm, cfg)
	d.Start(ctx)

	// Modified
	modEvent := appEvent(watch.Modified, "argocd", "app", "uid-A")
	modEvent.Application.Generation = 1
	_ = d.ProcessMessage(modEvent)

	// wait for 0.5s
	fakeClock.Step(500 * time.Millisecond)

	// Deleted
	deleteEvent := appEvent(watch.Deleted, "argocd", "app", "uid-A")
	deleteEvent.Application.Generation = 2
	_ = d.ProcessMessage(deleteEvent)

	// wait for 1 sec
	fakeClock.Step(time.Second)

	recordedEvent := <-recordedChan

	if recordedEvent.Application.Generation != deleteEvent.Application.Generation {
		t.Errorf(
			"The received event was not the last added generation, got event generation %d",
			recordedEvent.Application.Generation)
	}
}
