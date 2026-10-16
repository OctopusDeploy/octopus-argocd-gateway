package runner

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRunner is a hand-rolled Runner test double — it implements the Runner interface and lets
// tests drive Start lifecycle, error emission, and ctx interaction without spinning up a real
// gRPC stream. The behaviour is intentionally simple: Start blocks until either onStart returns
// (sub-runner finished naturally) or ctx is cancelled.
type stubRunner struct {
	ctx      context.Context
	errCh    chan error
	onStart  func(s *stubRunner)
	started  atomic.Bool
	finished atomic.Bool
}

func newStubRunner(ctx context.Context, onStart func(s *stubRunner)) *stubRunner {
	return &stubRunner{
		ctx:     ctx,
		errCh:   make(chan error, 4),
		onStart: onStart,
	}
}

func (s *stubRunner) Start() {
	s.started.Store(true)
	defer s.finished.Store(true)
	if s.onStart != nil {
		s.onStart(s)
		return
	}
	<-s.ctx.Done()
}

func (s *stubRunner) Errors() <-chan error     { return s.errCh }
func (s *stubRunner) Context() context.Context { return s.ctx }

func TestMultiRunner_EmptyList_BlocksUntilCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := NewMultiRunner(ctx, nil)

	done := make(chan struct{})
	go func() {
		m.Start()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Start returned before ctx was cancelled")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}

	// Errors channel should be closed after Start returns.
	_, ok := <-m.Errors()
	assert.False(t, ok, "Errors channel must be closed after Start returns")
}

func TestMultiRunner_SingleRunner_ErrorsPassThrough(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wantErr := errors.New("boom")
	sub := newStubRunner(ctx, func(s *stubRunner) {
		s.errCh <- wantErr
		<-s.ctx.Done()
	})

	m := NewMultiRunner(ctx, []Runner{sub})

	doneStart := make(chan struct{})
	go func() {
		m.Start()
		close(doneStart)
	}()

	select {
	case err := <-m.Errors():
		assert.Equal(t, wantErr, err)
	case <-time.After(time.Second):
		t.Fatal("did not receive sub-runner error via MultiRunner.Errors()")
	}

	cancel()

	select {
	case <-doneStart:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}

	assert.True(t, sub.started.Load())
	assert.True(t, sub.finished.Load())
}

func TestMultiRunner_MultipleRunners_StartInParallel_FanInErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const n = 4
	errs := make([]error, n)
	subs := make([]Runner, 0, n)
	var started atomic.Int32

	for i := 0; i < n; i++ {
		errs[i] = errors.New("err-" + string(rune('A'+i)))
		i := i
		sub := newStubRunner(ctx, func(s *stubRunner) {
			started.Add(1)
			s.errCh <- errs[i]
			<-s.ctx.Done()
		})
		subs = append(subs, sub)
	}

	m := NewMultiRunner(ctx, subs)

	doneStart := make(chan struct{})
	go func() {
		m.Start()
		close(doneStart)
	}()

	// Wait until all sub-runners reported started.
	require.Eventually(t, func() bool { return started.Load() == n }, time.Second, 5*time.Millisecond,
		"all sub-runners should start in parallel")

	got := map[error]bool{}
	for i := 0; i < n; i++ {
		select {
		case err := <-m.Errors():
			got[err] = true
		case <-time.After(time.Second):
			t.Fatalf("missing error from sub-runner %d (got=%d/%d)", i, len(got), n)
		}
	}
	for _, want := range errs {
		assert.True(t, got[want], "missing error %v", want)
	}

	cancel()

	select {
	case <-doneStart:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
}

func TestMultiRunner_OneFatal_DoesNotCancelSiblings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// One sub-runner exits immediately (the "fatal" one). The other continues to run, and we
	// verify that its Start was not aborted by the sibling's failure.
	fatal := newStubRunner(ctx, func(s *stubRunner) {
		s.errCh <- errors.New("fatal")
		// Return immediately — simulates the fatal path where the sub-runner stops.
	})

	var siblingProgress atomic.Int32
	sibling := newStubRunner(ctx, func(s *stubRunner) {
		t := time.NewTicker(10 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				siblingProgress.Add(1)
			case <-s.ctx.Done():
				return
			}
		}
	})

	m := NewMultiRunner(ctx, []Runner{fatal, sibling})

	doneStart := make(chan struct{})
	go func() {
		m.Start()
		close(doneStart)
	}()

	// Confirm fatal error arrives.
	select {
	case err := <-m.Errors():
		assert.Equal(t, "fatal", err.Error())
	case <-time.After(time.Second):
		t.Fatal("expected fatal error from sub-runner")
	}

	// Give sibling time to keep ticking; it must not be cancelled by the fatal sub-runner.
	require.Eventually(t, func() bool { return siblingProgress.Load() >= 3 }, time.Second, 5*time.Millisecond,
		"sibling sub-runner should keep running after sibling fatal")

	cancel()

	select {
	case <-doneStart:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
}

func TestMultiRunner_ParentCancelPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var subStarted sync.WaitGroup
	subStarted.Add(2)

	mkSub := func() *stubRunner {
		return newStubRunner(ctx, func(s *stubRunner) {
			subStarted.Done()
			<-s.ctx.Done()
		})
	}

	a, b := mkSub(), mkSub()
	m := NewMultiRunner(ctx, []Runner{a, b})

	doneStart := make(chan struct{})
	go func() {
		m.Start()
		close(doneStart)
	}()

	subStarted.Wait()
	cancel()

	select {
	case <-doneStart:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after parent ctx cancel")
	}

	assert.True(t, a.finished.Load(), "sub-runner A should observe ctx cancel")
	assert.True(t, b.finished.Load(), "sub-runner B should observe ctx cancel")

	// Errors channel should be closed after Start returns.
	_, ok := <-m.Errors()
	assert.False(t, ok, "Errors channel must be closed after Start returns")
}
