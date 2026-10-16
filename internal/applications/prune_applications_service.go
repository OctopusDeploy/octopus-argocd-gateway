package applications

import (
	"context"
	"log/slog"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/samber/lo"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopus"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
)

type applicationService interface {
	ListApplications(applicationProject string) (*v1alpha1.ApplicationList, error)
}

type PruneApplicationsService struct {
	logger             *slog.Logger
	ctx                context.Context
	applicationService applicationService
	client             pb.PruneApplicationsServiceClient
	argoClient         argocd.Client
}

func NewPruneApplicationsService(
	logger *slog.Logger,
	ctx context.Context,
	octopusClient octopus.Client,
	argoClient argocd.Client,
) (*PruneApplicationsService, error) {
	return &PruneApplicationsService{
		logger:             logger,
		ctx:                ctx,
		applicationService: argoClient.Applications(),
		client:             octopusClient.PruneApplications(),
		argoClient:         argoClient,
	}, nil
}

func (s PruneApplicationsService) PruneApplications() (*pb.PruneApplicationsResponse, error) {
	s.logger.Info("Pruning stale applications from Octopus Server")

	projects := s.argoClient.AvailableProjects()
	var currentApplicationUids []*pb.ApplicationUid
	if len(projects) == 0 {
		// Global-token mode: one unfiltered list call covers every project.
		appUids, err := s.getApplicationUidsForProject("")
		if err != nil {
			return nil, err
		}
		currentApplicationUids = appUids
	} else {
		for _, projectName := range projects {
			projectAppUids, err := s.getApplicationUidsForProject(projectName)
			if err != nil {
				return nil, err
			}

			currentApplicationUids = append(currentApplicationUids, projectAppUids...)
		}
	}
	s.logger.With(slog.Any("applicationUids", currentApplicationUids)).Debug("Current Argo CD Application UIDs")

	request := &pb.PruneApplicationsRequest{
		CurrentApplicationUids: currentApplicationUids,
	}
	response, err := s.client.PruneStaleApplications(s.ctx, request)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (s PruneApplicationsService) getApplicationUidsForProject(projectName string) ([]*pb.ApplicationUid, error) {
	appList, err := s.applicationService.ListApplications(projectName)
	if err != nil {
		return nil, err
	}

	currentApplicationUids := lo.Map(appList.Items, func(item v1alpha1.Application, i int) *pb.ApplicationUid {
		return &pb.ApplicationUid{Value: string(item.UID)}
	})

	return currentApplicationUids, nil
}
