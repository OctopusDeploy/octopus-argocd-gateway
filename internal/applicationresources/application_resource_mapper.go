package applicationresources

import (
	"fmt"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/utils/kube"
	"github.com/samber/lo"

	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
)

type ApplicationResources struct {
	PresentResources []*pb.PresentApplicationResource
	MissingResources []*pb.MissingApplicationResource
	ChildResources   []*pb.ChildApplicationResource
}

type ApplicationResourceType string

const (
	Present ApplicationResourceType = "present"
	Missing ApplicationResourceType = "missing"
	Child   ApplicationResourceType = "child"
	Unknown ApplicationResourceType = "unknown"
)

func MapResourceNodesToApplicationResources(
	nodes []v1alpha1.ResourceNode,
	application *v1alpha1.Application,
) (*ApplicationResources, []error) {
	applicationResources := &ApplicationResources{
		PresentResources: make([]*pb.PresentApplicationResource, 0),
		MissingResources: make([]*pb.MissingApplicationResource, 0),
		ChildResources:   make([]*pb.ChildApplicationResource, 0),
	}

	resourceMap := lo.KeyBy(nodes, func(item v1alpha1.ResourceNode) string {
		return item.UID
	})

	errors := make([]error, 0)
	for _, node := range nodes {
		resourceNodeType := GetResourceNodeType(node)
		switch resourceNodeType {
		case Present:
			mapped, err := MapResourceNodeToPresentApplicationResource(node, application)
			if err != nil {
				errors = append(errors, err)
				continue
			}
			applicationResources.PresentResources = append(applicationResources.PresentResources, mapped)
		case Missing:
			mapped, err := MapResourceNodeToMissingApplicationResource(node)
			if err != nil {
				errors = append(errors, err)
				continue
			}
			applicationResources.MissingResources = append(applicationResources.MissingResources, mapped)
		case Child:
			mapped, err := MapResourceNodeToChildApplicationResource(resourceMap, node)
			if err != nil {
				errors = append(errors, err)
				continue
			}
			applicationResources.ChildResources = append(applicationResources.ChildResources, mapped)
		}
	}

	return applicationResources, errors
}

func GetResourceNodeType(node v1alpha1.ResourceNode) ApplicationResourceType {
	if node.ResourceVersion == "" && node.UID == "" && node.CreatedAt == nil {
		return Missing
	}

	if len(node.ParentRefs) <= 0 {
		return Present
	}

	if len(node.ParentRefs) > 0 {
		return Child
	}

	return Unknown
}

func MapResourceNodeToPresentApplicationResource(
	node v1alpha1.ResourceNode,
	application *v1alpha1.Application,
) (*pb.PresentApplicationResource, error) {
	// If the resource is missing these values won't exist
	if node.ResourceVersion == "" && node.UID == "" && node.CreatedAt == nil {
		return nil, fmt.Errorf("attempted to map missing resource(%s) as a present resource", node.FullName())
	}

	syncStatus, ok := getSyncStatus(node, application)

	if !ok {
		return nil, fmt.Errorf(
			"failed to find resource in application manifest while mapping resource(%s) as present resource",
			node.FullName(),
		)
	}

	presentResource := &pb.PresentApplicationResource{
		ResourceUid:            node.UID,
		Name:                   node.Name,
		KubernetesNamespace:    node.Namespace,
		Group:                  node.Group,
		Version:                node.Version,
		Kind:                   node.Kind,
		ResourceSyncStatusCode: syncStatus,
	}

	healthStatus := getHealthStatus(node)
	if healthStatus != nil {
		presentResource.ResourceStatusCode = healthStatus.resourceStatusCode
		presentResource.ResourceStatusMessage = healthStatus.resourceStatusMessage
	}

	return presentResource, nil
}

func MapResourceNodeToMissingApplicationResource(
	node v1alpha1.ResourceNode,
) (*pb.MissingApplicationResource, error) {
	// If the resource is missing these values won't exist
	if node.ResourceVersion != "" && node.UID != "" && node.CreatedAt != nil {
		return nil, fmt.Errorf("attempted to map present resource(%s) as a missing resource", node.FullName())
	}

	appResource := &pb.MissingApplicationResource{
		Name:                node.Name,
		KubernetesNamespace: node.Namespace,
		Group:               node.Group,
		Version:             node.Version,
		Kind:                node.Kind,
	}

	return appResource, nil
}

func MapResourceNodeToChildApplicationResource(
	resourceMap map[string]v1alpha1.ResourceNode,
	node v1alpha1.ResourceNode,
) (*pb.ChildApplicationResource, error) {
	// If the resource is missing these values won't exist
	if node.ResourceVersion == "" && node.UID == "" && node.CreatedAt == nil {
		return nil, fmt.Errorf("attempted to map missing resource(%s) as a child resource", node.FullName())
	}

	parentUid := getParentResourceUid(node)
	if parentUid == "" {
		return nil, fmt.Errorf("attempted to map non-child resource(%s) as a child resource", node.FullName())
	}

	appResource := &pb.ChildApplicationResource{
		ResourceUid:         node.UID,
		ParentResourceUid:   parentUid,
		RootResourceUid:     getRootParentResourceUid(resourceMap, parentUid),
		Name:                node.Name,
		KubernetesNamespace: node.Namespace,
		Group:               node.Group,
		Version:             node.Version,
		Kind:                node.Kind,
	}

	healthStatus := getHealthStatus(node)
	if healthStatus != nil {
		appResource.ResourceStatusCode = healthStatus.resourceStatusCode
		appResource.ResourceStatusMessage = healthStatus.resourceStatusMessage
	}

	return appResource, nil
}

func getParentResourceUid(node v1alpha1.ResourceNode) string {
	for _, parent := range node.ParentRefs {
		if parent.UID != "" {
			return parent.UID
		}
	}

	return ""
}

func getRootParentResourceUid(resourceMap map[string]v1alpha1.ResourceNode, initialParentId string) string {
	currentId := initialParentId
	visited := make(map[string]bool)

	for currentId != "" {
		// If we've seen this ID before, we have a circular reference
		if visited[currentId] {
			// Return the starting parent as fallback in case of cycle
			return initialParentId
		}
		visited[currentId] = true

		// Look up the current resource
		currentResource, exists := resourceMap[currentId]
		if !exists {
			// Resource not found, current ID is the root
			return currentId
		}

		// Get the parent of the current resource
		parentUid := getParentResourceUid(currentResource)
		if parentUid == "" {
			// No parent, current resource is the root
			return currentId
		}

		currentId = parentUid
	}

	return initialParentId
}

type applicationResourceHealthStatus struct {
	resourceStatusCode    string
	resourceStatusMessage string
}

func getHealthStatus(node v1alpha1.ResourceNode) *applicationResourceHealthStatus {
	if node.Health == nil {
		return nil
	}

	return &applicationResourceHealthStatus{
		resourceStatusCode:    string(node.Health.Status),
		resourceStatusMessage: node.Health.Message,
	}
}

func getSyncStatus(node v1alpha1.ResourceNode, application *v1alpha1.Application) (string, bool) {
	appResource, ok := application.Status.FindResource(
		kube.NewResourceKey(node.Group, node.Kind, node.Namespace, node.Name),
	)

	if !ok {
		return "", false
	}

	switch appResource.Status {
	case v1alpha1.SyncStatusCodeSynced:
		// The Octopus representation of SyncStatusCodeSynced is "InSync" instead of Synced
		return "InSync", true
	default:
		return string(appResource.Status), true
	}
}
