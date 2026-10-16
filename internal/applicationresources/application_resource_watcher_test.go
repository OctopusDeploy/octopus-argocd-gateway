package applicationresources

import (
	"testing"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/mock/gomock"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/applicationresources/applicationresourcestest"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/testutilities"
)

var logger = testutilities.CreateTestLogger()

var query = &pb.WatchApplicationQuery{
	ApplicationUid:       &pb.ApplicationUid{Value: "mock"},
	ApplicationName:      "application",
	ApplicationProject:   "project",
	ApplicationNamespace: "namespace",
}

func TestNewResourceWatcher_ReturnsNewResourceWatcher(t *testing.T) {
	ctrl := gomock.NewController(t)
	mocks := createApplicationResourceWatcherMocks(ctrl, t)
	createResourceWatcher(t, mocks, make(chan *resourceWatcherResult), 5*time.Second, false)
}

func TestWatchResources_FetchesInitialResults_Successfully(t *testing.T) {
	ctrl := gomock.NewController(t)
	mocks := createApplicationResourceWatcherMocks(ctrl, t)

	expectedResult := &resourceWatcherResult{
		Query: query,
		Resources: &v1alpha1.ApplicationTree{
			Nodes: []v1alpha1.ResourceNode{{}},
		},
		Error: nil,
	}
	mocks.mockInitialResources(expectedResult.Resources)
	resultsChan := make(chan *resourceWatcherResult)
	createResourceWatcher(t, mocks, resultsChan, 5*time.Second, true)

	for {
		select {
		case result := <-resultsChan:
			if diff := cmp.Diff(
				expectedResult,
				result,
				cmpopts.IgnoreUnexported(pb.ApplicationUid{}, pb.WatchApplicationQuery{}),
			); diff != "" {
				t.Error(diff)
			}
			return
		case <-time.After(5 * time.Second):
			t.Error("Timed out waiting for initial results")
			return
		case <-t.Context().Done():
			t.Error("Test context was canceled before initial results were received")
			return
		}
	}
}

func TestWatchResources_PublishesResourceUpdates_Successfully(t *testing.T) {
	ctrl := gomock.NewController(t)
	mocks := createApplicationResourceWatcherMocks(ctrl, t)

	resourceUpdate := &resourceWatcherResult{
		Query: query,
		Resources: &v1alpha1.ApplicationTree{
			Nodes: []v1alpha1.ResourceNode{{}},
		},
		Error: nil,
	}
	mocks.mockInitialResources(resourceUpdate.Resources)
	resultsChan := make(chan *resourceWatcherResult)
	createResourceWatcher(t, mocks, resultsChan, 5*time.Second, true)

	// Get the initial resource results
	initialRes := <-resultsChan
	if diff := cmp.Diff(
		resourceUpdate,
		initialRes,
		cmpopts.IgnoreUnexported(pb.ApplicationUid{}, pb.WatchApplicationQuery{}),
	); diff != "" {
		t.Error(diff)
	}

	// Send another update
	mocks.applicationResourceWatchStream <- resourceUpdate.Resources
	resUpdateOne := <-resultsChan
	if diff := cmp.Diff(
		resourceUpdate,
		resUpdateOne,
		cmpopts.IgnoreUnexported(pb.ApplicationUid{}, pb.WatchApplicationQuery{}),
	); diff != "" {
		t.Error(diff)
	}

	// Send yet another update
	mocks.applicationResourceWatchStream <- resourceUpdate.Resources
	resUpdateTwo := <-resultsChan
	if diff := cmp.Diff(
		resourceUpdate,
		resUpdateTwo,
		cmpopts.IgnoreUnexported(pb.ApplicationUid{}, pb.WatchApplicationQuery{}),
	); diff != "" {
		t.Error(diff)
	}
}

func TestWatchResources_RespectsTtl(t *testing.T) {
	ctrl := gomock.NewController(t)
	mocks := createApplicationResourceWatcherMocks(ctrl, t)

	resourceUpdate := &resourceWatcherResult{
		Query: query,
		Resources: &v1alpha1.ApplicationTree{
			Nodes: []v1alpha1.ResourceNode{{}},
		},
		Error: nil,
	}
	mocks.mockInitialResources(resourceUpdate.Resources)
	resultsChan := make(chan *resourceWatcherResult)
	watcher := createResourceWatcher(t, mocks, resultsChan, 1*time.Second, true)

	// TTL is 1-second so sleep for 2 and hope that everything is cleaned up
	time.Sleep(2 * time.Second)
	if time.Since(watcher.lastActivity) <= watcher.ttl {
		t.Error("Expected watcher's ttl to be expired")
	}
}

func TestWatchResources_RespectsCancellation(t *testing.T) {
	ctrl := gomock.NewController(t, gomock.WithOverridableExpectations())
	mocks := createApplicationResourceWatcherMocks(ctrl, t)

	resourceUpdate := &resourceWatcherResult{
		Query: query,
		Resources: &v1alpha1.ApplicationTree{
			Nodes: []v1alpha1.ResourceNode{{}},
		},
		Error: nil,
	}
	mocks.mockInitialResources(resourceUpdate.Resources)
	resultsChan := make(chan *resourceWatcherResult)
	watcher := createResourceWatcher(t, mocks, resultsChan, 30*time.Second, true)

	// Get the initial resource results
	initialRes := <-resultsChan
	if diff := cmp.Diff(
		resourceUpdate,
		initialRes,
		cmpopts.IgnoreUnexported(pb.ApplicationUid{}, pb.WatchApplicationQuery{}),
	); diff != "" {
		t.Error(diff)
	}

	// Send another update
	mocks.applicationResourceWatchStream <- resourceUpdate.Resources
	resUpdateOne := <-resultsChan
	if diff := cmp.Diff(
		resourceUpdate,
		resUpdateOne,
		cmpopts.IgnoreUnexported(pb.ApplicationUid{}, pb.WatchApplicationQuery{}),
	); diff != "" {
		t.Error(diff)
	}

	// After the cancel function is called we should expect the stream to be closed
	mocks.applicationResourceWatchClient.EXPECT().CloseSend().Times(1)
	watcher.cancelFunc(nil)

	if err := applicationresourcestest.WaitForMocks(t.Context(), ctrl, 5*time.Second); err != nil {
		t.Error("call failed:", err)
	}
}

type applicationResourceWatcherMocks struct {
	ctrl                           *gomock.Controller
	applicationService             *applicationresourcestest.MockapplicationService
	applicationResourceWatchClient *applicationresourcestest.MockApplicationService_WatchResourceTreeClient
	applicationResourceWatchStream chan *v1alpha1.ApplicationTree
}

func createApplicationResourceWatcherMocks(ctrl *gomock.Controller, t *testing.T) *applicationResourceWatcherMocks {
	appService := applicationresourcestest.NewMockapplicationService(ctrl)
	applicationResourceWatchClient, watchStream := applicationresourcestest.NewMockResourceWatchClient(
		ctrl,
		t.Context(),
	)

	appService.EXPECT().
		WatchApplicationResourceTree(gomock.Any(), gomock.Eq("project"), gomock.Eq("application"), gomock.Eq("namespace")).
		Return(applicationResourceWatchClient, nil)

	return &applicationResourceWatcherMocks{
		ctrl:                           ctrl,
		applicationService:             appService,
		applicationResourceWatchClient: applicationResourceWatchClient,
		applicationResourceWatchStream: watchStream,
	}
}

func (m *applicationResourceWatcherMocks) mockInitialResources(resources *v1alpha1.ApplicationTree) {
	m.applicationService.EXPECT().
		GetApplicationResourceTree(gomock.Eq("project"), gomock.Eq("application"), gomock.Eq("namespace")).
		Return(resources, nil)
}

func createResourceWatcher(
	t *testing.T,
	mocks *applicationResourceWatcherMocks,
	resultsChan chan *resourceWatcherResult,
	ttl time.Duration,
	startWatch bool,
) *resourceWatcher {
	watcher, err := newResourceWatcher(
		logger,
		t.Context(),
		query,
		mocks.applicationService,
		resultsChan,
		ttl,
	)
	if err != nil {
		t.Error("Unexpected error while creating new resource watcher:", err)
	}

	if startWatch {
		go watcher.watchResources()
	}

	return watcher
}
