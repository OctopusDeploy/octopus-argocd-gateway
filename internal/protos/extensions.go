package contractsv1

import "github.com/OctopusDeploy/octopus-argocd-gateway/internal/utilities"

func (x *WatchApplicationQuery) QualifiedName() string {
	return utilities.QualifiedName(x.ApplicationName, x.ApplicationNamespace)
}

func (x *SyncApplicationRequest) QualifiedName() string {
	return utilities.QualifiedName(x.ApplicationName, x.ApplicationNamespace)
}

func (x *GetResourceEventsRequest) QualifiedName() string {
	return utilities.QualifiedName(x.ApplicationName, x.ApplicationNamespace)
}

func (x *GetResourceManifestsRequest) QualifiedName() string {
	return utilities.QualifiedName(x.ApplicationName, x.ApplicationNamespace)
}
