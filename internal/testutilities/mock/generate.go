package mock

// These interfaces are defined in external packages or generated files that we don't control,
// so mock generation directives need to be maintained separately here.

//go:generate mockgen -package=mock -destination=application_event_service_mock.go github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos ApplicationEventServiceClient
//go:generate mockgen -package=mock -destination=argo_application_service_mock.go github.com/argoproj/argo-cd/v3/pkg/apiclient/application ApplicationService_WatchResourceTreeClient
