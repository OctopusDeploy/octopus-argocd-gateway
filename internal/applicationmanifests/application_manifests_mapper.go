package applicationmanifests

import (
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"

	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
)

func MapManagedManifests(requestId string, manifests *v1alpha1.ResourceDiff) (*pb.GetResourceManifestsResponse, error) {
	predictedLiveManifest := manifests.PredictedLiveState
	if predictedLiveManifest == "null" {
		predictedLiveManifest = ""
	}

	normalizedLiveManifest := manifests.NormalizedLiveState
	if normalizedLiveManifest == "null" {
		normalizedLiveManifest = ""
	}

	liveManifest := manifests.LiveState
	if liveManifest == "null" || liveManifest == "" {
		liveManifest = ""
	}

	targetManifest := manifests.TargetState
	if targetManifest == "null" || targetManifest == "" {
		targetManifest = ""
	}

	return &pb.GetResourceManifestsResponse{
		RequestId:              requestId,
		TargetManifest:         targetManifest,
		LiveManifest:           liveManifest,
		NormalizedLiveManifest: normalizedLiveManifest,
		PredictedLiveManifest:  predictedLiveManifest,
	}, nil
}
