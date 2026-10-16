package applicationresourcetree

import (
	"errors"
	"testing"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"go.uber.org/mock/gomock"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/applicationresourcetree/applicationresourcetreetest"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/testutilities"
)

func TestHandler_ProcessRequest_SuccessfulTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()

	tree := &v1alpha1.ApplicationTree{
		Nodes: []v1alpha1.ResourceNode{
			{
				ResourceRef: v1alpha1.ResourceRef{
					UID:       "uid-root",
					Name:      "root",
					Namespace: "argocd",
					Group:     "apps",
					Version:   "v1",
					Kind:      "Deployment",
				},
				Health: &v1alpha1.HealthStatus{
					Status:  "Healthy",
					Message: "all good",
				},
			},
			{
				ResourceRef: v1alpha1.ResourceRef{
					UID:       "uid-child",
					Name:      "child",
					Namespace: "argocd",
					Version:   "v1",
					Kind:      "ReplicaSet",
				},
				ParentRefs: []v1alpha1.ResourceRef{
					{UID: ""},
					{UID: "uid-root"},
				},
			},
		},
	}

	mockService := applicationresourcetreetest.NewBuilder(ctrl).
		WithGetApplicationResourceTree(func(project, name, namespace string) (*v1alpha1.ApplicationTree, error) {
			if project != "default" || name != "test-app" || namespace != "argocd" {
				t.Errorf("unexpected parameters: project=%s, name=%s, namespace=%s", project, name, namespace)
			}
			return tree, nil
		}).
		Build()

	handler := &Handler{
		logger:             logger,
		applicationService: mockService,
	}

	request := &pb.GetApplicationResourceTreeRequest{
		RequestId:            "req-123",
		ApplicationName:      "test-app",
		ApplicationProject:   "default",
		ApplicationNamespace: "argocd",
	}

	response, err := handler.ProcessRequest(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response.RequestId != "req-123" {
		t.Errorf("expected request ID %q, got %q", "req-123", response.RequestId)
	}

	if response.RequestError != nil {
		t.Errorf("unexpected error in response: %v", response.RequestError)
	}

	if len(response.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(response.Nodes))
	}

	root := response.Nodes[0]
	if root.Uid != "uid-root" || root.Kind != "Deployment" || root.Group != "apps" {
		t.Errorf("unexpected root node mapping: %+v", root)
	}
	if root.HealthStatusCode != "Healthy" || root.HealthStatusMessage != "all good" {
		t.Errorf("unexpected health mapping: code=%q message=%q", root.HealthStatusCode, root.HealthStatusMessage)
	}
	if root.ParentUid != "" {
		t.Errorf("expected empty parent UID for root, got %q", root.ParentUid)
	}

	child := response.Nodes[1]
	if child.ParentUid != "uid-root" {
		t.Errorf("expected child parent UID %q, got %q", "uid-root", child.ParentUid)
	}
	if child.HealthStatusCode != "" || child.HealthStatusMessage != "" {
		t.Errorf(
			"expected empty health for node without health, got code=%q message=%q",
			child.HealthStatusCode,
			child.HealthStatusMessage,
		)
	}
}

func TestHandler_ProcessRequest_GetTreeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()

	mockService := applicationresourcetreetest.NewBuilder(ctrl).
		WithGetApplicationResourceTree(func(_, _, _ string) (*v1alpha1.ApplicationTree, error) {
			return nil, errors.New("application not found")
		}).
		Build()

	handler := &Handler{
		logger:             logger,
		applicationService: mockService,
	}

	request := &pb.GetApplicationResourceTreeRequest{
		RequestId:            "req-456",
		ApplicationName:      "missing-app",
		ApplicationProject:   "default",
		ApplicationNamespace: "argocd",
	}

	response, err := handler.ProcessRequest(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response.RequestId != "req-456" {
		t.Errorf("expected request ID %q, got %q", "req-456", response.RequestId)
	}

	if response.RequestError == nil {
		t.Fatal("expected a request error, got nil")
	}

	if response.RequestError.Code != pb.ErrorCode_ERROR_CODE_UNEXPECTED {
		t.Errorf("expected error code %v, got %v", pb.ErrorCode_ERROR_CODE_UNEXPECTED, response.RequestError.Code)
	}

	if response.RequestError.Message != "application not found" {
		t.Errorf("unexpected error message: %q", response.RequestError.Message)
	}

	if response.Nodes != nil {
		t.Errorf("expected no nodes on error, got %d", len(response.Nodes))
	}
}
