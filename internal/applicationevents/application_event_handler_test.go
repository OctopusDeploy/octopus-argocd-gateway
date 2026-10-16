package applicationevents

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/metrics"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/testutilities"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/testutilities/mock"
)

type mockClusterService struct {
	getClusterFunc func(ctx context.Context, clusterId string, idType argocd.ClusterIdType) (*v1alpha1.Cluster, error)
}

func (m *mockClusterService) ListClusters() (*v1alpha1.ClusterList, error) {
	return nil, nil
}

func (m *mockClusterService) GetCluster(
	ctx context.Context,
	clusterId string,
	idType argocd.ClusterIdType,
) (*v1alpha1.Cluster, error) {
	return m.getClusterFunc(ctx, clusterId, idType)
}

type mockProjectService struct {
	getProjectFunc func(ctx context.Context, name string) (*v1alpha1.AppProject, error)
}

func (m *mockProjectService) GetProject(
	ctx context.Context,
	name string,
) (*v1alpha1.AppProject, error) {
	if m.getProjectFunc == nil {
		return nil, fmt.Errorf("%w: get project", argocd.ErrNotFound)
	}
	return m.getProjectFunc(ctx, name)
}

func TestHandler_ProcessMessage_SendsEventToOctopus(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()
	ctx := context.Background()

	cluster := &v1alpha1.Cluster{
		Name:   "test-cluster",
		Server: "https://kubernetes.default.svc",
	}

	clusterService := &mockClusterService{
		getClusterFunc: func(_ context.Context, clusterId string, idType argocd.ClusterIdType) (*v1alpha1.Cluster, error) {
			if clusterId != "https://kubernetes.default.svc" {
				t.Errorf("expected cluster ID %q, got %q", "https://kubernetes.default.svc", clusterId)
			}
			if idType != argocd.ClusterIdServer {
				t.Errorf("expected id type %q, got %q", argocd.ClusterIdServer, idType)
			}
			return cluster, nil
		},
	}

	eventClient := mock.NewMockApplicationEventServiceClient(ctrl)
	eventClient.EXPECT().
		ApplicationEventUpdate(gomock.Any(), gomock.Any()).
		Return(&pb.ApplicationEventUpdateResponse{}, nil)

	handler := &Handler{
		logger:         logger,
		ctx:            ctx,
		client:         eventClient,
		clusterService: clusterService,
		projectService: &mockProjectService{},
		metrics:        &metrics.NoopMetrics{},
	}

	event := &v1alpha1.ApplicationWatchEvent{
		Type: "ADDED",
		Application: v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{
				Destination: v1alpha1.ApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "default",
				},
			},
		},
	}

	err := handler.ProcessMessage(event)
	if err != nil {
		t.Errorf("ProcessMessage returned unexpected error: %v", err)
	}
}

func TestHandler_ProcessMessage_UsesClusterNameWhenProvided(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()
	ctx := context.Background()

	cluster := &v1alpha1.Cluster{
		Name:   "production-cluster",
		Server: "https://prod.example.com",
	}

	clusterService := &mockClusterService{
		getClusterFunc: func(_ context.Context, clusterId string, idType argocd.ClusterIdType) (*v1alpha1.Cluster, error) {
			if clusterId != "production-cluster" {
				t.Errorf("expected cluster ID %q, got %q", "production-cluster", clusterId)
			}
			if idType != argocd.ClusterIdName {
				t.Errorf("expected id type %q, got %q", argocd.ClusterIdName, idType)
			}
			return cluster, nil
		},
	}

	eventClient := mock.NewMockApplicationEventServiceClient(ctrl)
	eventClient.EXPECT().
		ApplicationEventUpdate(gomock.Any(), gomock.Any()).
		Return(&pb.ApplicationEventUpdateResponse{}, nil)

	handler := &Handler{
		logger:         logger,
		ctx:            ctx,
		client:         eventClient,
		clusterService: clusterService,
		projectService: &mockProjectService{},
		metrics:        &metrics.NoopMetrics{},
	}

	event := &v1alpha1.ApplicationWatchEvent{
		Type: "MODIFIED",
		Application: v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{
				Destination: v1alpha1.ApplicationDestination{
					Name:      "production-cluster",
					Namespace: "default",
				},
			},
		},
	}

	err := handler.ProcessMessage(event)
	if err != nil {
		t.Errorf("ProcessMessage returned unexpected error: %v", err)
	}
}

func TestHandler_ProcessMessage_ContinuesAndLogsOnceOnClusterError(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()
	ctx := context.Background()

	// clusterService isn't a gomock yet, we should probably do that at some point
	callCount := 0
	clusterService := &mockClusterService{
		getClusterFunc: func(_ context.Context, clusterId string, idType argocd.ClusterIdType) (*v1alpha1.Cluster, error) {
			callCount++
			return nil, fmt.Errorf("%w: get cluster: missing", argocd.ErrNotFound)
		},
	}

	eventClient := mock.NewMockApplicationEventServiceClient(ctrl)
	eventClient.EXPECT().
		ApplicationEventUpdate(gomock.Any(), gomock.Any()).
		Return(&pb.ApplicationEventUpdateResponse{}, nil).
		Times(3)

	handler := &Handler{
		logger:         logger,
		ctx:            ctx,
		client:         eventClient,
		clusterService: clusterService,
		projectService: &mockProjectService{},
		metrics:        &metrics.NoopMetrics{},
	}

	event := &v1alpha1.ApplicationWatchEvent{
		Type: "ADDED",
		Application: v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{
				Destination: v1alpha1.ApplicationDestination{
					Server:    "https://missing.example.com",
					Namespace: "default",
				},
			},
		},
	}

	for i := 0; i < 3; i++ {
		if err := handler.ProcessMessage(event); err != nil {
			t.Fatalf("ProcessMessage call %d returned unexpected error: %v", i+1, err)
		}
	}

	if callCount != 3 {
		t.Errorf("expected GetCluster to be called 3 times, got %d", callCount)
	}

	missKey := missingClusterKey{id: "https://missing.example.com", idType: argocd.ClusterIdServer}
	if _, ok := handler.loggedMissingClusters.Load(missKey); !ok {
		t.Errorf("expected cluster ID to be recorded in loggedMissingClusters after first miss")
	}

	logged := 0
	handler.loggedMissingClusters.Range(func(_, _ any) bool {
		logged++
		return true
	})
	if logged != 1 {
		t.Errorf("expected exactly one entry in loggedMissingClusters, got %d", logged)
	}
}

func TestHandler_ProcessMessage_UsesApplicationProjectForTokenContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()

	// The shared dispatcher processes events from every project, so the handler must derive the
	// project from the application itself (Spec.Project) and pass it on the context used for cluster
	// resolution, so the auth interceptor selects that project's token.
	var gotProject string
	var gotOK bool
	clusterService := &mockClusterService{
		getClusterFunc: func(ctx context.Context, _ string, _ argocd.ClusterIdType) (*v1alpha1.Cluster, error) {
			gotProject, gotOK = argocd.ArgoProjectFromContext(ctx)
			return &v1alpha1.Cluster{Name: "c", Server: "https://kubernetes.default.svc"}, nil
		},
	}

	eventClient := mock.NewMockApplicationEventServiceClient(ctrl)
	eventClient.EXPECT().
		ApplicationEventUpdate(gomock.Any(), gomock.Any()).
		Return(&pb.ApplicationEventUpdateResponse{}, nil)

	handler := &Handler{
		logger:         logger,
		ctx:            context.Background(),
		client:         eventClient,
		clusterService: clusterService,
		projectService: &mockProjectService{},
		metrics:        &metrics.NoopMetrics{},
	}

	event := &v1alpha1.ApplicationWatchEvent{
		Type: "ADDED",
		Application: v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "argocd"},
			Spec: v1alpha1.ApplicationSpec{
				Project: "team-a",
				Destination: v1alpha1.ApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "default",
				},
			},
		},
	}

	if err := handler.ProcessMessage(event); err != nil {
		t.Fatalf("ProcessMessage returned unexpected error: %v", err)
	}

	if !gotOK {
		t.Fatal("expected the application's project to be set on the cluster-resolution context")
	}
	if gotProject != "team-a" {
		t.Errorf("expected project %q on context, got %q", "team-a", gotProject)
	}
}

func TestHandler_ProcessMessage_NoProjectContextWhenApplicationHasNone(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()

	// With no project on the application (e.g. a single global auth token), the base context is used
	// unchanged and the global resolver picks the only token regardless.
	var gotOK bool
	clusterService := &mockClusterService{
		getClusterFunc: func(ctx context.Context, _ string, _ argocd.ClusterIdType) (*v1alpha1.Cluster, error) {
			_, gotOK = argocd.ArgoProjectFromContext(ctx)
			return &v1alpha1.Cluster{Name: "c", Server: "https://kubernetes.default.svc"}, nil
		},
	}

	eventClient := mock.NewMockApplicationEventServiceClient(ctrl)
	eventClient.EXPECT().
		ApplicationEventUpdate(gomock.Any(), gomock.Any()).
		Return(&pb.ApplicationEventUpdateResponse{}, nil)

	handler := &Handler{
		logger:         logger,
		ctx:            context.Background(),
		client:         eventClient,
		clusterService: clusterService,
		projectService: &mockProjectService{},
		metrics:        &metrics.NoopMetrics{},
	}

	event := &v1alpha1.ApplicationWatchEvent{
		Type: "ADDED",
		Application: v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "argocd"},
			Spec: v1alpha1.ApplicationSpec{
				Destination: v1alpha1.ApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "default",
				},
			},
		},
	}

	if err := handler.ProcessMessage(event); err != nil {
		t.Fatalf("ProcessMessage returned unexpected error: %v", err)
	}

	if gotOK {
		t.Error("expected no project on the cluster-resolution context when the application has none")
	}
}

func TestHandler_ProcessMessage_TimesOutHungSend(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()

	clusterService := &mockClusterService{
		getClusterFunc: func(_ context.Context, _ string, _ argocd.ClusterIdType) (*v1alpha1.Cluster, error) {
			return &v1alpha1.Cluster{Name: "c", Server: "https://kubernetes.default.svc"}, nil
		},
	}

	// Simulate a server that accepts the call but never responds: the send returns only once its
	// context is cancelled, which is exactly what the eventTimeout must do.
	eventClient := mock.NewMockApplicationEventServiceClient(ctrl)
	eventClient.EXPECT().
		ApplicationEventUpdate(gomock.Any(), gomock.Any()).
		DoAndReturn(
			func(
				ctx context.Context,
				_ *pb.ApplicationEventUpdateRequest,
				_ ...grpc.CallOption,
			) (*pb.ApplicationEventUpdateResponse, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		)

	handler := &Handler{
		logger:         logger,
		ctx:            context.Background(),
		client:         eventClient,
		clusterService: clusterService,
		projectService: &mockProjectService{},
		metrics:        &metrics.NoopMetrics{},
		eventTimeout:   50 * time.Millisecond,
	}

	event := &v1alpha1.ApplicationWatchEvent{
		Type: "MODIFIED",
		Application: v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "argocd"},
			Spec: v1alpha1.ApplicationSpec{
				Destination: v1alpha1.ApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "default",
				},
			},
		},
	}

	start := time.Now()
	err := handler.ProcessMessage(event)
	elapsed := time.Since(start)

	// The error is surfaced so the dispatcher retries, and the worker is freed near eventTimeout
	// rather than blocking forever.
	if err == nil {
		t.Fatal("expected the hung send to return a timeout error, got nil")
	}
	if elapsed > time.Second {
		t.Errorf("expected the send to be bounded near eventTimeout (50ms), but it took %v", elapsed)
	}
}

func TestHandler_ProcessMessage_TracksMissingClustersIndependently(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()
	ctx := context.Background()

	clusterService := &mockClusterService{
		getClusterFunc: func(_ context.Context, clusterId string, idType argocd.ClusterIdType) (*v1alpha1.Cluster, error) {
			return nil, fmt.Errorf("%w: get cluster: missing", argocd.ErrNotFound)
		},
	}

	eventClient := mock.NewMockApplicationEventServiceClient(ctrl)
	eventClient.EXPECT().
		ApplicationEventUpdate(gomock.Any(), gomock.Any()).
		Return(&pb.ApplicationEventUpdateResponse{}, nil).
		AnyTimes()

	handler := &Handler{
		logger:         logger,
		ctx:            ctx,
		client:         eventClient,
		clusterService: clusterService,
		projectService: &mockProjectService{},
		metrics:        &metrics.NoopMetrics{},
	}

	makeEvent := func(server string) *v1alpha1.ApplicationWatchEvent {
		return &v1alpha1.ApplicationWatchEvent{
			Type: "ADDED",
			Application: v1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "argocd"},
				Spec: v1alpha1.ApplicationSpec{
					Destination: v1alpha1.ApplicationDestination{
						Server:    server,
						Namespace: "default",
					},
				},
			},
		}
	}

	for _, server := range []string{"https://a.example.com", "https://b.example.com", "https://a.example.com"} {
		if err := handler.ProcessMessage(makeEvent(server)); err != nil {
			t.Fatalf("ProcessMessage returned unexpected error: %v", err)
		}
	}

	logged := 0
	handler.loggedMissingClusters.Range(func(_, _ any) bool {
		logged++
		return true
	})
	if logged != 2 {
		t.Errorf("expected 2 distinct missing cluster entries, got %d", logged)
	}
}

func TestHandler_ProcessMessage_InjectsProjectAnnotations(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()
	ctx := context.Background()

	clusterService := &mockClusterService{
		getClusterFunc: func(_ context.Context, _ string, _ argocd.ClusterIdType) (*v1alpha1.Cluster, error) {
			return &v1alpha1.Cluster{
				Name:   "test-cluster",
				Server: "https://kubernetes.default.svc",
				Annotations: map[string]string{
					argocd.OctopusAnnotationsPrefix + "source":       "cluster",
					argocd.OctopusAnnotationsPrefix + "cluster-only": "yes",
				},
			}, nil
		},
	}

	var gotProjectName string
	projectService := &mockProjectService{
		getProjectFunc: func(_ context.Context, name string) (*v1alpha1.AppProject, error) {
			gotProjectName = name
			return &v1alpha1.AppProject{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
					Annotations: map[string]string{
						argocd.OctopusAnnotationsPrefix + "source":       "project",
						argocd.OctopusAnnotationsPrefix + "project-only": "yes",
						"unrelated.example.com/ignored":                  "yes",
					},
				},
			}, nil
		},
	}

	eventClient := mock.NewMockApplicationEventServiceClient(ctrl)
	eventClient.EXPECT().
		ApplicationEventUpdate(gomock.Any(), gomock.Any()).
		Return(&pb.ApplicationEventUpdateResponse{}, nil)

	handler := &Handler{
		logger:         logger,
		ctx:            ctx,
		client:         eventClient,
		clusterService: clusterService,
		projectService: projectService,
		metrics:        &metrics.NoopMetrics{},
	}

	event := &v1alpha1.ApplicationWatchEvent{
		Type: "ADDED",
		Application: v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{
				Project: "my-project",
				Destination: v1alpha1.ApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "default",
				},
			},
		},
	}

	if err := handler.ProcessMessage(event); err != nil {
		t.Fatalf("ProcessMessage returned unexpected error: %v", err)
	}

	if gotProjectName != "my-project" {
		t.Errorf("expected project lookup for %q, got %q", "my-project", gotProjectName)
	}

	annotations := event.Application.Annotations
	// Cluster wins over project: the destination is more specific than the project.
	if got := annotations[argocd.OctopusAnnotationsPrefix+"source"]; got != "cluster" {
		t.Errorf("expected cluster annotation to win, got %q", got)
	}
	if got := annotations[argocd.OctopusAnnotationsPrefix+"project-only"]; got != "yes" {
		t.Errorf("expected project annotation to be injected, got %q", got)
	}
	if _, ok := annotations["unrelated.example.com/ignored"]; ok {
		t.Error("expected non-Octopus project annotations to be filtered out")
	}
}

func TestHandler_ProcessMessage_LooksUpDefaultProjectWhenUnset(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()
	ctx := context.Background()

	clusterService := &mockClusterService{
		getClusterFunc: func(_ context.Context, _ string, _ argocd.ClusterIdType) (*v1alpha1.Cluster, error) {
			return &v1alpha1.Cluster{Name: "c", Server: "https://kubernetes.default.svc"}, nil
		},
	}

	var gotProjectName string
	projectService := &mockProjectService{
		getProjectFunc: func(_ context.Context, name string) (*v1alpha1.AppProject, error) {
			gotProjectName = name
			return &v1alpha1.AppProject{ObjectMeta: metav1.ObjectMeta{Name: name}}, nil
		},
	}

	eventClient := mock.NewMockApplicationEventServiceClient(ctrl)
	eventClient.EXPECT().
		ApplicationEventUpdate(gomock.Any(), gomock.Any()).
		Return(&pb.ApplicationEventUpdateResponse{}, nil)

	handler := &Handler{
		logger:         logger,
		ctx:            ctx,
		client:         eventClient,
		clusterService: clusterService,
		projectService: projectService,
		metrics:        &metrics.NoopMetrics{},
	}

	event := &v1alpha1.ApplicationWatchEvent{
		Type: "ADDED",
		Application: v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "argocd"},
			Spec: v1alpha1.ApplicationSpec{
				Destination: v1alpha1.ApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "default",
				},
			},
		},
	}

	if err := handler.ProcessMessage(event); err != nil {
		t.Fatalf("ProcessMessage returned unexpected error: %v", err)
	}

	if gotProjectName != v1alpha1.DefaultAppProjectName {
		t.Errorf("expected project lookup for %q, got %q", v1alpha1.DefaultAppProjectName, gotProjectName)
	}
}
