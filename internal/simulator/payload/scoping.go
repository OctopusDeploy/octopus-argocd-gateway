package payload

import (
	"encoding/json"
	"fmt"
	"os"
)

// Annotation keys consumed by Octopus Server's ScopingAnnotationReader to
// resolve an ArgoCD Application onto a (project, environment) pair. Without
// these, the server-side desired-resource flow bails out before creating an
// ApplicationInstance — so simulator payloads must carry them whenever we
// want to exercise the real handler path.
const (
	ProjectAnnotationKey     = "argo.octopus.com/project"
	EnvironmentAnnotationKey = "argo.octopus.com/environment"
)

// ScopingPool drives how simulator-generated applications are stamped with
// Octopus scoping annotations. Loaded from disk by the test runner.
//
// ManagedPercent is the proportion of distinct application UIDs (i.e. seeds
// in the payload pool) that should be marked as Octopus-managed by gaining
// project + environment annotations. The remainder are emitted unannotated to
// represent ArgoCD applications that are *not* managed by Octopus.
type ScopingPool struct {
	ManagedPercent int      `json:"managed_percent"`
	Projects       []string `json:"projects"`
	Environments   []string `json:"environments"`
}

func LoadScopingPool(path string) (*ScopingPool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scoping pool %s: %w", path, err)
	}
	var p ScopingPool
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse scoping pool %s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("scoping pool %s: %w", path, err)
	}
	return &p, nil
}

func (p *ScopingPool) Validate() error {
	if p.ManagedPercent < 0 || p.ManagedPercent > 100 {
		return fmt.Errorf("managed_percent must be in [0,100], got %d", p.ManagedPercent)
	}
	if p.ManagedPercent > 0 {
		if len(p.Projects) == 0 {
			return fmt.Errorf("managed_percent>0 requires at least one project")
		}
		if len(p.Environments) == 0 {
			return fmt.Errorf("managed_percent>0 requires at least one environment")
		}
	}
	return nil
}

// scopeFor returns the (project, environment) slugs for a given application
// seed, or ok=false when this seed should be left unannotated. The mapping is
// deterministic so the same UID always lands on the same scope across replays
// — this keeps the server's per-(gateway, UID) UPSERT path stable.
func (p *ScopingPool) scopeFor(seed uint64) (project string, environment string, ok bool) {
	if p == nil || p.ManagedPercent <= 0 {
		return "", "", false
	}
	// splitMix64 spreads adjacent seeds across the full 64-bit space so that
	// our small modular partitions don't collapse onto the same bucket.
	if int(splitMix64(seed)%100) >= p.ManagedPercent {
		return "", "", false
	}
	projectIdx := int(splitMix64(seed^projectMixSalt) % uint64(len(p.Projects)))
	envIdx := int(splitMix64(seed^environmentMixSalt) % uint64(len(p.Environments)))
	return p.Projects[projectIdx], p.Environments[envIdx], true
}

// Distinct salts so the project and environment buckets are independently
// distributed (otherwise every seed that hits project[0] would also hit env[0]).
const (
	projectMixSalt     uint64 = 0xA5A5A5A5A5A5A5A5
	environmentMixSalt uint64 = 0x5A5A5A5A5A5A5A5A
)
