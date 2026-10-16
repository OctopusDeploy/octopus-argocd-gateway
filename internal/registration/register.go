package registration

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/google/uuid"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/configuration"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopusdeploy"
)

func Run(ctx context.Context, config *configuration.Configuration, logger *slog.Logger) error {
	configuredSpaces := config.Registration.ConfiguredSpaceIds()
	if len(configuredSpaces) == 0 {
		return fmt.Errorf("at least one space id must be configured via space-id or space-ids")
	}

	state, err := config.GetRegistrationState(ctx)
	if err != nil {
		return err
	}

	freshInstallation := state == nil
	if freshInstallation {
		state, err = registerPrimarySpace(ctx, config, configuredSpaces[0], logger)
		if err != nil {
			return err
		}
	} else if len(state.RegisteredSpaces) == 0 {
		// secrets written before space tracking only ever registered the primary space
		state.RegisteredSpaces[configuredSpaces[0]] = ""
	}

	missingSpaces := SpacesToRegister(config.Registration, state.RegisteredSpaces)
	if len(missingSpaces) == 0 {
		if !freshInstallation {
			logger.Info("ArgoCD Gateway is already registered with Octopus Server in all configured spaces, skipping")
		}
		return nil
	}

	// In case we bail out mid-registration, we should try and register the remaining spaces that are still missing
	for _, spaceId := range missingSpaces {
		response, err := registerWithOctopusServer(&config.Registration, state.ClientId, spaceId, true)
		if err != nil {
			logger.Error(
				"Failed to register ArgoCD Gateway in additional space",
				slog.String("spaceId", spaceId),
				slog.Any("error", err),
			)
			return err
		}

		state.RegisteredSpaces[spaceId] = response.Resource.Id
		logger.Info(
			"Registered ArgoCD Gateway in additional space",
			slog.String("id", response.Resource.Id),
			slog.String("spaceId", spaceId),
		)

		// Save here rather than at the end in case we bail out mid-registration
		if err := savePostRegistrationConfig(ctx, config, state, logger); err != nil {
			return err
		}
	}

	return nil
}

func registerPrimarySpace(
	ctx context.Context, config *configuration.Configuration, primarySpaceId string, logger *slog.Logger,
) (*configuration.RegistrationState, error) {
	clientId := uuid.New().String()

	response, err := registerWithOctopusServer(&config.Registration, clientId, primarySpaceId, false)
	if err != nil {
		logger.Error("Failed to register ArgoCD Gateway with Octopus server", slog.Any("error", err))
		return nil, err
	}

	logger.Info(
		"Registered ArgoCD Gateway with Octopus server",
		slog.String("id", response.Resource.Id),
		slog.String("name", response.Resource.Name),
		slog.String("spaceId", primarySpaceId),
	)

	state := &configuration.RegistrationState{
		ClientId:              response.Resource.ClientId,
		AuthenticationToken:   response.AuthenticationToken,
		CertificateThumbprint: response.CertificateThumbprint,
		RegisteredSpaces:      map[string]string{primarySpaceId: response.Resource.Id},
	}

	if err := savePostRegistrationConfig(ctx, config, state, logger); err != nil {
		return nil, err
	}

	return state, nil
}

// SpacesToRegister returns the configured spaces that don't yet have a registration, in configured order.
func SpacesToRegister(config configuration.RegistrationConfig, registeredSpaces map[string]string) []string {
	var missing []string

	for _, spaceId := range config.ConfiguredSpaceIds() {
		if _, registered := registeredSpaces[spaceId]; !registered {
			missing = append(missing, spaceId)
		}
	}

	return missing
}

func savePostRegistrationConfig(
	ctx context.Context, config *configuration.Configuration, state *configuration.RegistrationState,
	logger *slog.Logger,
) error {
	err := config.SetPostRegistrationConfig(
		ctx,
		state.ClientId,
		state.AuthenticationToken,
		state.CertificateThumbprint,
		state.RegisteredSpaces,
	)
	if err != nil {
		logger.Error("Failed to save registration information", slog.Any("error", err))
		return err
	}

	logger.Info("Saved registration configuration")
	return nil
}

func registerWithOctopusServer(
	config *configuration.RegistrationConfig,
	clientId string,
	spaceId string,
	preserveAuthenticationToken bool,
) (*octopusdeploy.RegisterArgoCDGatewayResponse, error) {
	apiUrl, err := url.Parse(config.ServerUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Octopus server URL: %w", err)
	}

	if config.ServerAccessToken == "" {
		return nil, fmt.Errorf("server access token is required for registration")
	}

	octopusClient, err := octopusdeploy.NewOctopusApiClient(apiUrl, config.ServerAccessToken, spaceId)
	if err != nil {
		return nil, fmt.Errorf("failed to create Octopus API client: %w", err)
	}

	return octopusClient.RegisterGateway(
		clientId,
		config.GatewayName,
		config.ArgoCDWebUIUrl,
		config.Environments,
		config.Tenants,
		preserveAuthenticationToken,
	)
}
