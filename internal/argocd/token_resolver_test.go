package argocd

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- globalTokenResolver --------------------------------------------------

func TestGlobalTokenResolver_ReturnsConfiguredToken(t *testing.T) {
	resolver := NewGlobalTokenResolver("global-token")

	token, err := resolver.Resolve(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "global-token", token)
}

func TestGlobalTokenResolver_IgnoresProjectInContext(t *testing.T) {
	resolver := NewGlobalTokenResolver("global-token")
	ctx := WithArgoProject(context.Background(), "team-a")

	token, err := resolver.Resolve(ctx)

	require.NoError(t, err)
	assert.Equal(t, "global-token", token, "global resolver must not vary by project")
}

func TestGlobalTokenResolver_ReturnsEmptyStringWhenConfiguredEmpty(t *testing.T) {
	// Constructing a global resolver with an empty token isn't an error here —
	// validation lives in NewClient. The resolver just returns what it was given.
	resolver := NewGlobalTokenResolver("")

	token, err := resolver.Resolve(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "", token)
}

// --- projectTokenResolver -------------------------------------------------

func TestProjectTokenResolver_ReturnsMatchingProjectToken(t *testing.T) {
	resolver := NewProjectTokenResolver(map[string]string{
		"team-a": "token-a",
		"team-b": "token-b",
	})

	for _, tc := range []struct {
		project string
		want    string
	}{
		{"team-a", "token-a"},
		{"team-b", "token-b"},
	} {
		t.Run(tc.project, func(t *testing.T) {
			ctx := WithArgoProject(context.Background(), tc.project)

			token, err := resolver.Resolve(ctx)

			require.NoError(t, err)
			assert.Equal(t, tc.want, token)
		})
	}
}

func TestProjectTokenResolver_ReturnsErrorWhenProjectNotConfigured(t *testing.T) {
	resolver := NewProjectTokenResolver(map[string]string{
		"team-a": "token-a",
	})
	ctx := WithArgoProject(context.Background(), "team-missing")

	token, err := resolver.Resolve(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `"team-missing"`,
		"error should name the missing project so operators can diagnose it")
	assert.Empty(t, token)
}

func TestProjectTokenResolver_NoProjectInContext_ReturnsSentinelError(t *testing.T) {
	resolver := NewProjectTokenResolver(map[string]string{
		"team-a": "token-a",
		"team-b": "token-b",
	})

	token, err := resolver.Resolve(context.Background())

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoProjectInContext),
		"caller code uses errors.Is on the sentinel; got %v", err)
	assert.Empty(t, token)
}

func TestProjectTokenResolver_UnscopedSentinel_PicksAnyTokenWhenSentinelMissingFromMap(t *testing.T) {
	// UnscopedProjectAuthToken is used by call sites (e.g. VersionService) that
	// cannot meaningfully scope to a project. When the map has no entry under
	// that sentinel, the resolver must return *some* configured token rather
	// than failing the call.
	resolver := NewProjectTokenResolver(map[string]string{
		"team-a": "token-a",
	})
	ctx := WithArgoProject(context.Background(), UnscopedProjectAuthToken)

	token, err := resolver.Resolve(ctx)

	require.NoError(t, err)
	assert.Equal(t, "token-a", token)
}

func TestProjectTokenResolver_UnscopedSentinel_PrefersExplicitEntryInMap(t *testing.T) {
	// If callers want to dedicate a specific token to unscoped calls they can
	// add it under the sentinel key. The explicit entry must win over the
	// "pick any" fallback.
	resolver := NewProjectTokenResolver(map[string]string{
		"team-a":                 "token-a",
		UnscopedProjectAuthToken: "explicit-unscoped",
	})
	ctx := WithArgoProject(context.Background(), UnscopedProjectAuthToken)

	token, err := resolver.Resolve(ctx)

	require.NoError(t, err)
	assert.Equal(t, "explicit-unscoped", token)
}

func TestProjectTokenResolver_UnscopedSentinel_EmptyMapReturnsError(t *testing.T) {
	resolver := NewProjectTokenResolver(map[string]string{})
	ctx := WithArgoProject(context.Background(), UnscopedProjectAuthToken)

	token, err := resolver.Resolve(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), UnscopedProjectAuthToken)
	assert.Empty(t, token)
}

func TestProjectTokenResolver_EmptyProjectNameInContext(t *testing.T) {
	// An empty string project name is a real value (hasProject == true) and is
	// not the unscoped sentinel — it should not match any configured project
	// and must surface a clear error rather than silently returning a token.
	resolver := NewProjectTokenResolver(map[string]string{
		"team-a": "token-a",
	})
	ctx := WithArgoProject(context.Background(), "")

	token, err := resolver.Resolve(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `""`)
	assert.Empty(t, token)
}

// --- context helpers ------------------------------------------------------

func TestArgoProjectFromContext_RoundTripsProjectName(t *testing.T) {
	ctx := WithArgoProject(context.Background(), "team-a")

	project, ok := ArgoProjectFromContext(ctx)

	assert.True(t, ok)
	assert.Equal(t, "team-a", project)
}

func TestArgoProjectFromContext_NoValueReturnsFalse(t *testing.T) {
	project, ok := ArgoProjectFromContext(context.Background())

	assert.False(t, ok)
	assert.Empty(t, project)
}

func TestArgoProjectFromContext_ChildContextInheritsValue(t *testing.T) {
	parent := WithArgoProject(context.Background(), "team-a")
	child, cancel := context.WithCancel(parent)
	defer cancel()

	project, ok := ArgoProjectFromContext(child)

	assert.True(t, ok)
	assert.Equal(t, "team-a", project)
}
