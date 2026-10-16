package applicationevents

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/testutilities"
)

func resyncApp(name string) v1alpha1.Application {
	return v1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "argocd", UID: types.UID(name)},
	}
}

// recordingSink captures the events the resyncer forwards.
type recordingSink struct {
	mu     sync.Mutex
	events []*v1alpha1.ApplicationWatchEvent
}

func (s *recordingSink) ProcessMessage(e *v1alpha1.ApplicationWatchEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return nil
}

// fakePruner records prune calls and can return a canned error.
type fakePruner struct {
	calls int
	err   error
}

func (p *fakePruner) PruneApplications() (*pb.PruneApplicationsResponse, error) {
	p.calls++
	return &pb.PruneApplicationsResponse{}, p.err
}

// fakeAppService implements only ListApplications; the rest of argocd.ApplicationService is unused.
type fakeAppService struct {
	argocd.ApplicationService
	lists map[string]*v1alpha1.ApplicationList
	errs  map[string]error
	calls []string
}

func (f *fakeAppService) ListApplications(project string) (*v1alpha1.ApplicationList, error) {
	f.calls = append(f.calls, project)
	if err := f.errs[project]; err != nil {
		return nil, err
	}
	return f.lists[project], nil
}

// fakeArgoClient implements only the methods the resyncer uses.
type fakeArgoClient struct {
	argocd.Client
	projects []string
	appSvc   argocd.ApplicationService
}

func (f *fakeArgoClient) AvailableProjects() []string             { return f.projects }
func (f *fakeArgoClient) Applications() argocd.ApplicationService { return f.appSvc }

func newTestResyncer(client argocd.Client, sink *recordingSink, pruner applicationPruner) *resyncer {
	return newResyncer(testutilities.CreateTestLogger(), time.Minute, client, sink, pruner)
}

func TestResyncer_GlobalMode_ForwardsAllAppsThenPrunes(t *testing.T) {
	appSvc := &fakeAppService{
		lists: map[string]*v1alpha1.ApplicationList{
			"": {Items: []v1alpha1.Application{resyncApp("a"), resyncApp("b")}},
		},
	}
	client := &fakeArgoClient{projects: nil, appSvc: appSvc}
	sink := &recordingSink{}
	pruner := &fakePruner{}

	newTestResyncer(client, sink, pruner).reconcile(context.Background())

	if got := appSvc.calls; len(got) != 1 || got[0] != "" {
		t.Fatalf("expected a single global list call, got %v", got)
	}
	if len(sink.events) != 2 {
		t.Fatalf("expected 2 forwarded events, got %d", len(sink.events))
	}
	for _, e := range sink.events {
		if e.Type != watch.Modified {
			t.Fatalf("expected forwarded events to be Modified, got %s", e.Type)
		}
	}
	if pruner.calls != 1 {
		t.Fatalf("expected prune to be called once, got %d", pruner.calls)
	}
}

func TestResyncer_ProjectMode_ListsEachProject(t *testing.T) {
	appSvc := &fakeAppService{
		lists: map[string]*v1alpha1.ApplicationList{
			"proj1": {Items: []v1alpha1.Application{resyncApp("a")}},
			"proj2": {Items: []v1alpha1.Application{resyncApp("b"), resyncApp("c")}},
		},
	}
	client := &fakeArgoClient{projects: []string{"proj1", "proj2"}, appSvc: appSvc}
	sink := &recordingSink{}
	pruner := &fakePruner{}

	newTestResyncer(client, sink, pruner).reconcile(context.Background())

	if len(appSvc.calls) != 2 {
		t.Fatalf("expected a list call per project, got %v", appSvc.calls)
	}
	if len(sink.events) != 3 {
		t.Fatalf("expected 3 forwarded events across projects, got %d", len(sink.events))
	}
	if pruner.calls != 1 {
		t.Fatalf("expected prune to be called once, got %d", pruner.calls)
	}
}

func TestResyncer_ListErrorForOneProject_ContinuesAndStillPrunes(t *testing.T) {
	appSvc := &fakeAppService{
		lists: map[string]*v1alpha1.ApplicationList{
			"proj2": {Items: []v1alpha1.Application{resyncApp("b")}},
		},
		errs: map[string]error{"proj1": errors.New("boom")},
	}
	client := &fakeArgoClient{projects: []string{"proj1", "proj2"}, appSvc: appSvc}
	sink := &recordingSink{}
	pruner := &fakePruner{}

	newTestResyncer(client, sink, pruner).reconcile(context.Background())

	if len(sink.events) != 1 {
		t.Fatalf("expected proj2's app to still be forwarded after proj1 list error, got %d", len(sink.events))
	}
	if pruner.calls != 1 {
		t.Fatalf("expected prune to still run after a list error, got %d", pruner.calls)
	}
}

func TestResyncer_PruneUnimplemented_IsTolerated(t *testing.T) {
	appSvc := &fakeAppService{
		lists: map[string]*v1alpha1.ApplicationList{
			"": {Items: []v1alpha1.Application{resyncApp("a")}},
		},
	}
	client := &fakeArgoClient{appSvc: appSvc}
	sink := &recordingSink{}
	pruner := &fakePruner{err: status.Error(codes.Unimplemented, "not implemented")}

	// Must not panic and must still have forwarded the app.
	newTestResyncer(client, sink, pruner).reconcile(context.Background())

	if len(sink.events) != 1 {
		t.Fatalf("expected the app to be forwarded even when prune is unimplemented, got %d", len(sink.events))
	}
	if pruner.calls != 1 {
		t.Fatalf("expected prune to be attempted once, got %d", pruner.calls)
	}
}
