package applicationevents

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/util/workqueue"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/metrics"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
)

const (
	// DefaultThreadiness is the number of worker goroutines draining the queue.
	DefaultThreadiness = 10
	// DefaultMaxRetries is how many times a failing event is re-queued before being dropped.
	DefaultMaxRetries = 5
	// DefaultTombstoneTTL is how long a delete tombstone is retained; generous relative to the max retry+resync window.
	DefaultTombstoneTTL = 30 * time.Minute

	// DefaultDrainTimeout bounds how long graceful shutdown waits for the dispatcher to drain; kept below the subscriber-shutdown budget.
	DefaultDrainTimeout = 20 * time.Second

	// DefaultBufferDuration is how long incoming events are buffered before sending to Octopus Server.
	DefaultBufferDuration = 2 * time.Second

	// maintenancePeriod is how often tombstones are swept and queue depth is sampled.
	maintenancePeriod = 1 * time.Minute
	// maxTombstones is a hard cap that backstops pathological churn; entries are tiny.
	maxTombstones = 100_000
)

// tombstone records the last deleted instance of an application by QualifiedName.
type tombstone struct {
	uid       types.UID
	deletedAt time.Time
}

// DispatchingHandler decouples the ArgoCD stream reader from event processing via a workqueue-backed worker pool.
type DispatchingHandler struct {
	inner          runner.MessageHandler[*v1alpha1.ApplicationWatchEvent]
	queue          workqueue.TypedRateLimitingInterface[string]
	threadiness    int
	maxRetries     int
	ttl            time.Duration
	drainTimeout   time.Duration
	bufferDuration time.Duration
	now            func() time.Time
	logger         *slog.Logger
	metrics        metrics.Metrics
	handlerName    string

	mu         sync.Mutex
	pending    map[string]*v1alpha1.ApplicationWatchEvent // QualifiedName -> latest unprocessed event
	tombstones map[string]tombstone                       // QualifiedName -> last delete

	wg sync.WaitGroup
}

func NewDispatchingHandler(
	inner runner.MessageHandler[*v1alpha1.ApplicationWatchEvent],
	logger *slog.Logger,
	m metrics.Metrics,
	handlerName string,
	cfg DispatcherConfig,
) *DispatchingHandler {
	if m == nil {
		m = metrics.NoopMetrics{}
	}

	cfg.QueueConfig.Name = handlerName

	queue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[string](),
		cfg.QueueConfig,
	)

	return &DispatchingHandler{
		inner:          inner,
		queue:          queue,
		threadiness:    cfg.Threadiness,
		maxRetries:     cfg.MaxRetries,
		ttl:            cfg.TombstoneTTL,
		drainTimeout:   cfg.DrainTimeout,
		bufferDuration: cfg.BufferDuration,
		now:            time.Now, // use a field for time.Now so it can be faked in tests; default to real time.
		logger:         logger,
		metrics:        m,
		handlerName:    handlerName,
		pending:        make(map[string]*v1alpha1.ApplicationWatchEvent),
		tombstones:     make(map[string]tombstone),
	}
}

func (d *DispatchingHandler) Start(ctx context.Context) {
	for i := 0; i < d.threadiness; i++ {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			for d.processNext() {
			}
		}()
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.runMaintenance(ctx)
	}()

	go func() {
		<-ctx.Done()
		// make sure any buffered events are flushed
		d.flushBuffered()
		d.queue.ShutDownWithDrain()
	}()
}

func (d *DispatchingHandler) flushBuffered() {
	d.mu.Lock()
	keys := make([]string, 0, len(d.pending))
	// get a list of all the pending keys
	for k := range d.pending {
		keys = append(keys, k)
	}
	d.mu.Unlock()
	// re-add the pending keys. this will dedupe and immediately execute
	for _, k := range keys {
		d.queue.Add(k)
	}
}

func (d *DispatchingHandler) WaitForDrain() {
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(d.drainTimeout):
		d.logger.Warn("Dispatcher drain timed out before all workers exited; abandoning remaining events",
			slog.String("handler", d.handlerName),
			slog.Duration("timeout", d.drainTimeout),
			slog.Int("queueDepth", d.queue.Len()))
	}
}

// ProcessMessage records the latest event and enqueues the key without blocking.
// Tombstones are written at ingestion so stale same-UID updates are dropped even while the delete is still queued.
func (d *DispatchingHandler) ProcessMessage(evt *v1alpha1.ApplicationWatchEvent) error {
	key := evt.Application.QualifiedName()

	d.mu.Lock()
	res := d.coalesceLocked(key, evt)
	tombstoneCount := len(d.tombstones)
	d.mu.Unlock()

	if res.tombstonesChanged {
		d.metrics.SetDispatcherTombstones(d.handlerName, tombstoneCount)
	}

	if !res.enqueue {
		if !res.isOlderApplicationGeneration {
			d.metrics.IncDispatcherEventsDropped(d.handlerName, "update_after_delete")
			d.logger.Info("Dropping stale update for an already-deleted application",
				slog.String("application", key),
				slog.Any("eventType", evt.Type))

		} else {
			d.metrics.IncDispatcherEventsDropped(d.handlerName, "out_of_order")
			d.logger.Info("Dropping incoming update as a newer update has been buffered",
				slog.String("application", key),
				slog.Any("eventType", evt.Type))
		}
		return nil
	}

	// if buffering is enabled, queue the message for later
	// we don't buffer Added or Deleted events because we want them to be immediately dispatched
	if (evt.Type != watch.Deleted && evt.Type != watch.Added) && d.bufferDuration > 0 {
		d.queue.AddAfter(key, d.bufferDuration)
	} else {
		d.queue.Add(key)
	}
	return nil
}

type coalesceResult struct {
	enqueue                      bool // false means the event was a stale update to a deleted instance and was dropped
	tombstonesChanged            bool // a tombstone was added or cleared, so the gauge should be refreshed
	isOlderApplicationGeneration bool // indicates if the received message was an older application generation
}

// coalesceLocked records the event as the latest pending state, applying delete/tombstone semantics. Caller must hold d.mu.
// Deletes record a tombstone immediately; same-UID non-deletes after a delete are dropped; different-UID clears the tombstone.
func (d *DispatchingHandler) coalesceLocked(key string, evt *v1alpha1.ApplicationWatchEvent) coalesceResult {
	if evt.Type == watch.Deleted {
		d.tombstones[key] = tombstone{uid: evt.Application.UID, deletedAt: d.now()}
		if len(d.tombstones) > maxTombstones {
			d.evictOldestLocked()
		}
		d.pending[key] = evt
		return coalesceResult{enqueue: true, tombstonesChanged: true}
	}

	if ts, ok := d.tombstones[key]; ok {
		if evt.Application.UID == ts.uid {
			return coalesceResult{} // stale update to the deleted instance: drop, leave pending delete intact
		}
		delete(d.tombstones, key)
		d.pending[key] = evt
		return coalesceResult{enqueue: true, tombstonesChanged: true}
	}

	if d.pending[key] != nil {
		// If the incoming event has a generation value less (earlier) than the stored value
		// we don't want to enqueue it (as it's older data), this will be tracked as dropped
		if evt.Application.Generation < d.pending[key].Application.Generation {
			return coalesceResult{enqueue: false, isOlderApplicationGeneration: true}
		} else {
			// track the number of buffered events
			d.metrics.IncDispatcherEventsBuffered(d.handlerName)
		}
	}

	d.pending[key] = evt
	return coalesceResult{enqueue: true}
}

// processNext handles a single queue item. It returns false once the queue has shut down.
func (d *DispatchingHandler) processNext() bool {
	key, shutdown := d.queue.Get()
	if shutdown {
		return false
	}
	defer d.queue.Done(key)

	evt := d.takePending(key)
	if evt == nil {
		// Already handled by a coalesced pass; nothing to do.
		d.queue.Forget(key)
		return true
	}

	logger := d.logger.With(
		slog.String("application", key),
		slog.Any("eventType", evt.Type),
	)

	if err := d.inner.ProcessMessage(evt); err != nil {
		// Read before AddRateLimited so the logged attempt number reflects the attempt that just failed (1-based).
		requeues := d.queue.NumRequeues(key)
		if requeues < d.maxRetries {
			d.restorePending(key, evt)
			d.queue.AddRateLimited(key)
			logger.Warn("Failed processing application event, requeuing",
				slog.Int("attempt", requeues+1),
				slog.Any("error", err))
		} else {
			d.metrics.IncDispatcherEventsDropped(d.handlerName, "max_retries")
			logger.Error("Max retries reached, dropping application event", slog.Any("error", err))
			d.queue.Forget(key)
		}
		return true
	}

	d.queue.Forget(key)
	return true
}

// takePending atomically reads and clears the latest event for a key.
func (d *DispatchingHandler) takePending(key string) *v1alpha1.ApplicationWatchEvent {
	d.mu.Lock()
	defer d.mu.Unlock()
	evt, ok := d.pending[key]
	if !ok {
		return nil
	}
	delete(d.pending, key)
	return evt
}

// restorePending re-stores an event for retry, but only if no newer event has arrived in the
// meantime (latest-wins).
func (d *DispatchingHandler) restorePending(key string, evt *v1alpha1.ApplicationWatchEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.pending[key]; !ok {
		d.pending[key] = evt
	}
}

// evictOldestLocked removes the single oldest tombstone. Caller must hold d.mu. This is a rarely-hit
// backstop, so an O(n) scan is acceptable.
func (d *DispatchingHandler) evictOldestLocked() {
	var oldestKey string
	var oldestAt time.Time
	first := true
	for k, ts := range d.tombstones {
		if first || ts.deletedAt.Before(oldestAt) {
			oldestKey, oldestAt, first = k, ts.deletedAt, false
		}
	}
	if !first {
		delete(d.tombstones, oldestKey)
	}
}

func (d *DispatchingHandler) runMaintenance(ctx context.Context) {
	ticker := time.NewTicker(maintenancePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.sweepTombstones()
			d.metrics.SetDispatcherQueueDepth(d.handlerName, d.queue.Len())
		}
	}
}

// sweepTombstones evicts tombstones older than the configured TTL.
func (d *DispatchingHandler) sweepTombstones() {
	cutoff := d.now().Add(-d.ttl)
	d.mu.Lock()
	for k, ts := range d.tombstones {
		if ts.deletedAt.Before(cutoff) {
			delete(d.tombstones, k)
		}
	}
	n := len(d.tombstones)
	d.mu.Unlock()
	d.metrics.SetDispatcherTombstones(d.handlerName, n)
}
