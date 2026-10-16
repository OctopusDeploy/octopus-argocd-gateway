package pool

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/credentials"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/simulator/gateway"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestPool(names ...string) *Pool {
	p := &Pool{}
	for i, n := range names {
		g := gateway.New(i, credentials.Entry{Name: n}, gateway.TLSOptions{}, testLogger(), nil, nil)
		p.gateways = append(p.gateways, g)
	}
	return p
}

func TestPool_NextSkipsDead(t *testing.T) {
	p := newTestPool("a", "b", "c")
	p.gateways[1].MarkDead()

	seen := map[string]int{}
	for range 6 {
		g := p.Next()
		if g != nil {
			seen[g.Name]++
		}
	}
	assert.Zero(t, seen["b"], "dead gateway must not be selected")
	assert.Equal(t, 6, seen["a"]+seen["c"])
}

func TestPool_NextReturnsNilWhenAllDead(t *testing.T) {
	p := newTestPool("a", "b")
	for _, g := range p.gateways {
		g.MarkDead()
	}
	assert.Nil(t, p.Next())
}

func TestPool_DeadCount(t *testing.T) {
	p := newTestPool("a", "b", "c")
	assert.Zero(t, p.DeadCount())
	p.gateways[0].MarkDead()
	p.gateways[2].MarkDead()
	assert.Equal(t, 2, p.DeadCount())
}
