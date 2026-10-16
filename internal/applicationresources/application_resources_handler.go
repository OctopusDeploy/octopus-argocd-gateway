package applicationresources

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/OctopusDeploy/octopus-grpc/go/pkg/grpcerror"
	"go.uber.org/multierr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/metrics"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopus"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
)

type resourceSessionResult int

const (
	sessionRedial resourceSessionResult = iota
	sessionStop
)

type Handler struct {
	logger             *slog.Logger
	ctx                context.Context
	client             pb.ApplicationResourceServiceClient
	stream             pb.ApplicationResourceService_WatchApplicationResourcesClient
	resourceWatchers   map[string]*resourceWatcher
	mu                 sync.RWMutex
	applicationService applicationService
	resultsChan        chan *resourceWatcherResult
	errorCh            chan error
	retryCount         int
	metrics            metrics.Metrics
}

func New(
	logger *slog.Logger,
	ctx context.Context,
	octopusClient octopus.Client,
	argoClient argocd.Client,
	m metrics.Metrics,
) (*Handler, error) {
	return &Handler{
		logger:             logger,
		ctx:                ctx,
		client:             octopusClient.ApplicationResources(),
		resourceWatchers:   make(map[string]*resourceWatcher),
		applicationService: argoClient.Applications(),
		errorCh:            make(chan error),
		metrics:            m,
	}, nil
}

func NewRunner(
	logger *slog.Logger,
	ctx context.Context,
	octopusClient octopus.Client,
	argoClient argocd.Client,
	m metrics.Metrics,
	_ ...octopus.RunnerOption,
) (octopus.Runner, error) {
	return New(logger, ctx, octopusClient, argoClient, m)
}

func (h *Handler) Connect() error {
	if h.stream != nil {
		_ = h.stream.CloseSend()
		h.stream = nil
	}

	stream, err := h.client.WatchApplicationResources(h.ctx)
	if err != nil {
		return err
	}

	h.stream = stream
	h.resourceWatchers = make(map[string]*resourceWatcher)
	return nil
}

func (h *Handler) Errors() <-chan error {
	return h.errorCh
}

func (h *Handler) Context() context.Context {
	return h.ctx
}

func (h *Handler) Start() {
	h.resultsChan = make(chan *resourceWatcherResult)
	go h.resultsPublisher(h.resultsChan)

	for {
		if h.ctx.Err() != nil {
			return
		}

		if err := h.Connect(); err != nil {
			h.errorCh <- err
			return
		}

		if h.runResourceStreamSession() == sessionStop {
			return
		}
	}
}

func (h *Handler) runResourceStreamSession() resourceSessionResult {
	for {
		request, err := h.stream.Recv()
		if err != nil {
			return h.handleRecvError(err)
		}
		h.handleRequest(request)
	}
}

func (h *Handler) handleRecvError(err error) resourceSessionResult {
	if h.ctx.Err() != nil {
		h.logger.With(slog.Any("error", err)).Debug("Stopping subscriber due to context cancellation")
		return sessionStop
	}

	switch grpcerror.Classify(err) {
	case grpcerror.Retry:
		h.logger.With(slog.Any("error", err)).Error("Recoverable stream error")
		return h.reconnectAfterBackoff()
	case grpcerror.Stop:
		return sessionStop
	}

	err = octopus.WrapIfUnauthorized(err)
	h.logger.With(slog.Any("error", err)).Error("Unrecoverable stream error")
	_, file, line, ok := runtime.Caller(3)
	if ok {
		callerErr := fmt.Errorf("%s#%d failed to receive stream message", file, line)
		h.errorCh <- multierr.Append(err, callerErr)
		return sessionStop
	}
	h.errorCh <- err
	return sessionStop
}

// reconnectAfterBackoff waits with increasing delay, then returns sessionRedial so the outer Start loop
// opens a fresh connection, or sessionStop if the context was cancelled during the wait.
func (h *Handler) reconnectAfterBackoff() resourceSessionResult {
	h.logger.Info("Attempting to restart subscriber")
	h.retryCount++

	select {
	case <-h.ctx.Done():
		return sessionStop
	case <-time.After(time.Second * time.Duration(h.retryCount)):
	}

	return sessionRedial
}

func (h *Handler) handleRequest(request *pb.WatchApplicationResourcesRequest) {
	for _, query := range request.Query {
		h.startResourceWatcher(query, h.resultsChan)
	}
}

func (h *Handler) startResourceWatcher(
	query *pb.WatchApplicationQuery,
	resultsChan chan *resourceWatcherResult,
) {
	resourceWatcherId := query.ApplicationUid.Value

	h.mu.RLock()
	existingWatcher := h.resourceWatchers[resourceWatcherId]
	h.mu.RUnlock()

	logger := h.logger.With(
		slog.String("application", query.QualifiedName()),
	)
	if existingWatcher != nil && existingWatcher.status != Started {
		logger.With(
			slog.Any("WatcherStatus", existingWatcher.status),
		).Debug("Resource watcher exists but is not active, recreating it")
		h.cleanupResourceWatcher(query.ApplicationUid)
	} else if existingWatcher != nil {
		logger.Debug("Resource watcher already exists, updating last activity timestamp")
		existingWatcher.lastActivity = time.Now()
		return
	}

	watcher, err := newResourceWatcher(
		h.logger,
		h.ctx,
		query,
		h.applicationService,
		resultsChan,
		60*time.Second,
	)
	if err != nil {
		logger.With(slog.Any("error", err)).Error("Failed to create resource watcher")
		resultsChan <- &resourceWatcherResult{
			Query: query,
			Error: err,
		}
		return
	}

	logger.Info("Created new resource watcher, starting to watch resources")
	h.mu.Lock()
	h.resourceWatchers[resourceWatcherId] = watcher
	h.mu.Unlock()

	h.metrics.IncActiveResourceWatchers()

	go watcher.watchResources()
}

func (h *Handler) resultsPublisher(resultsChan chan *resourceWatcherResult) {
	for {
		result := <-resultsChan

		logger := h.logger.With(
			slog.String("watcherId", result.Query.ApplicationUid.Value),
			slog.String("application", result.Query.QualifiedName()),
		)
		if result.Error != nil {
			grpcError := status.Convert(result.Error)

			if grpcError.Code() != codes.Canceled {
				logger.With(slog.Any("error", result.Error)).Error("Resource watcher encountered an error")

				errUpdate := &pb.WatchApplicationResourcesResponse{
					Results: &pb.ApplicationResourceResult{
						ApplicationUid: result.Query.ApplicationUid,
						Error:          &pb.WatchApplicationResourcesError{ErrorMessage: result.Error.Error()},
					},
				}

				err := h.stream.SendMsg(errUpdate)
				if err != nil {
					logger.With(slog.Any("error", err)).Error("Failed to send error to Octopus")
				}
			}

			h.metrics.IncResourceTreeUpdates("error")

			h.cleanupResourceWatcher(result.Query.ApplicationUid)
			continue
		}

		if result.Resources != nil {
			logger.Debug("Publishing resource updates to Octopus")

			app, err := h.applicationService.GetApplication(
				result.Query.ApplicationProject,
				result.Query.ApplicationName,
				result.Query.ApplicationNamespace,
			)
			if err != nil {
				logger.With(slog.Any("error", err)).
					Error("Error getting application from ArgoCD")
				continue
			}

			mappedResources, errors := MapResourceNodesToApplicationResources(result.Resources.Nodes, app)
			if len(errors) > 0 {
				for _, err := range errors {
					logger.With(slog.Any("error", err)).Error("Error converting resource to update")
				}
			}

			update := pb.WatchApplicationResourcesResponse{
				Results: &pb.ApplicationResourceResult{
					PresentResources: mappedResources.PresentResources,
					MissingResources: mappedResources.MissingResources,
					ChildResources:   mappedResources.ChildResources,
					ApplicationUid:   result.Query.ApplicationUid,
				},
			}
			err = h.stream.SendMsg(&update)
			if err != nil {
				logger.With(slog.Any("error", err)).Error("Error publishing results to Octopus")
				h.metrics.IncResourceTreeUpdates("error")
				h.cleanupResourceWatcher(result.Query.ApplicationUid)
				continue
			}

			h.metrics.IncResourceTreeUpdates("success")
		}
	}
}

func (h *Handler) cleanupResourceWatcher(applicationUid *pb.ApplicationUid) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if watcher, exists := h.resourceWatchers[applicationUid.Value]; exists {
		watcher.cleanUp()
		delete(h.resourceWatchers, applicationUid.Value)
		h.metrics.DecActiveResourceWatchers()
		h.logger.With(slog.String("watcherId", applicationUid.Value)).Debug("Cleaned up resource watcher")
	}
}
