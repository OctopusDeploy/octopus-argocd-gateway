package metrics

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry *prometheus.Registry

	UpdatesTotal       *prometheus.CounterVec
	UpdateLatency      prometheus.Histogram
	SyncsReceivedTotal *prometheus.CounterVec
	SyncReplyLatency   prometheus.Histogram
	ConnectedGateways  prometheus.Gauge
	TargetRate         prometheus.Gauge
	AchievedRate       prometheus.Gauge
	DeadGateways       prometheus.Gauge
}

func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		Registry: reg,
		UpdatesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "simulator",
				Name:      "updates_total",
				Help:      "ApplicationEventUpdate calls by outcome.",
			},
			[]string{"result", "grpc_code"},
		),
		UpdateLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "simulator",
			Name:      "update_latency_seconds",
			Help:      "End-to-end client-side latency of ApplicationEventUpdate.",
			Buckets:   updateLatencyBuckets(),
		}),
		SyncsReceivedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "simulator",
				Name:      "syncs_received_total",
				Help:      "Sync requests received from the server, per virtual gateway.",
			},
			[]string{"gateway"},
		),
		SyncReplyLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "simulator",
			Name:      "sync_reply_latency_seconds",
			Help:      "Receive-to-reply microlatency for sync requests inside the simulator.",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 14),
		}),
		ConnectedGateways: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "simulator",
			Name:      "connected_gateways",
			Help:      "Virtual gateways currently connected to Octopus Server.",
		}),
		TargetRate: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "simulator",
			Name:      "target_rate_updates_per_second",
			Help:      "Current target update rate.",
		}),
		AchievedRate: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "simulator",
			Name:      "achieved_rate_updates_per_second",
			Help:      "Rolling 1-second achieved update rate.",
		}),
		DeadGateways: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "simulator",
			Name:      "dead_gateways",
			Help:      "Virtual gateways marked dead after unrecoverable stream errors.",
		}),
	}

	reg.MustRegister(
		m.UpdatesTotal,
		m.UpdateLatency,
		m.SyncsReceivedTotal,
		m.SyncReplyLatency,
		m.ConnectedGateways,
		m.TargetRate,
		m.AchievedRate,
		m.DeadGateways,
	)
	return m
}

func (m *Metrics) Serve(addr string) (net.Listener, func(), error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := srv.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = err
		}
	}()
	return lis, func() { _ = srv.Close() }, nil
}

// updateLatencyBuckets returns a 1-2-5 series of bucket boundaries (in seconds)
// from 1ms to 10s, which is a reasonable range for end-to-end client-side latency of ApplicationEventUpdate.
func updateLatencyBuckets() []float64 {
	return []float64{
		0.001, 0.002, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10,
	}
}
