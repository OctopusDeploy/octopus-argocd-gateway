package runner

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"go.uber.org/multierr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/metrics"
)

// WatchRunner handles server-streaming gRPC connections where the server pushes events to the client.
// It manages connection lifecycle, automatic retries with configurable backoff, and error handling via
// configurable policies. Used for ArgoCD watch streams that emit events without requiring responses
// (e.g., application events, resource tree updates).
type WatchRunner[T any] struct {
	streamSource          StreamSource[T]
	handler               MessageHandler[T]
	logger                *slog.Logger
	ctx                   context.Context
	errorCh               chan error
	retryBackoff          time.Duration
	silentOnUnimplemented bool
	metrics               metrics.Metrics
	handlerName           string
}

func NewWatchRunner[T any](
	streamSource StreamSource[T],
	handler MessageHandler[T],
	logger *slog.Logger,
	ctx context.Context,
	opts ...RunnerOption,
) *WatchRunner[T] {
	cfg := &runnerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	retryBackoff := cfg.retryBackoff
	if retryBackoff == 0 {
		retryBackoff = DefaultRetryBackoff
	}

	m := cfg.metrics
	if m == nil {
		m = metrics.NoopMetrics{}
	}

	return &WatchRunner[T]{
		streamSource:          streamSource,
		handler:               handler,
		logger:                logger,
		ctx:                   ctx,
		errorCh:               make(chan error),
		retryBackoff:          retryBackoff,
		silentOnUnimplemented: cfg.silentOnUnimplemented,
		metrics:               m,
		handlerName:           cfg.handlerName,
	}
}

func (r *WatchRunner[T]) Errors() <-chan error {
	return r.errorCh
}

func (r *WatchRunner[T]) Start() {
	for {
		if r.ctx.Err() != nil {
			return
		}

		stream, err := r.streamSource.CreateStream(r.ctx)
		if err != nil {
			if r.handleError(err, r.handleErrorAction) == sessionStop {
				return
			}
			continue
		}

		if r.runWatchSession(stream) == sessionStop {
			return
		}
	}
}

func (r *WatchRunner[T]) runWatchSession(stream RecvStream[T]) streamSessionResult {
	for {
		msg, err := stream.Recv()
		if err != nil {
			return r.handleError(err, r.handleErrorAction)
		}

		r.metrics.IncWatchEventsReceived(r.handlerName)

		start := time.Now()
		processErr := r.handler.ProcessMessage(msg)

		r.metrics.ObserveWatchEventDuration(r.handlerName, time.Since(start).Seconds())
		result := "success"
		if processErr != nil {
			result = "error"
		}
		r.metrics.IncWatchEventsProcessed(r.handlerName, result)

		if processErr != nil {
			r.logger.Error("Error processing message", slog.Any("error", processErr))
		}
	}
}

// handleError returns sessionStop to leave Start, or sessionRedial after backoff to open a new stream.
func (r *WatchRunner[T]) handleError(err error, handleErrAction func(error) ErrorAction) streamSessionResult {
	if r.handleRecvError(err) {
		return sessionStop
	}
	if handleErrAction(err) != ActionRetry {
		return sessionStop
	}
	r.waitRetryBackoff()
	return sessionRedial
}

func (r *WatchRunner[T]) handleRecvError(err error) bool {
	if r.ctx.Err() != nil {
		r.logger.Debug("Stopping subscriber due to context cancellation", slog.Any("error", err))
		return true
	}

	grpcErr := status.Convert(err)
	if grpcErr.Code() == codes.Unimplemented {
		if !r.silentOnUnimplemented {
			r.logger.Info("Service not implemented on server, stopping subscriber")
		}
		return true
	}
	return false
}

func (r *WatchRunner[T]) Context() context.Context {
	return r.ctx
}

func (r *WatchRunner[T]) handleErrorAction(err error) ErrorAction {
	policy := r.streamSource.ErrorPolicy()
	action := policy.Handle(err)

	switch action {
	case ActionRetry:
		r.logger.Error("Recoverable stream error", slog.Any("error", err))
	case ActionFatal:
		wrappedErr := policy.Wrap(err)
		r.logger.Error("Unrecoverable stream error", slog.Any("error", wrappedErr))
		_, file, line, ok := runtime.Caller(2)
		if ok {
			callerErr := fmt.Errorf("%s#%d failed to receive stream message", file, line)
			r.errorCh <- multierr.Append(wrappedErr, callerErr)
			return ActionFatal
		}
		r.errorCh <- wrappedErr
		return ActionFatal
	case ActionIgnore:
		r.logger.Debug("Ignoring error", slog.Any("error", err))
	default:
		return action
	}
	return action
}

// waitRetryBackoff blocks for retryBackoff before the outer Start loop opens another stream (it does not call Start).
func (r *WatchRunner[T]) waitRetryBackoff() {
	r.metrics.IncWatchStreamRestarts(r.handlerName)
	r.logger.Info("Attempting to restart subscriber", slog.Duration("backoff", r.retryBackoff))
	select {
	case <-r.ctx.Done():
		return
	case <-time.After(r.retryBackoff):
	}
}
