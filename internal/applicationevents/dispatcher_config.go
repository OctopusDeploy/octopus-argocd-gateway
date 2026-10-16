package applicationevents

import (
	"time"

	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
)

// DispatcherConfig tunes a DispatchingHandler. Zero values fall back to the Default* constants.
type DispatcherConfig struct {
	Threadiness    int
	MaxRetries     int
	TombstoneTTL   time.Duration
	DrainTimeout   time.Duration
	BufferDuration time.Duration
	QueueConfig    workqueue.TypedRateLimitingQueueConfig[string]
}

// Option sets one field of a DispatcherConfig.
type Option func(*DispatcherConfig)

func NewDispatcherConfig(options ...Option) DispatcherConfig {
	cfg := DispatcherConfig{
		QueueConfig: workqueue.TypedRateLimitingQueueConfig[string]{},
	}
	cfg = cfg.withDefaults()

	for _, option := range options {
		option(&cfg)
	}

	return cfg
}

func WithThreadiness(threadiness int) Option {
	return func(c *DispatcherConfig) { c.Threadiness = threadiness }
}

func WithMaxRetries(maxRetries int) Option {
	return func(c *DispatcherConfig) { c.MaxRetries = maxRetries }
}

func WithTombstoneTTL(tombstoneTTL time.Duration) Option {
	return func(c *DispatcherConfig) { c.TombstoneTTL = tombstoneTTL }
}

func WithDrainTimeout(drainTimeout time.Duration) Option {
	return func(c *DispatcherConfig) { c.DrainTimeout = drainTimeout }
}

func WithBufferDuration(bufferDuration time.Duration) Option {
	return func(c *DispatcherConfig) { c.BufferDuration = bufferDuration }
}

func (c DispatcherConfig) withDefaults() DispatcherConfig {
	if c.Threadiness <= 0 {
		c.Threadiness = DefaultThreadiness
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = DefaultMaxRetries
	}
	if c.TombstoneTTL <= 0 {
		c.TombstoneTTL = DefaultTombstoneTTL
	}
	if c.DrainTimeout <= 0 {
		c.DrainTimeout = DefaultDrainTimeout
	}

	// Buffer duration is not included in defaults as 0 represents no buffering
	return c
}

func WithClock(clock clock.WithTicker) Option {
	return func(c *DispatcherConfig) { c.QueueConfig.Clock = clock }
}
