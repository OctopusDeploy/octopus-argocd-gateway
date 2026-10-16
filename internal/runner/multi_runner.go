package runner

import (
	"context"
	"sync"
)

// MultiRunner fans out a single Runner-shaped lifecycle over N sub-runners. It is the
// composition primitive used to fan out the fleet-wide ApplicationsWatch into one stream per
// configured AppProject (each authenticated with its project-scoped bearer token).
//
// Behaviour:
//   - Start launches each sub-runner in its own goroutine and a per-sub-runner forwarder goroutine
//     that pipes its Errors() into the shared error channel. Start returns once every sub-runner's
//     Start has returned (so the caller can await full shutdown), then closes Errors() so
//     downstream consumers can range over it cleanly.
//   - A fatal failure on one sub-runner forwards the error but does NOT cancel siblings. We do
//     not want a single tenant's broken stream to take the whole gateway down; sub-runner-level
//     retry handles transient failures.
//   - Parent ctx cancellation propagates because all sub-runners share that ctx. There is no
//     separate cancel plumbed inside MultiRunner.
type MultiRunner struct {
	runners []Runner
	errCh   chan error
	ctx     context.Context
}

// NewMultiRunner builds a MultiRunner over the provided sub-runners. The runners provide their own context
func NewMultiRunner(ctx context.Context, runners []Runner) *MultiRunner {
	return &MultiRunner{
		runners: runners,
		errCh:   make(chan error),
		ctx:     ctx,
	}
}

// Errors returns the fan-in channel for all sub-runner errors. The channel is closed when every
// sub-runner has stopped, so callers can `for err := range mr.Errors()` to drain.
func (m *MultiRunner) Errors() <-chan error {
	return m.errCh
}

func (m *MultiRunner) Context() context.Context {
	return m.ctx
}

// Start blocks until every sub-runner's Start AND its forwarder goroutine has returned, then
// closes the shared error channel. Forwarders are tracked in the WaitGroup so the deferred close
// cannot race a forwarder mid-send and panic on a closed channel.
func (m *MultiRunner) Start() {
	defer close(m.errCh)

	if len(m.runners) == 0 {
		<-m.ctx.Done()
		return
	}

	var wg sync.WaitGroup
	for _, r := range m.runners {
		r := r
		done := make(chan struct{})

		wg.Add(2)
		go func() {
			defer wg.Done()
			defer close(done)
			r.Start()
		}()
		go func() {
			defer wg.Done()
			m.forward(r, done)
		}()
	}

	wg.Wait()
}

func (m *MultiRunner) forward(r Runner, done <-chan struct{}) {
	for {
		select {
		case err, ok := <-r.Errors():
			if !ok {
				return
			}
			if !m.send(err) {
				return
			}
		case <-done:
			// Sub-runner finished. Drain any errors still sitting in its buffered channel before
			// exiting — those were emitted before Start returned and would otherwise be lost.
			// We loop with a non-blocking receive so we exit promptly when the buffer is empty.
			for {
				select {
				case err, ok := <-r.Errors():
					if !ok {
						return
					}
					if !m.send(err) {
						return
					}
				default:
					return
				}
			}
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *MultiRunner) send(err error) bool {
	select {
	case m.errCh <- err:
		return true
	case <-m.ctx.Done():
		return false
	}
}
