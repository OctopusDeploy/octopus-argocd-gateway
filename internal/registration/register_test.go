package registration

import (
	"testing"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/configuration"
)

func TestSpacesToRegister(t *testing.T) {
	tests := []struct {
		name             string
		config           configuration.RegistrationConfig
		registeredSpaces map[string]string
		want             []string
	}{
		{
			name: "nothing registered returns space-id then space-ids",
			config: configuration.RegistrationConfig{
				SpaceId: "Spaces-1", SpaceIds: []string{"Spaces-2", "Spaces-3"},
			},
			registeredSpaces: map[string]string{},
			want:             []string{"Spaces-1", "Spaces-2", "Spaces-3"},
		},
		{
			name: "already registered spaces are skipped",
			config: configuration.RegistrationConfig{
				SpaceId: "Spaces-1", SpaceIds: []string{"Spaces-2", "Spaces-3"},
			},
			registeredSpaces: map[string]string{"Spaces-1": "ArgoCDGateways-1", "Spaces-2": "ArgoCDGateways-2"},
			want:             []string{"Spaces-3"},
		},
		{
			name:             "registered with unknown gateway id still counts as registered",
			config:           configuration.RegistrationConfig{SpaceId: "Spaces-1"},
			registeredSpaces: map[string]string{"Spaces-1": ""},
			want:             nil,
		},
		{
			name: "duplicate spaces are ignored",
			config: configuration.RegistrationConfig{
				SpaceId: "Spaces-1", SpaceIds: []string{"Spaces-1", "Spaces-2", "Spaces-2"},
			},
			registeredSpaces: map[string]string{},
			want:             []string{"Spaces-1", "Spaces-2"},
		},
		{
			name: "space-ids works without space-id",
			config: configuration.RegistrationConfig{
				SpaceIds: []string{"Spaces-2", "Spaces-1"},
			},
			registeredSpaces: map[string]string{},
			want:             []string{"Spaces-2", "Spaces-1"},
		},
		{
			name: "all spaces registered returns nothing",
			config: configuration.RegistrationConfig{
				SpaceId: "Spaces-1", SpaceIds: []string{"Spaces-2"},
			},
			registeredSpaces: map[string]string{"Spaces-1": "ArgoCDGateways-1", "Spaces-2": "ArgoCDGateways-2"},
			want:             nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SpacesToRegister(tt.config, tt.registeredSpaces)
			if len(got) != len(tt.want) {
				t.Fatalf("SpacesToRegister() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("SpacesToRegister() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestRegisteredSpacesRoundTrip(t *testing.T) {
	registeredSpaces := map[string]string{
		"Spaces-1":  "ArgoCDGateways-1",
		"Spaces-2":  "ArgoCDGateways-42",
		"Spaces-10": "",
	}

	formatted := configuration.FormatRegisteredSpaces(registeredSpaces)
	if formatted != "Spaces-10=,Spaces-1=ArgoCDGateways-1,Spaces-2=ArgoCDGateways-42" {
		t.Fatalf("FormatRegisteredSpaces() = %q", formatted)
	}

	parsed := configuration.ParseRegisteredSpaces(formatted)
	if len(parsed) != len(registeredSpaces) {
		t.Fatalf("ParseRegisteredSpaces() = %v, want %v", parsed, registeredSpaces)
	}
	for spaceId, gatewayId := range registeredSpaces {
		if parsed[spaceId] != gatewayId {
			t.Fatalf("ParseRegisteredSpaces()[%s] = %q, want %q", spaceId, parsed[spaceId], gatewayId)
		}
	}
}

func TestParseRegisteredSpacesEmpty(t *testing.T) {
	if got := configuration.ParseRegisteredSpaces(""); len(got) != 0 {
		t.Fatalf("ParseRegisteredSpaces(\"\") = %v, want empty", got)
	}
}
