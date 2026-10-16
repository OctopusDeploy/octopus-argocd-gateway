package applicationsync

import (
	"errors"
	"testing"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"go.uber.org/mock/gomock"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/applicationsync/applicationsynctest"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/testutilities"
)

func TestHandler_ProcessRequest_SuccessfulSync(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()

	app := &v1alpha1.Application{
		Spec: v1alpha1.ApplicationSpec{
			SyncPolicy: &v1alpha1.SyncPolicy{
				Automated: &v1alpha1.SyncPolicyAutomated{
					Prune: true,
				},
			},
		},
	}

	mockService := applicationsynctest.NewBuilder(ctrl).
		WithGetApplication(func(project, name, namespace string) (*v1alpha1.Application, error) {
			if project != "default" || name != "test-app" || namespace != "argocd" {
				t.Errorf("unexpected parameters: project=%s, name=%s, namespace=%s", project, name, namespace)
			}
			return app, nil
		}).
		WithSyncApplication(func(project, name, namespace string, opts *argocd.SyncOptions) (*v1alpha1.Application, error) {
			if !opts.Prune {
				t.Error("expected prune to be true from sync policy")
			}
			return app, nil
		}).
		Build()

	handler := &Handler{
		logger:             logger,
		applicationService: mockService,
	}

	request := &pb.SyncApplicationRequest{
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
}

func TestHandler_ProcessRequest_GetApplicationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()

	mockService := applicationsynctest.NewBuilder(ctrl).
		WithGetApplication(func(_, _, _ string) (*v1alpha1.Application, error) {
			return nil, errors.New("application not found")
		}).
		WithSyncApplication(func(_, _, _ string, _ *argocd.SyncOptions) (*v1alpha1.Application, error) {
			t.Error("sync should not be called when get application fails")
			return nil, nil
		}).
		Build()

	handler := &Handler{
		logger:             logger,
		applicationService: mockService,
	}

	request := &pb.SyncApplicationRequest{
		RequestId:            "req-456",
		ApplicationName:      "missing-app",
		ApplicationProject:   "default",
		ApplicationNamespace: "argocd",
	}

	response, err := handler.ProcessRequest(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response.RequestError == nil {
		t.Fatal("expected error in response")
	}

	if response.RequestError.Message != "application not found" {
		t.Errorf("expected error message %q, got %q", "application not found", response.RequestError.Message)
	}
}

func TestHandler_ProcessRequest_SyncApplicationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()

	app := &v1alpha1.Application{}

	mockService := applicationsynctest.NewBuilder(ctrl).
		WithGetApplication(func(_, _, _ string) (*v1alpha1.Application, error) {
			return app, nil
		}).
		WithSyncApplication(func(_, _, _ string, _ *argocd.SyncOptions) (*v1alpha1.Application, error) {
			return nil, errors.New("sync failed: permission denied")
		}).
		Build()

	handler := &Handler{
		logger:             logger,
		applicationService: mockService,
	}

	request := &pb.SyncApplicationRequest{
		RequestId:            "req-789",
		ApplicationName:      "test-app",
		ApplicationProject:   "default",
		ApplicationNamespace: "argocd",
	}

	response, err := handler.ProcessRequest(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response.RequestError == nil {
		t.Fatal("expected error in response")
	}

	if response.RequestError.Message != "sync failed: permission denied" {
		t.Errorf("expected error message %q, got %q", "sync failed: permission denied", response.RequestError.Message)
	}
}

func TestHandler_ProcessRequest_NoSyncPolicy(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testutilities.CreateTestLogger()

	appWithoutPolicy := &v1alpha1.Application{
		Spec: v1alpha1.ApplicationSpec{
			SyncPolicy: nil,
		},
	}

	var capturedOpts *argocd.SyncOptions
	mockService := applicationsynctest.NewBuilder(ctrl).
		WithGetApplication(func(_, _, _ string) (*v1alpha1.Application, error) {
			return appWithoutPolicy, nil
		}).
		WithSyncApplication(func(_, _, _ string, opts *argocd.SyncOptions) (*v1alpha1.Application, error) {
			capturedOpts = opts
			return appWithoutPolicy, nil
		}).
		Build()

	handler := &Handler{
		logger:             logger,
		applicationService: mockService,
	}

	request := &pb.SyncApplicationRequest{
		RequestId:            "req-000",
		ApplicationName:      "test-app",
		ApplicationProject:   "default",
		ApplicationNamespace: "argocd",
	}

	_, err := handler.ProcessRequest(request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedOpts == nil {
		t.Fatal("expected sync options to be passed")
	}

	if capturedOpts.Prune {
		t.Error("expected prune to be false when no sync policy")
	}
}
