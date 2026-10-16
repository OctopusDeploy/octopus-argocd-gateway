package argocd

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoProjectInContext is returned by projectTokenResolver when an RPC is made
// without WithArgoProject and more than one project token is configured, so
// there is no unambiguous token to pick.
var ErrNoProjectInContext = errors.New("no Argo CD project in context for project-scoped token resolver")

// UnscopedProjectAuthToken is used when API calls to Argo CD cannot be scoped to a project e.g. calls to VersionService
// if not set in the configuration
var UnscopedProjectAuthToken = "octo-gateway-unscoped"

type TokenResolver interface {
	Resolve(ctx context.Context) (string, error)
}

type projectTokenResolver struct {
	tokenMap map[string]string
}

func NewProjectTokenResolver(tokenMap map[string]string) TokenResolver {
	return projectTokenResolver{tokenMap: tokenMap}
}

func (t projectTokenResolver) Resolve(ctx context.Context) (string, error) {
	if project, hasProject := ArgoProjectFromContext(ctx); hasProject {
		if token, ok := t.tokenMap[project]; ok {
			return token, nil
		}

		if project == UnscopedProjectAuthToken {
			for _, token := range t.tokenMap {
				return token, nil
			}
			return "", fmt.Errorf("no Argo CD auth token configured for project %q", project)
		}

		return "", fmt.Errorf("no Argo CD auth token configured for project %q", project)
	}

	return "", ErrNoProjectInContext
}

type globalTokenResolver struct {
	globalToken string
}

func NewGlobalTokenResolver(globalToken string) TokenResolver {
	return globalTokenResolver{globalToken: globalToken}
}

func (t globalTokenResolver) Resolve(ctx context.Context) (string, error) {
	return t.globalToken, nil
}

type argoProjectContextKey struct{}

func WithArgoProject(ctx context.Context, projectName string) context.Context {
	return context.WithValue(ctx, argoProjectContextKey{}, projectName)
}

func ArgoProjectFromContext(ctx context.Context) (string, bool) {
	project, ok := ctx.Value(argoProjectContextKey{}).(string)
	return project, ok
}
