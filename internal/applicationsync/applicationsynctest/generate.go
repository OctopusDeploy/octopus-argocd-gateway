package applicationsynctest

//go:generate mockgen -package=applicationsynctest -destination=application_service_mock.go -source=../application_sync_handler.go applicationService
//go:generate go run github.com/OctopusDeploy/octopus-argocd-gateway/cmd/mockbuilder -source=../application_sync_handler.go -interface=applicationService -destination=builder.go -package=applicationsynctest
