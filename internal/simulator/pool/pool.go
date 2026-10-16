package pool

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/multierr"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/credentials"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/gateway"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/metrics"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/report"
)

type Config struct {
	Entries         []credentials.Entry
	TLS             gateway.TLSOptions
	DialTimeout     time.Duration
	DialConcurrency int
}

type Pool struct {
	cfg     Config
	logger  *slog.Logger
	metrics *metrics.Metrics
	rec     *report.Recorder

	gateways []*gateway.Gateway
	rr       atomic.Uint64
	runWg    sync.WaitGroup
}

func New(cfg Config, logger *slog.Logger, m *metrics.Metrics, rec *report.Recorder) *Pool {
	if cfg.DialConcurrency <= 0 {
		cfg.DialConcurrency = 32
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 30 * time.Second
	}
	return &Pool{cfg: cfg, logger: logger, metrics: m, rec: rec}
}

func (p *Pool) Size() int { return len(p.cfg.Entries) }

func (p *Pool) Dial(ctx context.Context) error {
	gws := make([]*gateway.Gateway, len(p.cfg.Entries))
	errs := make([]error, len(p.cfg.Entries))

	sem := make(chan struct{}, p.cfg.DialConcurrency)
	var wg sync.WaitGroup

	for i, entry := range p.cfg.Entries {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int, e credentials.Entry) {
			defer wg.Done()
			defer func() { <-sem }()

			g := gateway.New(i, e, p.cfg.TLS, p.logger, p.metrics, p.rec)
			dialCtx, cancel := context.WithTimeout(ctx, p.cfg.DialTimeout)
			defer cancel()
			if err := g.Dial(dialCtx); err != nil {
				errs[i] = err
				return
			}
			gws[i] = g
		}(i, entry)
	}
	wg.Wait()

	var combined error
	alive := 0
	for i := range gws {
		if errs[i] != nil {
			combined = multierr.Append(combined, errs[i])
		} else {
			alive++
		}
	}
	if combined != nil {
		for _, g := range gws {
			if g != nil {
				_ = g.Close()
			}
		}
		return fmt.Errorf("failed to dial %d/%d gateways: %w", len(p.cfg.Entries)-alive, len(p.cfg.Entries), combined)
	}

	p.gateways = gws
	p.logger.Info("pool dialled", slog.Int("gateways", alive))
	return nil
}

func (p *Pool) Run(ctx context.Context) {
	for _, g := range p.gateways {
		p.runWg.Add(1)
		go func() {
			defer p.runWg.Done()
			if err := g.Start(ctx); err != nil {
				p.logger.Warn("gateway stopped with error",
					slog.String("gateway", g.Name), slog.Any("error", err))
				g.MarkDead()
			}
		}()
	}
}

func (p *Pool) Wait() { p.runWg.Wait() }

func (p *Pool) Close() {
	for _, g := range p.gateways {
		_ = g.Close()
	}
}

func (p *Pool) Gateways() []*gateway.Gateway { return p.gateways }

func (p *Pool) Next() *gateway.Gateway {
	n := len(p.gateways)
	if n == 0 {
		return nil
	}
	for range n {
		idx := int(p.rr.Add(1)-1) % n
		g := p.gateways[idx]
		if !g.IsDead() {
			return g
		}
	}
	return nil
}

func (p *Pool) DeadCount() int {
	dead := 0
	for _, g := range p.gateways {
		if g.IsDead() {
			dead++
		}
	}
	return dead
}
