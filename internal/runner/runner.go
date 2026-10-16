package runner

import (
	"context"
	"time"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/metrics"
)

const DefaultRetryBackoff = 5 * time.Second

type streamSessionResult int

const (
	sessionRedial streamSessionResult = iota // backoff then CreateStream again
	sessionStop                              // leave Start (fatal, cancel, Unimplemented, policy stop)
)

type ErrorAction int

const (
	ActionRetry  ErrorAction = iota // Reconnect with backoff
	ActionFatal                     // Send to error channel, stop runner
	ActionIgnore                    // Log and continue
)

type ErrorPolicy interface {
	Handle(err error) ErrorAction
	Wrap(err error) error
}

type RecvStream[T any] interface {
	Recv() (T, error)
}

type StreamSource[T any] interface {
	CreateStream(ctx context.Context) (RecvStream[T], error)
	ErrorPolicy() ErrorPolicy
}

type MessageHandler[T any] interface {
	ProcessMessage(msg T) error
}

type BidirectionalStream[Req, Resp any] interface {
	Recv() (Req, error)
	SendMsg(m any) error
}

type BidirectionalStreamSource[Req, Resp any] interface {
	CreateStream(ctx context.Context) (BidirectionalStream[Req, Resp], error)
	ErrorPolicy() ErrorPolicy
}

type RequestHandler[Req, Resp any] interface {
	ProcessRequest(req Req) (Resp, error)
}

type Runner interface {
	Start()
	Errors() <-chan error
	Context() context.Context
}

type RunnerOption func(*runnerConfig)

type runnerConfig struct {
	silentOnUnimplemented bool
	retryBackoff          time.Duration
	metrics               metrics.Metrics
	handlerName           string
}

func WithSilentUnimplemented() RunnerOption {
	return func(c *runnerConfig) {
		c.silentOnUnimplemented = true
	}
}

func WithRetryBackoff(d time.Duration) RunnerOption {
	return func(c *runnerConfig) {
		c.retryBackoff = d
	}
}

func WithMetrics(m metrics.Metrics, handlerName string) RunnerOption {
	return func(c *runnerConfig) {
		c.metrics = m
		c.handlerName = handlerName
	}
}
