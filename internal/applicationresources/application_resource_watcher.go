package applicationresources

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"

	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
)

type applicationService interface {
	GetApplication(project, name, namespace string) (*v1alpha1.Application, error)
	GetApplicationResourceTree(project, name, namespace string) (*v1alpha1.ApplicationTree, error)
	WatchApplicationResourceTree(
		ctx context.Context,
		project, name, namespace string,
	) (application.ApplicationService_WatchResourceTreeClient, error)
}

type resourceWatcherContextKey struct{}

type resourceWatcherResult struct {
	Query     *pb.WatchApplicationQuery
	Resources *v1alpha1.ApplicationTree
	Error     error
}
type ResourceWatcherStatus string

const (
	Started ResourceWatcherStatus = "started"
	Stopped ResourceWatcherStatus = "stopped"
	Initial ResourceWatcherStatus = "initial"
)

type resourceWatcher struct {
	query                         *pb.WatchApplicationQuery
	applicationService            applicationService
	resourceStream                application.ApplicationService_WatchResourceTreeClient
	ttl                           time.Duration
	lastActivity                  time.Time
	resultsChan                   chan *resourceWatcherResult
	context                       context.Context
	cancelFunc                    context.CancelCauseFunc
	logger                        *slog.Logger
	status                        ResourceWatcherStatus
	lastReceivedResourceEventHash string
}

func newResourceWatcher(
	logger *slog.Logger,
	parentContext context.Context,
	query *pb.WatchApplicationQuery,
	appService applicationService,
	resultsChan chan *resourceWatcherResult,
	ttl time.Duration,
) (*resourceWatcher, error) {
	ctx := context.WithValue(
		parentContext,
		resourceWatcherContextKey{},
		fmt.Sprintf("ResourceWatcher-%s", query.ApplicationUid.Value),
	)
	ctxWithCancel, cancelFunc := context.WithCancelCause(ctx)

	client, err := appService.WatchApplicationResourceTree(
		ctxWithCancel,
		query.ApplicationProject,
		query.ApplicationName,
		query.ApplicationNamespace,
	)
	if err != nil {
		logger.With(slog.Any("error", err)).Error("Failed to create resource watcher")
		cancelFunc(err)
		return nil, err
	}

	watcher := &resourceWatcher{
		query:              query,
		applicationService: appService,
		resourceStream:     client,
		ttl:                ttl,
		lastActivity:       time.Now(),
		resultsChan:        resultsChan,
		context:            ctxWithCancel,
		cancelFunc:         cancelFunc,
		status:             Initial,
	}

	resourceWatcherLogger := logger.With(
		slog.String("watcherId", query.ApplicationUid.Value),
		slog.String("application", query.QualifiedName()),
		slog.Time("lastActivity", watcher.lastActivity),
	)
	watcher.logger = resourceWatcherLogger

	return watcher, nil
}

// publishResult sends the result either from an event or from polling the resource tree endpoint to application_resources_handler to send to Octopus Deploy
// Results will only be published if resources produces a different sha256 hash to previous function calls.
// Setting forcePublish to true will cause all resource updates to be published
func (w *resourceWatcher) publishResult(resources *v1alpha1.ApplicationTree, forcePublish bool) {
	if resources != nil {
		resourceHash, hashErr := hashNodes(resources.Nodes)

		if hashErr != nil && !forcePublish {
			w.logger.Error("Failed to hash nodes", slog.Any("error", hashErr))
			return
		}

		// If the hashes match we don't send an update to Octopus to avoid unnecessary updates
		if resourceHash == w.lastReceivedResourceEventHash && !forcePublish {
			return
		}

		if resourceHash != "" {
			w.lastReceivedResourceEventHash = resourceHash
		}
	}

	select {
	// To prevent deadlocks if for some reason the resultsChan doesn't have a receiver we want to unblock when the context is cancelled
	case w.resultsChan <- &resourceWatcherResult{
		Query:     w.query,
		Resources: resources,
		Error:     nil,
	}:
		// Successfully sent
	case <-w.context.Done():
		// Context cancelled, don't block on send
		w.logger.Debug("Skipping result publish due to cancelled context")
	}
}

func (w *resourceWatcher) publishError(err error) {
	select {
	// To prevent deadlocks if for some reason the resultsChan doesn't have a receiver we want to unblock when the context is cancelled
	case w.resultsChan <- &resourceWatcherResult{
		Query:     w.query,
		Resources: nil,
		Error:     err,
	}:
		// Successfully sent
	case <-w.context.Done():
		// Context cancelled, don't block on send
		w.logger.Debug("Skipping result publish due to cancelled context")
	}
}

func (w *resourceWatcher) cleanUp() {
	w.status = Stopped
	if err := w.resourceStream.CloseSend(); err != nil {
		w.publishError(err)
	}

	w.cancelFunc(nil)
}

func (w *resourceWatcher) watchResources() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	defer w.cleanUp()
	w.lastActivity = time.Now()
	w.status = Started

	// Required so the select statement below doesn't block the thread
	recvChan := make(chan struct{})

	initialRes, err := w.applicationService.GetApplicationResourceTree(
		w.query.ApplicationProject,
		w.query.ApplicationName,
		w.query.ApplicationNamespace,
	)
	if err != nil {
		w.publishError(err)
		return
	}

	if initialRes != nil {
		w.logger.Debug("Found initial resources for application")
		w.publishResult(initialRes, false)
	}

	// Resource tree event stream listener
	go w.resourceTreeEventListener(recvChan)

	// Resource tree polling loop
	refreshTicker := time.NewTicker(5 * time.Second)
	defer refreshTicker.Stop()
	go w.pollResourceTree(refreshTicker)

	for {
		select {
		case <-ticker.C:
			// Check if TTL expired
			if time.Since(w.lastActivity) > w.ttl {
				w.logger.Debug("Resource watcher TTL expired")
				return
			}
		case <-recvChan:
			// Message received and processed
			continue
		case <-w.context.Done():
			cancellationCause := context.Cause(w.context)
			w.logger.With(slog.Any("Cancellation Cause", cancellationCause)).Debug("Resource watcher was canceled")
			return
		}
	}
}

func (w *resourceWatcher) resourceTreeEventListener(recvChan chan struct{}) {
	for {
		msg, err := w.resourceStream.Recv()
		if err != nil {
			w.publishError(err)
			return
		}

		w.lastActivity = time.Now()
		w.logger.Debug("Received resource update from Argo")
		if msg != nil {
			w.publishResult(msg, true)
		}
		recvChan <- struct{}{}
	}
}

func (w *resourceWatcher) pollResourceTree(ticker *time.Ticker) {
	for {
		select {
		case <-w.context.Done():
			return
		case <-ticker.C:
			res, err := w.applicationService.GetApplicationResourceTree(
				w.query.ApplicationProject,
				w.query.ApplicationName,
				w.query.ApplicationNamespace,
			)
			if err != nil {
				w.publishError(err)
				return
			}

			if res != nil {
				w.publishResult(res, false)
			}
		}
	}
}

func hashNodes(nodes []v1alpha1.ResourceNode) (string, error) {
	b, err := json.Marshal(nodes)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum), nil
}
