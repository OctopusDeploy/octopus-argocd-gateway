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

// StreamRunner handles bidirectional gRPC streams where the server sends requests and expects responses.
// It manages connection lifecycle, automatic retries with configurable backoff, and error handling via
// configurable policies. Used for Octopus server streams that require request-response interactions
// (e.g., live status updates, manifest fetching).
type StreamRunner[Req, Resp any] struct {
	streamSource          BidirectionalStreamSource[Req, Resp]
	handler               RequestHandler[Req, Resp]
	logger                *slog.Logger
	ctx                   context.Context
	errorCh               chan error
	retryBackoff          time.Duration
	silentOnUnimplemented bool
	metrics               metrics.Metrics
	handlerName           string
}

func NewStreamRunner[Req, Resp any](
	streamSource BidirectionalStreamSource[Req, Resp],
	handler RequestHandler[Req, Resp],
	logger *slog.Logger,
	ctx context.Context,
	opts ...RunnerOption,
) *StreamRunner[Req, Resp] {
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

	return &StreamRunner[Req, Resp]{
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

func (r *StreamRunner[Req, Resp]) Errors() <-chan error {
	return r.errorCh
}

func (r *StreamRunner[Req, Resp]) Start() {
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

		if r.runStreamSession(stream) == sessionStop {
			return
		}
	}
}

func (r *StreamRunner[Req, Resp]) runStreamSession(stream BidirectionalStream[Req, Resp]) streamSessionResult {
	for {
		req, err := stream.Recv()
		if err != nil {
			return r.handleError(err, r.handleErrorAction)
		}

		r.metrics.IncStreamRequestsReceived(r.handlerName)

		start := time.Now()

		resp, procErr := r.handler.ProcessRequest(req)
		if procErr != nil {
			r.logger.Error("Error processing message", slog.Any("error", procErr))
		}

		r.metrics.ObserveStreamRequestDuration(r.handlerName, time.Since(start).Seconds())
		r.metrics.IncStreamRequestsProcessed(r.handlerName, "success")

		if sendErr := stream.SendMsg(resp); sendErr != nil {
			return r.handleError(sendErr, r.handleSendErrorAction)
		}
	}
}

// handleError returns sessionStop to leave Start, or sessionRedial after backoff to open a new stream.
func (r *StreamRunner[Req, Resp]) handleError(err error, handleErrAction func(error) ErrorAction) streamSessionResult {
	if r.handleRecvError(err) {
		return sessionStop
	}
	if handleErrAction(err) != ActionRetry {
		return sessionStop
	}
	r.waitRetryBackoff()
	return sessionRedial
}

func (r *StreamRunner[Req, Resp]) Context() context.Context {
	return r.ctx
}

func (r *StreamRunner[Req, Resp]) handleRecvError(err error) bool {
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

func (r *StreamRunner[Req, Resp]) handleErrorAction(err error) ErrorAction {
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

func (r *StreamRunner[Req, Resp]) handleSendErrorAction(err error) ErrorAction {
	policy := r.streamSource.ErrorPolicy()
	action := policy.Handle(err)

	if action != ActionRetry {
		return action
	}

	_, file, line, ok := runtime.Caller(2)
	if ok {
		callerErr := fmt.Errorf("%s#%d failed to send stream message", file, line)
		err = multierr.Append(err, callerErr)
	}
	r.logger.Error("Recoverable stream send error", slog.Any("error", err))
	return ActionRetry
}

// waitRetryBackoff blocks for retryBackoff before the outer Start loop opens another stream (instead of calling start again to prevent call stack grows)
func (r *StreamRunner[Req, Resp]) waitRetryBackoff() {
	r.metrics.IncStreamRestarts(r.handlerName)
	r.logger.Info("Attempting to restart subscriber", slog.Duration("backoff", r.retryBackoff))
	select {
	case <-r.ctx.Done():
		return
	case <-time.After(r.retryBackoff):
	}
}
