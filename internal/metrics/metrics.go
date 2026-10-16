package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/watch"
)

// Metrics defines the interface for recording gateway operational metrics.
type Metrics interface {
	// WatchRunner — ArgoCD events pushed to Octopus
	IncWatchEventsReceived(handler string)
	IncWatchEventsProcessed(handler, result string)
	ObserveWatchEventDuration(handler string, seconds float64)
	IncWatchStreamRestarts(handler string)

	// StreamRunner — Octopus requests handled via ArgoCD
	IncStreamRequestsReceived(handler string)
	IncStreamRequestsProcessed(handler, result string)
	ObserveStreamRequestDuration(handler string, seconds float64)
	IncStreamRestarts(handler string)

	// Application event specifics
	IncApplicationEventsByType(eventType watch.EventType)
	IncApplicationEventsSent(eventType watch.EventType, result string)
	ObserveApplicationEventSendDuration(seconds float64)

	// Application event dispatcher (concurrent processing)
	SetDispatcherQueueDepth(handler string, depth int)
	IncDispatcherEventsDropped(handler, reason string)
	SetDispatcherTombstones(handler string, count int)
	IncDispatcherEventsBuffered(handler string)

	// Resource watchers
	IncActiveResourceWatchers()
	DecActiveResourceWatchers()
	IncResourceTreeUpdates(result string)

	// Leader election
	SetLeader(isLeader bool)
}

// PrometheusMetrics is the live implementation backed by Prometheus counters and histograms.
type PrometheusMetrics struct {
	watchEventsReceived  *prometheus.CounterVec
	watchEventsProcessed *prometheus.CounterVec
	watchEventDuration   *prometheus.HistogramVec
	watchStreamRestarts  *prometheus.CounterVec

	streamRequestsReceived  *prometheus.CounterVec
	streamRequestsProcessed *prometheus.CounterVec
	streamRequestDuration   *prometheus.HistogramVec
	streamRestarts          *prometheus.CounterVec

	applicationEventsByType      *prometheus.CounterVec
	applicationEventsSent        *prometheus.CounterVec
	applicationEventSendDuration *prometheus.HistogramVec

	dispatcherQueueDepth     *prometheus.GaugeVec
	dispatcherEventsDropped  *prometheus.CounterVec
	dispatcherTombstones     *prometheus.GaugeVec
	dispatcherEventsBuffered *prometheus.CounterVec

	activeResourceWatchers prometheus.Gauge
	resourceTreeUpdates    *prometheus.CounterVec

	isLeader prometheus.Gauge
}

func New() Metrics {
	m := &PrometheusMetrics{
		watchEventsReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_watch_events_received_total",
			Help: "Total number of watch events received from ArgoCD.",
		}, []string{"handler"}),

		watchEventsProcessed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_watch_events_processed_total",
			Help: "Total number of watch events processed, by result.",
		}, []string{"handler", "result"}),

		watchEventDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_watch_event_duration_seconds",
			Help:    "Duration of watch event processing in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"handler"}),

		watchStreamRestarts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_watch_stream_restarts_total",
			Help: "Total number of watch stream restarts.",
		}, []string{"handler"}),

		streamRequestsReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_stream_requests_received_total",
			Help: "Total number of stream requests received from Octopus.",
		}, []string{"handler"}),

		streamRequestsProcessed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_stream_requests_processed_total",
			Help: "Total number of stream requests processed, by result.",
		}, []string{"handler", "result"}),

		streamRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_stream_request_duration_seconds",
			Help:    "Duration of stream request processing in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"handler"}),

		streamRestarts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_stream_restarts_total",
			Help: "Total number of bidirectional stream restarts.",
		}, []string{"handler"}),

		applicationEventsByType: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_application_events_by_type_total",
			Help: "Total number of ArgoCD application events received, by event type (ADDED, MODIFIED, DELETED).",
		}, []string{"event_type"}),

		applicationEventsSent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_application_events_sent_total",
			Help: "Total number of application events sent to Octopus Deploy, by result.",
		}, []string{"event_type", "result"}),

		applicationEventSendDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_application_event_send_duration_seconds",
			Help:    "Duration of sending an application event to Octopus Deploy in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{}),

		dispatcherQueueDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gateway_dispatcher_queue_depth",
			Help: "Current number of application keys waiting in the dispatcher workqueue.",
		}, []string{"handler"}),

		dispatcherEventsDropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_dispatcher_events_dropped_total",
			Help: "Total number of application events dropped by the dispatcher, by reason (update_after_delete, max_retries, out_of_order).",
		}, []string{"handler", "reason"}),

		dispatcherTombstones: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gateway_dispatcher_tombstones",
			Help: "Current number of delete tombstones held by the dispatcher.",
		}, []string{"handler"}),

		dispatcherEventsBuffered: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_dispatcher_events_buffered_total",
			Help: "Total number of events buffered by the dispatcher.",
		}, []string{"handler"}),

		activeResourceWatchers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gateway_active_resource_watchers",
			Help: "Number of currently active per-application resource watchers.",
		}),

		resourceTreeUpdates: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_resource_tree_updates_total",
			Help: "Total number of resource tree updates published to Octopus, by result.",
		}, []string{"result"}),

		isLeader: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gateway_is_leader",
			Help: "1 if this replica currently holds the leader-election lease, 0 otherwise.",
		}),
	}

	prometheus.MustRegister(
		m.watchEventsReceived,
		m.watchEventsProcessed,
		m.watchEventDuration,
		m.watchStreamRestarts,
		m.streamRequestsReceived,
		m.streamRequestsProcessed,
		m.streamRequestDuration,
		m.streamRestarts,
		m.applicationEventsByType,
		m.applicationEventsSent,
		m.applicationEventSendDuration,
		m.dispatcherQueueDepth,
		m.dispatcherEventsDropped,
		m.dispatcherTombstones,
		m.dispatcherEventsBuffered,
		m.activeResourceWatchers,
		m.resourceTreeUpdates,
		m.isLeader,
	)

	return m
}

func (m *PrometheusMetrics) IncWatchEventsReceived(handler string) {
	m.watchEventsReceived.WithLabelValues(handler).Inc()
}

func (m *PrometheusMetrics) IncWatchEventsProcessed(handler, result string) {
	m.watchEventsProcessed.WithLabelValues(handler, result).Inc()
}

func (m *PrometheusMetrics) ObserveWatchEventDuration(handler string, seconds float64) {
	m.watchEventDuration.WithLabelValues(handler).Observe(seconds)
}

func (m *PrometheusMetrics) IncWatchStreamRestarts(handler string) {
	m.watchStreamRestarts.WithLabelValues(handler).Inc()
}

func (m *PrometheusMetrics) IncStreamRequestsReceived(handler string) {
	m.streamRequestsReceived.WithLabelValues(handler).Inc()
}

func (m *PrometheusMetrics) IncStreamRequestsProcessed(handler, result string) {
	m.streamRequestsProcessed.WithLabelValues(handler, result).Inc()
}

func (m *PrometheusMetrics) ObserveStreamRequestDuration(handler string, seconds float64) {
	m.streamRequestDuration.WithLabelValues(handler).Observe(seconds)
}

func (m *PrometheusMetrics) IncStreamRestarts(handler string) {
	m.streamRestarts.WithLabelValues(handler).Inc()
}

func (m *PrometheusMetrics) IncApplicationEventsByType(eventType watch.EventType) {
	m.applicationEventsByType.WithLabelValues(string(eventType)).Inc()
}

func (m *PrometheusMetrics) IncApplicationEventsSent(eventType watch.EventType, result string) {
	m.applicationEventsSent.WithLabelValues(string(eventType), result).Inc()
}

func (m *PrometheusMetrics) ObserveApplicationEventSendDuration(seconds float64) {
	m.applicationEventSendDuration.WithLabelValues().Observe(seconds)
}

func (m *PrometheusMetrics) SetDispatcherQueueDepth(handler string, depth int) {
	m.dispatcherQueueDepth.WithLabelValues(handler).Set(float64(depth))
}

func (m *PrometheusMetrics) IncDispatcherEventsDropped(handler, reason string) {
	m.dispatcherEventsDropped.WithLabelValues(handler, reason).Inc()
}

func (m *PrometheusMetrics) SetDispatcherTombstones(handler string, count int) {
	m.dispatcherTombstones.WithLabelValues(handler).Set(float64(count))
}

func (m *PrometheusMetrics) IncDispatcherEventsBuffered(handler string) {
	m.dispatcherEventsBuffered.WithLabelValues(handler).Inc()
}

func (m *PrometheusMetrics) IncActiveResourceWatchers() {
	m.activeResourceWatchers.Inc()
}

func (m *PrometheusMetrics) DecActiveResourceWatchers() {
	m.activeResourceWatchers.Dec()
}

func (m *PrometheusMetrics) IncResourceTreeUpdates(result string) {
	m.resourceTreeUpdates.WithLabelValues(result).Inc()
}

func (m *PrometheusMetrics) SetLeader(isLeader bool) {
	if isLeader {
		m.isLeader.Set(1)
	} else {
		m.isLeader.Set(0)
	}
}

// NoopMetrics is a no-op implementation for use in tests.
type NoopMetrics struct{}

func (NoopMetrics) IncWatchEventsReceived(_ string)                      {}
func (NoopMetrics) IncWatchEventsProcessed(_, _ string)                  {}
func (NoopMetrics) ObserveWatchEventDuration(_ string, _ float64)        {}
func (NoopMetrics) IncWatchStreamRestarts(_ string)                      {}
func (NoopMetrics) IncStreamRequestsReceived(_ string)                   {}
func (NoopMetrics) IncStreamRequestsProcessed(_, _ string)               {}
func (NoopMetrics) ObserveStreamRequestDuration(_ string, _ float64)     {}
func (NoopMetrics) IncStreamRestarts(_ string)                           {}
func (NoopMetrics) IncApplicationEventsByType(_ watch.EventType)         {}
func (NoopMetrics) IncApplicationEventsSent(_ watch.EventType, _ string) {}
func (NoopMetrics) ObserveApplicationEventSendDuration(_ float64)        {}
func (NoopMetrics) SetDispatcherQueueDepth(_ string, _ int)              {}
func (NoopMetrics) IncDispatcherEventsDropped(_, _ string)               {}
func (NoopMetrics) SetDispatcherTombstones(_ string, _ int)              {}
func (NoopMetrics) IncDispatcherEventsBuffered(_ string)                 {}
func (NoopMetrics) IncActiveResourceWatchers()                           {}
func (NoopMetrics) DecActiveResourceWatchers()                           {}
func (NoopMetrics) IncResourceTreeUpdates(_ string)                      {}
func (NoopMetrics) SetLeader(_ bool)                                     {}
