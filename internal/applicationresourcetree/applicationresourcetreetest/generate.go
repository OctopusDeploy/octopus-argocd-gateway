package applicationresourcetreetest

//go:generate mockgen -package=applicationresourcetreetest -destination=application_service_mock.go -source=../application_resource_tree_handler.go applicationService
//go:generate go run github.com/OctopusDeploy/octopus-argocd-gateway/cmd/mockbuilder -source=../application_resource_tree_handler.go -interface=applicationService -destination=builder.go -package=applicationresourcetreetest
