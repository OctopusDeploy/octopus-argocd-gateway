package payload

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strconv"
	"sync/atomic"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"k8s.io/apimachinery/pkg/watch"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

type SizeClass string

const (
	SizeSmall  SizeClass = "small"
	SizeMedium SizeClass = "medium"
	SizeLarge  SizeClass = "large"
)

func (s SizeClass) ResourceCount() (int, error) {
	switch s {
	case SizeSmall:
		return 5, nil
	case SizeMedium:
		return 20, nil
	case SizeLarge:
		return 100, nil
	default:
		return 0, fmt.Errorf("unknown size class %q (want small|medium|large)", s)
	}
}

type kindTemplate struct {
	group   string
	version string
	kind    string
}

var resourceKinds = []kindTemplate{
	{"apps", "v1", "Deployment"},
	{"apps", "v1", "Deployment"},
	{"apps", "v1", "ReplicaSet"},
	{"apps", "v1", "StatefulSet"},
	{"", "v1", "Service"},
	{"", "v1", "Service"},
	{"", "v1", "ConfigMap"},
	{"", "v1", "ConfigMap"},
	{"", "v1", "Secret"},
	{"", "v1", "Pod"},
	{"", "v1", "Pod"},
	{"", "v1", "Pod"},
	{"networking.k8s.io", "v1", "Ingress"},
	{"batch", "v1", "Job"},
	{"batch", "v1", "CronJob"},
}

func BuildApplication(seed uint64, resourceCount int, scope *ScopingPool) v1alpha1.Application {
	r := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))

	appName := fmt.Sprintf("sim-app-%d", seed)
	namespace := "argocd"
	destNamespace := fmt.Sprintf("sim-ns-%d", seed%16)

	resources := make([]v1alpha1.ResourceStatus, resourceCount)
	for i := range resources {
		k := resourceKinds[r.IntN(len(resourceKinds))]
		resources[i] = v1alpha1.ResourceStatus{
			Group:     k.group,
			Version:   k.version,
			Kind:      k.kind,
			Namespace: destNamespace,
			Name:      fmt.Sprintf("%s-%s-%s", appName, lowerKind(k.kind), strconv.Itoa(i)),
			Status:    v1alpha1.SyncStatusCodeSynced,
			Health: &v1alpha1.HealthStatus{
				Status: "Healthy",
			},
		}
	}

	var annotations map[string]string
	if project, environment, ok := scope.scopeFor(seed); ok {
		annotations = map[string]string{
			ProjectAnnotationKey:     project,
			EnvironmentAnnotationKey: environment,
		}
	}

	return v1alpha1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        appName,
			Namespace:   namespace,
			UID:         syntheticUID(seed),
			Annotations: annotations,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "octopus-argocd-gateway-simulator",
				"simulator.octopus/seed":       strconv.FormatUint(seed, 10),
			},
		},
		Spec: v1alpha1.ApplicationSpec{
			Project: "default",
			Source: &v1alpha1.ApplicationSource{
				RepoURL:        "https://example.com/simulator/repo.git",
				Path:           "manifests",
				TargetRevision: "HEAD",
			},
			Destination: v1alpha1.ApplicationDestination{
				Server:    "https://kubernetes.default.svc",
				Namespace: destNamespace,
			},
		},
		Status: v1alpha1.ApplicationStatus{
			Resources: resources,
			Sync: v1alpha1.SyncStatus{
				Status:   v1alpha1.SyncStatusCodeSynced,
				Revision: fmt.Sprintf("rev-%016x", seed),
			},
			Health: v1alpha1.HealthStatus{
				Status: "Healthy",
			},
		},
	}
}

func BuildEventJSON(seed uint64, resourceCount int, scope *ScopingPool) (string, error) {
	app := BuildApplication(seed, resourceCount, scope)
	event := v1alpha1.ApplicationWatchEvent{
		Type:        watch.Modified,
		Application: app,
	}
	b, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("marshal application watch event: %w", err)
	}
	return string(b), nil
}

const DefaultPoolSize = 256

// Cache holds pre-marshalled payloads keyed by size class. Each entry within a
// class uses a distinct seed so the application UID/name differs per entry —
// this spreads load across many simulated apps and avoids triggering the
// server's per-(gateway, UID) upsert race under sustained concurrency.
type Cache struct {
	pools  map[SizeClass][]string
	cursor map[SizeClass]*atomicCursor
}

type atomicCursor struct{ v atomic.Uint64 }

func NewCache(seed uint64, scope *ScopingPool, classes ...SizeClass) (*Cache, error) {
	return NewCacheWithPool(seed, DefaultPoolSize, scope, classes...)
}

func NewCacheWithPool(seed uint64, poolSize int, scope *ScopingPool, classes ...SizeClass) (*Cache, error) {
	if poolSize <= 0 {
		return nil, fmt.Errorf("poolSize must be > 0, got %d", poolSize)
	}
	if len(classes) == 0 {
		classes = []SizeClass{SizeSmall, SizeMedium, SizeLarge}
	}
	out := &Cache{
		pools:  make(map[SizeClass][]string, len(classes)),
		cursor: make(map[SizeClass]*atomicCursor, len(classes)),
	}
	for _, c := range classes {
		count, err := c.ResourceCount()
		if err != nil {
			return nil, err
		}
		pool := make([]string, poolSize)
		for i := range pool {
			js, err := BuildEventJSON(seed+uint64(i), count, scope)
			if err != nil {
				return nil, err
			}
			pool[i] = js
		}
		out.pools[c] = pool
		out.cursor[c] = &atomicCursor{}
	}
	return out, nil
}

func (c *Cache) Get(s SizeClass) (string, error) {
	pool, ok := c.pools[s]
	if !ok || len(pool) == 0 {
		return "", fmt.Errorf("payload class %q not cached", s)
	}
	cur := c.cursor[s]
	idx := cur.v.Add(1) - 1
	return pool[int(idx%uint64(len(pool)))], nil
}

func (c *Cache) PoolSize(s SizeClass) int {
	return len(c.pools[s])
}

func lowerKind(kind string) string {
	b := make([]byte, len(kind))
	for i := 0; i < len(kind); i++ {
		c := kind[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func syntheticUID(seed uint64) k8stypes.UID {
	mix := splitMix64(seed)
	return k8stypes.UID(fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		uint32(seed),
		uint16(seed>>32),
		uint16(seed>>48),
		uint16(mix),
		(mix>>16)&0xFFFFFFFFFFFF,
	))
}

func splitMix64(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}
