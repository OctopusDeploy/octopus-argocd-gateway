package runner

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OctopusDeploy/octopus-grpc/go/pkg/grpcerror"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// testDefaultPolicy stands in for octopus.DefaultErrorPolicy, which cannot be
// imported here without a cycle.
type testDefaultPolicy struct{}

func (testDefaultPolicy) Handle(err error) ErrorAction {
	switch grpcerror.Classify(err) {
	case grpcerror.Retry:
		return ActionRetry
	case grpcerror.Stop:
		return ActionIgnore
	default:
		return ActionFatal
	}
}

func (testDefaultPolicy) Wrap(err error) error {
	return err
}

// --- fakes

type alwaysUnavailableBiStream struct{}

func (alwaysUnavailableBiStream) Recv() (struct{}, error) {
	return struct{}{}, status.Error(codes.Unavailable, "unavailable")
}

func (alwaysUnavailableBiStream) SendMsg(any) error {
	return nil
}

type staticBiStreamSource struct {
	policy ErrorPolicy
}

func (s *staticBiStreamSource) CreateStream(context.Context) (BidirectionalStream[struct{}, struct{}], error) {
	return alwaysUnavailableBiStream{}, nil
}

func (s *staticBiStreamSource) ErrorPolicy() ErrorPolicy {
	return s.policy
}

type noopRequestHandler struct{}

func (noopRequestHandler) ProcessRequest(struct{}) (struct{}, error) {
	return struct{}{}, nil
}

type alwaysUnavailableRecvStream struct{}

func (alwaysUnavailableRecvStream) Recv() (int, error) {
	return 0, status.Error(codes.Unavailable, "unavailable")
}

type staticWatchStreamSource struct {
	policy ErrorPolicy
}

func (s *staticWatchStreamSource) CreateStream(context.Context) (RecvStream[int], error) {
	return alwaysUnavailableRecvStream{}, nil
}

func (s *staticWatchStreamSource) ErrorPolicy() ErrorPolicy {
	return s.policy
}

type noopMessageHandler struct{}

func (noopMessageHandler) ProcessMessage(int) error {
	return nil
}

// countingRecoverableHook increments when StreamRunner/WatchRunner log the recoverable recv path.
type countingRecoverableHook struct {
	inner       slog.Handler
	recoverable *atomic.Int32
}

func (h *countingRecoverableHook) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}

func (h *countingRecoverableHook) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "Recoverable stream error" {
		h.recoverable.Add(1)
	}
	return h.inner.Handle(ctx, r)
}

func (h *countingRecoverableHook) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &countingRecoverableHook{inner: h.inner.WithAttrs(attrs), recoverable: h.recoverable}
}

func (h *countingRecoverableHook) WithGroup(name string) slog.Handler {
	return &countingRecoverableHook{inner: h.inner.WithGroup(name), recoverable: h.recoverable}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testRecoverableRecvThenCancelStops(t *testing.T, start func(ctx context.Context, logger *slog.Logger)) {
	t.Helper()

	var recoverableCount atomic.Int32
	hook := &countingRecoverableHook{
		inner:       slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}),
		recoverable: &recoverableCount,
	}
	logger := slog.New(hook)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		start(ctx, logger)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel (stuck?)")
	}

	if recoverableCount.Load() < 1 {
		t.Fatal("expected at least one recoverable recv error path (check policy / fake stream)")
	}
}

// TestStreamRunner_RecoverableErrors_CancelExits exercises retry/backoff on Unavailable recv; cancelling the
// context must stop Start (runner does not block forever).
func TestStreamRunner_RecoverableErrors_CancelExits(t *testing.T) {
	testRecoverableRecvThenCancelStops(t, func(ctx context.Context, logger *slog.Logger) {
		src := &staticBiStreamSource{policy: testDefaultPolicy{}}
		r := NewStreamRunner(src, noopRequestHandler{}, logger, ctx, WithRetryBackoff(50*time.Millisecond))
		r.Start()
	})
}

// TestWatchRunner_RecoverableErrors_CancelExits is the watch-stream analogue of TestStreamRunner_RecoverableErrors_CancelExits.
func TestWatchRunner_RecoverableErrors_CancelExits(t *testing.T) {
	testRecoverableRecvThenCancelStops(t, func(ctx context.Context, logger *slog.Logger) {
		src := &staticWatchStreamSource{policy: testDefaultPolicy{}}
		w := NewWatchRunner(src, noopMessageHandler{}, logger, ctx, WithRetryBackoff(50*time.Millisecond))
		w.Start()
	})
}

type countingProcessErrHook struct {
	inner slog.Handler
	count *atomic.Int32
}

func (h *countingProcessErrHook) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}

func (h *countingProcessErrHook) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "Error processing message" {
		h.count.Add(1)
	}
	return h.inner.Handle(ctx, r)
}

func (h *countingProcessErrHook) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &countingProcessErrHook{inner: h.inner.WithAttrs(attrs), count: h.count}
}

func (h *countingProcessErrHook) WithGroup(name string) slog.Handler {
	return &countingProcessErrHook{inner: h.inner.WithGroup(name), count: h.count}
}

type failingRequestHandler struct{}

func (failingRequestHandler) ProcessRequest(struct{}) (struct{}, error) {
	return struct{}{}, errors.New("process failed")
}

type failingMessageHandler struct{}

func (failingMessageHandler) ProcessMessage(int) error {
	return errors.New("process failed")
}

type oneReqThenUnavailableBiStream struct{ n int }

func (s *oneReqThenUnavailableBiStream) Recv() (struct{}, error) {
	s.n++
	if s.n == 1 {
		return struct{}{}, nil
	}
	return struct{}{}, status.Error(codes.Unavailable, "unavailable")
}

func (oneReqThenUnavailableBiStream) SendMsg(any) error {
	return nil
}

type staticBiStreamSourceWithStream struct {
	policy ErrorPolicy
	stream BidirectionalStream[struct{}, struct{}]
}

func (s *staticBiStreamSourceWithStream) CreateStream(context.Context) (
	BidirectionalStream[struct{}, struct{}],
	error,
) {
	return s.stream, nil
}

func (s *staticBiStreamSourceWithStream) ErrorPolicy() ErrorPolicy {
	return s.policy
}

type oneMsgThenUnavailableRecv struct{ n int }

func (s *oneMsgThenUnavailableRecv) Recv() (int, error) {
	s.n++
	if s.n == 1 {
		return 1, nil
	}
	return 0, status.Error(codes.Unavailable, "unavailable")
}

type staticWatchStreamSourceWithStream struct {
	policy ErrorPolicy
	stream RecvStream[int]
}

func (s *staticWatchStreamSourceWithStream) CreateStream(context.Context) (RecvStream[int], error) {
	return s.stream, nil
}

func (s *staticWatchStreamSourceWithStream) ErrorPolicy() ErrorPolicy {
	return s.policy
}

func TestStreamRunner_ProcessRequestError_Logs(t *testing.T) {
	var n atomic.Int32
	hook := &countingProcessErrHook{
		inner: slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}),
		count: &n,
	}
	logger := slog.New(hook)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := &staticBiStreamSourceWithStream{
		policy: testDefaultPolicy{},
		stream: &oneReqThenUnavailableBiStream{},
	}
	r := NewStreamRunner(src, failingRequestHandler{}, logger, ctx, WithRetryBackoff(50*time.Millisecond))

	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Start()
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return")
	}

	if n.Load() < 1 {
		t.Fatal("expected Error processing message log from ProcessRequest failure")
	}
}

func TestWatchRunner_ProcessMessageError_Logs(t *testing.T) {
	var n atomic.Int32
	hook := &countingProcessErrHook{
		inner: slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}),
		count: &n,
	}
	logger := slog.New(hook)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := &staticWatchStreamSourceWithStream{
		policy: testDefaultPolicy{},
		stream: &oneMsgThenUnavailableRecv{},
	}
	w := NewWatchRunner(src, failingMessageHandler{}, logger, ctx, WithRetryBackoff(50*time.Millisecond))

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Start()
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return")
	}

	if n.Load() < 1 {
		t.Fatal("expected Error processing message log from ProcessMessage failure")
	}
}

type fatalArgBiStream struct{}

func (fatalArgBiStream) Recv() (struct{}, error) {
	return struct{}{}, status.Error(codes.InvalidArgument, "fatal")
}

func (fatalArgBiStream) SendMsg(any) error {
	return nil
}

type fatalBiStreamSource struct {
	policy ErrorPolicy
}

func (s *fatalBiStreamSource) CreateStream(context.Context) (BidirectionalStream[struct{}, struct{}], error) {
	return fatalArgBiStream{}, nil
}

func (s *fatalBiStreamSource) ErrorPolicy() ErrorPolicy {
	return s.policy
}

func TestStreamRunner_UnrecoverableRecvError_ReportsOnErrorsChannel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	src := &fatalBiStreamSource{policy: testDefaultPolicy{}}
	r := NewStreamRunner(src, noopRequestHandler{}, discardLogger(), ctx)

	done := make(chan struct{})
	go func() {
		r.Start()
		close(done)
	}()

	select {
	case err := <-r.Errors():
		if err == nil {
			t.Fatal("expected non-nil error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for fatal error on Errors()")
	}

	cancel()
	<-done
}

type fatalArgWatchStream struct{}

func (fatalArgWatchStream) Recv() (int, error) {
	return 0, status.Error(codes.InvalidArgument, "fatal")
}

type fatalWatchStreamSource struct {
	policy ErrorPolicy
}

func (s *fatalWatchStreamSource) CreateStream(context.Context) (RecvStream[int], error) {
	return fatalArgWatchStream{}, nil
}

func (s *fatalWatchStreamSource) ErrorPolicy() ErrorPolicy {
	return s.policy
}

// flakeBiStreamSource fails CreateStream with Unavailable a few times then returns alwaysUnavailableBiStream
// (covers CreateStream error path in StreamRunner.Start).
type flakeBiStreamSource struct {
	policy ErrorPolicy
	mu     sync.Mutex
	calls  int
	failN  int
}

func (s *flakeBiStreamSource) CreateStream(context.Context) (BidirectionalStream[struct{}, struct{}], error) {
	s.mu.Lock()
	s.calls++
	c := s.calls
	s.mu.Unlock()
	if c <= s.failN {
		return nil, status.Error(codes.Unavailable, "not ready")
	}
	return alwaysUnavailableBiStream{}, nil
}

func (s *flakeBiStreamSource) ErrorPolicy() ErrorPolicy {
	return s.policy
}

func TestStreamRunner_CreateStreamRecoverable_BackoffsAndContinues(t *testing.T) {
	var recoverableN atomic.Int32
	hook := &countingRecoverableHook{
		inner:       slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}),
		recoverable: &recoverableN,
	}
	logger := slog.New(hook)

	ctx, cancel := context.WithCancel(context.Background())
	src := &flakeBiStreamSource{policy: testDefaultPolicy{}, failN: 2}
	r := NewStreamRunner(src, noopRequestHandler{}, logger, ctx, WithRetryBackoff(20*time.Millisecond))

	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Start()
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}

	if recoverableN.Load() < 1 {
		t.Fatal("expected recoverable error logs from CreateStream and/or Recv")
	}
}

type flakeWatchStreamSource struct {
	policy ErrorPolicy
	mu     sync.Mutex
	calls  int
	failN  int
}

func (s *flakeWatchStreamSource) CreateStream(context.Context) (RecvStream[int], error) {
	s.mu.Lock()
	s.calls++
	c := s.calls
	s.mu.Unlock()
	if c <= s.failN {
		return nil, status.Error(codes.Unavailable, "not ready")
	}
	return alwaysUnavailableRecvStream{}, nil
}

func (s *flakeWatchStreamSource) ErrorPolicy() ErrorPolicy {
	return s.policy
}

func TestWatchRunner_CreateStreamRecoverable_BackoffsAndContinues(t *testing.T) {
	var recoverableN atomic.Int32
	hook := &countingRecoverableHook{
		inner:       slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}),
		recoverable: &recoverableN,
	}
	logger := slog.New(hook)

	ctx, cancel := context.WithCancel(context.Background())
	src := &flakeWatchStreamSource{policy: testDefaultPolicy{}, failN: 2}
	w := NewWatchRunner(src, noopMessageHandler{}, logger, ctx, WithRetryBackoff(20*time.Millisecond))

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Start()
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}

	if recoverableN.Load() < 1 {
		t.Fatal("expected recoverable error logs from CreateStream and/or Recv")
	}
}

// recvOKSendUnavailableStream succeeds on Recv then fails SendMsg with Unavailable (handleSendErrorAction path).
type recvOKSendUnavailableStream struct{}

func (recvOKSendUnavailableStream) Recv() (struct{}, error) {
	return struct{}{}, nil
}

func (recvOKSendUnavailableStream) SendMsg(any) error {
	return status.Error(codes.Unavailable, "send")
}

type countingSendRecoverableHook struct {
	inner       slog.Handler
	sendRecover *atomic.Int32
}

func (h *countingSendRecoverableHook) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}

func (h *countingSendRecoverableHook) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "Recoverable stream send error" {
		h.sendRecover.Add(1)
	}
	return h.inner.Handle(ctx, r)
}

func (h *countingSendRecoverableHook) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &countingSendRecoverableHook{inner: h.inner.WithAttrs(attrs), sendRecover: h.sendRecover}
}

func (h *countingSendRecoverableHook) WithGroup(name string) slog.Handler {
	return &countingSendRecoverableHook{inner: h.inner.WithGroup(name), sendRecover: h.sendRecover}
}

func TestStreamRunner_RecoverableSendError_LogsSendPath(t *testing.T) {
	var n atomic.Int32
	hook := &countingSendRecoverableHook{
		inner:       slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}),
		sendRecover: &n,
	}
	logger := slog.New(hook)

	ctx, cancel := context.WithCancel(context.Background())
	src := &staticBiStreamSourceWithStream{
		policy: testDefaultPolicy{},
		stream: recvOKSendUnavailableStream{},
	}
	r := NewStreamRunner(src, noopRequestHandler{}, logger, ctx, WithRetryBackoff(30*time.Millisecond))

	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Start()
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return")
	}

	if n.Load() < 1 {
		t.Fatal("expected Recoverable stream send error log")
	}
}

func TestWatchRunner_UnrecoverableRecvError_ReportsOnErrorsChannel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	src := &fatalWatchStreamSource{policy: testDefaultPolicy{}}
	w := NewWatchRunner(src, noopMessageHandler{}, discardLogger(), ctx)

	done := make(chan struct{})
	go func() {
		w.Start()
		close(done)
	}()

	select {
	case err := <-w.Errors():
		if err == nil {
			t.Fatal("expected non-nil error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for fatal error on Errors()")
	}

	cancel()
	<-done
}
