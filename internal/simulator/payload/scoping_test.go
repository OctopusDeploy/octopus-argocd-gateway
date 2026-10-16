package payload

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadScopingPool_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pool.json")
	written := ScopingPool{
		ManagedPercent: 70,
		Projects:       []string{"p1", "p2"},
		Environments:   []string{"e1"},
	}
	b, err := json.Marshal(written)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, b, 0o600))

	loaded, err := LoadScopingPool(path)
	require.NoError(t, err)
	assert.Equal(t, written, *loaded)
}

func TestScopingPool_ValidateRejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		p    ScopingPool
	}{
		{"managed_percent>100", ScopingPool{ManagedPercent: 101, Projects: []string{"p"}, Environments: []string{"e"}}},
		{"managed_percent<0", ScopingPool{ManagedPercent: -1, Projects: []string{"p"}, Environments: []string{"e"}}},
		{"managed but no projects", ScopingPool{ManagedPercent: 50, Environments: []string{"e"}}},
		{"managed but no envs", ScopingPool{ManagedPercent: 50, Projects: []string{"p"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Error(t, c.p.Validate())
		})
	}
}

func TestScopingPool_ZeroPercentValid(t *testing.T) {
	// managed_percent=0 disables scoping; no project/env required.
	p := ScopingPool{ManagedPercent: 0}
	assert.NoError(t, p.Validate())
	_, _, ok := p.scopeFor(123)
	assert.False(t, ok)
}

func TestScopingPool_NilSafe(t *testing.T) {
	var p *ScopingPool
	_, _, ok := p.scopeFor(7)
	assert.False(t, ok)
}

func TestBuildApplication_StampsAnnotationsWhenManaged(t *testing.T) {
	pool := &ScopingPool{
		ManagedPercent: 100,
		Projects:       []string{"my-project"},
		Environments:   []string{"my-env"},
	}
	app := BuildApplication(42, 5, pool)
	require.NotNil(t, app.ObjectMeta.Annotations)
	assert.Equal(t, "my-project", app.ObjectMeta.Annotations[ProjectAnnotationKey])
	assert.Equal(t, "my-env", app.ObjectMeta.Annotations[EnvironmentAnnotationKey])
}

func TestBuildApplication_NoAnnotationsWhenUnmanaged(t *testing.T) {
	pool := &ScopingPool{ManagedPercent: 0}
	app := BuildApplication(42, 5, pool)
	assert.Nil(t, app.ObjectMeta.Annotations)
}

func TestBuildApplication_NoAnnotationsWhenScopeNil(t *testing.T) {
	app := BuildApplication(42, 5, nil)
	assert.Nil(t, app.ObjectMeta.Annotations)
}

func TestScopingPool_DistributesAcrossPools(t *testing.T) {
	// With several projects/envs and 100% managed, every seed must land on
	// some entry in each pool, and over a sample we should see at least 2 of
	// each (otherwise the mixing function has collapsed onto one bucket).
	pool := &ScopingPool{
		ManagedPercent: 100,
		Projects:       []string{"p0", "p1", "p2", "p3"},
		Environments:   []string{"e0", "e1", "e2"},
	}
	seenProjects := map[string]struct{}{}
	seenEnvs := map[string]struct{}{}
	for seed := uint64(0); seed < 200; seed++ {
		project, env, ok := pool.scopeFor(seed)
		require.True(t, ok, "seed %d should be managed at 100%%", seed)
		seenProjects[project] = struct{}{}
		seenEnvs[env] = struct{}{}
	}
	assert.Len(t, seenProjects, len(pool.Projects), "all projects should appear across the seed range")
	assert.Len(t, seenEnvs, len(pool.Environments), "all environments should appear across the seed range")
}

func TestScopingPool_Deterministic(t *testing.T) {
	// Same seed must always map to the same scope — the server's per-(gateway, UID)
	// upsert keys depend on this stability.
	pool := &ScopingPool{
		ManagedPercent: 70,
		Projects:       []string{"p0", "p1", "p2"},
		Environments:   []string{"e0", "e1"},
	}
	for seed := uint64(0); seed < 50; seed++ {
		p1, e1, ok1 := pool.scopeFor(seed)
		p2, e2, ok2 := pool.scopeFor(seed)
		assert.Equal(t, ok1, ok2)
		assert.Equal(t, p1, p2)
		assert.Equal(t, e1, e2)
	}
}

func TestScopingPool_RoughlyHonoursManagedPercent(t *testing.T) {
	pool := &ScopingPool{
		ManagedPercent: 30,
		Projects:       []string{"p"},
		Environments:   []string{"e"},
	}
	const samples = 10000
	managed := 0
	for seed := uint64(0); seed < samples; seed++ {
		if _, _, ok := pool.scopeFor(seed); ok {
			managed++
		}
	}
	// Allow generous tolerance — splitMix64 is a good mixer but small windows
	// still drift; we only need to confirm the configured percent is honoured
	// directionally, not statistically.
	ratio := float64(managed) / float64(samples)
	assert.InDelta(t, 0.30, ratio, 0.05, "managed ratio should be near 30%, got %0.3f", ratio)
}

func TestBuildEventJSON_AnnotationsRoundTrip(t *testing.T) {
	pool := &ScopingPool{
		ManagedPercent: 100,
		Projects:       []string{"my-proj"},
		Environments:   []string{"my-env"},
	}
	js, err := BuildEventJSON(7, 3, pool)
	require.NoError(t, err)

	var evt v1alpha1.ApplicationWatchEvent
	require.NoError(t, json.Unmarshal([]byte(js), &evt))
	assert.Equal(t, "my-proj", evt.Application.ObjectMeta.Annotations[ProjectAnnotationKey])
	assert.Equal(t, "my-env", evt.Application.ObjectMeta.Annotations[EnvironmentAnnotationKey])
}
