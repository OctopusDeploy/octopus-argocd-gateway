package argocd

import (
	"strings"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/samber/lo"
)

const OctopusAnnotationsPrefix = "argo.octopus.com/"

// InjectAnnotations injects Octopus prefixed annotations from the source object
func InjectAnnotations(application *v1alpha1.Application, annotations map[string]string) *v1alpha1.Application {
	octopusAnnotations := lo.PickBy(annotations, func(key string, value string) bool {
		return strings.HasPrefix(key, OctopusAnnotationsPrefix)
	})
	// We want the application annotations to win in the event of a conflict
	application.Annotations = lo.Assign(octopusAnnotations, application.Annotations)
	return application
}
