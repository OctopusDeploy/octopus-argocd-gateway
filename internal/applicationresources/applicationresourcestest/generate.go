package applicationresourcestest

//go:generate mockgen -package=applicationresourcestest -destination=application_service_mock.go -source=../application_resource_watcher.go applicationService
//go:generate mockgen -package=applicationresourcestest -destination=watch_resource_tree_client_mock.go github.com/argoproj/argo-cd/v3/pkg/apiclient/application ApplicationService_WatchResourceTreeClient
//go:generate go run github.com/OctopusDeploy/octopus-argocd-gateway/cmd/mockbuilder -source=../application_resource_watcher.go -interface=applicationService -destination=builder.go -package=applicationresourcestest
