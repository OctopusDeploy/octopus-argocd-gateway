package leaderelection

import (
	"os"
	"testing"
)

func TestResolveLockName(t *testing.T) {
	tests := []struct {
		name        string
		releaseName string
		unset       bool
		want        string
	}{
		{
			name:        "derives lock name from release name",
			releaseName: "gateway-one",
			want:        "gateway-one-leader",
		},
		{
			name:        "derives distinct lock names for a different release",
			releaseName: "gateway-two",
			want:        "gateway-two-leader",
		},
		{
			name:  "falls back to the historical fixed name when unset",
			unset: true,
			want:  defaultLockName,
		},
		{
			name:        "falls back to the historical fixed name when empty",
			releaseName: "",
			want:        defaultLockName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, wasSet := os.LookupEnv("HELM_RELEASE_NAME")
			t.Cleanup(func() {
				if wasSet {
					os.Setenv("HELM_RELEASE_NAME", original)
				} else {
					os.Unsetenv("HELM_RELEASE_NAME")
				}
			})

			if tt.unset {
				os.Unsetenv("HELM_RELEASE_NAME")
			} else {
				os.Setenv("HELM_RELEASE_NAME", tt.releaseName)
			}

			if got := resolveLockName(); got != tt.want {
				t.Errorf("resolveLockName() = %q, want %q", got, tt.want)
			}
		})
	}
}
