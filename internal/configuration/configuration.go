package configuration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sigs.k8s.io/yaml"

	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sClient "k8s.io/client-go/kubernetes"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/kubernetes"
)

const (
	clientIdKey                                             = "octopus-grpc-client-id"
	authenticationTokenKey                                  = "octopus-grpc-authentication-token"
	certificateThumbprintKey                                = "octopus-grpc-thumbprint"
	registeredSpacesKey                                     = "octopus-registered-spaces"
	gatewayOctopusAuthenticationSecretConfigurationFileName = "octopus-argocd-gateway-octopus-authentication-secret.yaml"
)

type RegistrationState struct {
	ClientId              string
	AuthenticationToken   string
	CertificateThumbprint string
	// Space ID -> Gateway ID map. Used to track registration.
	RegisteredSpaces map[string]string
}

func FormatRegisteredSpaces(registeredSpaces map[string]string) string {
	entries := make([]string, 0, len(registeredSpaces))
	for spaceId, gatewayId := range registeredSpaces {
		entries = append(entries, fmt.Sprintf("%s=%s", spaceId, gatewayId))
	}
	sort.Strings(entries)
	return strings.Join(entries, ",")
}

func ParseRegisteredSpaces(value string) map[string]string {
	registeredSpaces := make(map[string]string)
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		spaceId, gatewayId, _ := strings.Cut(entry, "=")
		if spaceId != "" {
			registeredSpaces[spaceId] = gatewayId
		}
	}
	return registeredSpaces
}

type GatewayRuntimeEnvironment string

const (
	GatewayRuntimeInCluster GatewayRuntimeEnvironment = "InCluster"
	GatewayRuntimeExternal  GatewayRuntimeEnvironment = "External"
)

type GatewayConfig struct {
	// Argo Cd gRPC configuration
	ArgoCdAuthToken          string            `mapstructure:"argocd-auth-token"`
	ArgoCdProjectAuthTokens  map[string]string `mapstructure:"argocd-project-auth-tokens"`
	ArgoCdServerUrl          string            `mapstructure:"argocd-server-url"`
	ArgoCdServerTlsPlainText bool              `mapstructure:"argocd-server-plaintext"`
	ArgoCdServerTlsInsecure  bool              `mapstructure:"argocd-server-insecure"`
	ArgoCdGrpcWeb            bool              `mapstructure:"argocd-server-grpcweb"`
	ArgoCdGrpcWebRootPath    string            `mapstructure:"argocd-server-grpcweb-root-path"`

	// Server gRPC configuration
	OctopusGrpcUrl                 string        `mapstructure:"octopus-grpc-url"`
	OctopusGrpcThumbprint          string        `mapstructure:"octopus-grpc-thumbprint"`
	OctopusGrpcPinCertificate      bool          `mapstructure:"octopus-grpc-pin-certificate"`
	OctopusGrpcTlsPlaintext        bool          `mapstructure:"octopus-grpc-plaintext"`
	OctopusGrpcClientId            string        `mapstructure:"octopus-grpc-client-id"`
	OctopusGrpcAuthenticationToken string        `mapstructure:"octopus-grpc-authentication-token"`
	OctopusHealthCheckInterval     time.Duration `mapstructure:"octopus-health-check-interval"`
	OctopusHealthCheckGiveUpAfter  time.Duration `mapstructure:"octopus-health-check-give-up-after"`

	// Shared configuration
	CertificatePath                     string `mapstructure:"gateway-certificate-path"`
	StreamRetryBackoff                  int    `mapstructure:"stream-retry-backoff"`
	MetricsPort                         int    `mapstructure:"metrics-port"`
	PprofPort                           int    `mapstructure:"pprof-port"`
	ApplicationEventWorkers             int    `mapstructure:"application-event-workers"`
	ApplicationEventMaxRetries          int    `mapstructure:"application-event-max-retries"`
	ApplicationEventTombstoneTTLSeconds int    `mapstructure:"application-event-tombstone-ttl-seconds"`
	ApplicationEventDrainTimeoutSeconds int    `mapstructure:"application-event-drain-timeout-seconds"`
	ApplicationEventTimeoutSeconds      int    `mapstructure:"application-event-timeout-seconds"`
	ApplicationEventBufferSeconds       int    `mapstructure:"application-event-buffer-seconds"`
}

type Configuration struct {
	configManager *ConfigManager
	Debug         bool               `mapstructure:"debug"`
	Gateway       GatewayConfig      `mapstructure:",squash"`
	Registration  RegistrationConfig `mapstructure:",squash"`
}

type RegistrationConfig struct {
	ServerUrl         string   `mapstructure:"server-api-url"`
	ServerAccessToken string   `mapstructure:"server-access-token"`
	ArgoCDWebUIUrl    string   `mapstructure:"argocd-web-ui-url"`
	SpaceId           string   `mapstructure:"space-id"`
	SpaceIds          []string `mapstructure:"space-ids"`
	GatewayName       string   `mapstructure:"gateway-name"`
	Environments      []string `mapstructure:"environment"`
	Tenants           []string `mapstructure:"tenant"`
	SecretName        string   `mapstructure:"secret-name"`
	Namespace         string   `mapstructure:"namespace"`
}

// ConfiguredSpaceIds merges SpaceId and SpaceIds into a deduplicated list, SpaceId first.
func (c RegistrationConfig) ConfiguredSpaceIds() []string {
	var spaceIds []string
	seen := make(map[string]bool)

	for _, spaceId := range append([]string{c.SpaceId}, c.SpaceIds...) {
		if spaceId == "" || seen[spaceId] {
			continue
		}
		seen[spaceId] = true
		spaceIds = append(spaceIds, spaceId)
	}

	return spaceIds
}

type safeConfigWriter interface {
	WriteConfig() error
	SafeWriteConfig() error
}

func (m *ConfigManager) GetConfiguration() *Configuration {
	readConfiguration(m.config)
	readSecretConfiguration(m.secrets)

	config := &Configuration{
		configManager: m,
	}

	if err := m.config.Unmarshal(&config); err != nil {
		fmt.Printf("couldn't read config: %s\n", err)
		cobra.CheckErr(err)
	}

	if err := m.secrets.Unmarshal(&config); err != nil {
		fmt.Printf("couldn't read config: %s\n", err)
		cobra.CheckErr(err)
	}

	mergeProjectAuthTokensFromEnv(&config.Gateway, os.Environ())

	return config
}

const projectAuthTokenEnvPrefix = "OCTOPUS_ARGOCD_PROJECT_AUTH_TOKEN_"

func mergeProjectAuthTokensFromEnv(gateway *GatewayConfig, env []string) {
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if !strings.HasPrefix(name, projectAuthTokenEnvPrefix) {
			continue
		}
		project := strings.TrimPrefix(name, projectAuthTokenEnvPrefix)
		if project == "" {
			continue
		}
		if gateway.ArgoCdProjectAuthTokens == nil {
			gateway.ArgoCdProjectAuthTokens = map[string]string{}
		}
		gateway.ArgoCdProjectAuthTokens[project] = value
	}
}

func GetGatewayRuntimeEnvironment() GatewayRuntimeEnvironment {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return GatewayRuntimeInCluster
	}

	return GatewayRuntimeExternal
}

func (c *Configuration) SetPostRegistrationConfig(
	ctx context.Context,
	clientId string,
	authenticationToken string,
	certificateThumbprint string,
	registeredSpaces map[string]string,
) error {
	// Namespace has been passed in so we should try and write the post registration config to the cluster
	if isRunningInCluster() {
		k8sClientSet, clientErr := kubernetes.NewKubernetesClient()

		if clientErr != nil {
			return clientErr
		}

		return c.writePostRegistrationConfigToCluster(
			ctx,
			clientId,
			authenticationToken,
			certificateThumbprint,
			registeredSpaces,
			k8sClientSet,
		)
	}

	return c.writePostRegistrationConfigToFile(clientId, authenticationToken, certificateThumbprint, registeredSpaces)
}

// GetRegistrationState returns the stored registration state, or nil when the gateway is not registered.
func (c *Configuration) GetRegistrationState(ctx context.Context) (*RegistrationState, error) {
	if isRunningInCluster() {
		k8sClientSet, err := kubernetes.NewKubernetesClient()
		if err != nil {
			return nil, err
		}

		return c.getRegistrationStateFromCluster(ctx, k8sClientSet)
	}

	return c.getRegistrationStateFromFile(), nil
}

func (c *Configuration) getRegistrationStateFromCluster(
	ctx context.Context,
	kubernetesClient k8sClient.Interface,
) (*RegistrationState, error) {
	secret, err := getGatewaySecret(ctx, &c.Registration, kubernetesClient)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	existingYamlData := secret.Data[gatewayOctopusAuthenticationSecretConfigurationFileName]
	if len(existingYamlData) == 0 {
		return nil, nil
	}

	existingConfig := make(map[string]string)
	if err := yaml.Unmarshal(existingYamlData, &existingConfig); err != nil {
		return nil, err
	}

	return registrationStateFromValues(
		existingConfig[clientIdKey],
		existingConfig[authenticationTokenKey],
		existingConfig[certificateThumbprintKey],
		existingConfig[registeredSpacesKey],
	), nil
}

func (c *Configuration) getRegistrationStateFromFile() *RegistrationState {
	return registrationStateFromValues(
		c.configManager.secrets.GetString(clientIdKey),
		c.configManager.secrets.GetString(authenticationTokenKey),
		c.configManager.secrets.GetString(certificateThumbprintKey),
		c.configManager.secrets.GetString(registeredSpacesKey),
	)
}

func registrationStateFromValues(
	clientId, authenticationToken, certificateThumbprint, registeredSpaces string,
) *RegistrationState {
	if clientId == "" || authenticationToken == "" {
		return nil
	}

	return &RegistrationState{
		ClientId:              clientId,
		AuthenticationToken:   authenticationToken,
		CertificateThumbprint: certificateThumbprint,
		RegisteredSpaces:      ParseRegisteredSpaces(registeredSpaces),
	}
}

func (c *Configuration) writePostRegistrationConfigToFile(
	clientId string,
	authenticationToken string,
	certificateThumbprint string,
	registeredSpaces map[string]string,
) error {
	c.configManager.secrets.Set(clientIdKey, clientId)
	c.configManager.secrets.Set(authenticationTokenKey, authenticationToken)
	c.configManager.secrets.Set(registeredSpacesKey, FormatRegisteredSpaces(registeredSpaces))

	// Older versions of Octopus Server don't return the thumbprint in the registration response,
	// so to avoid override an existing thumbprint with an empty default string we need to check
	if certificateThumbprint != "" {
		c.configManager.secrets.Set(certificateThumbprintKey, certificateThumbprint)
		c.Gateway.OctopusGrpcThumbprint = certificateThumbprint
	}

	c.Gateway.OctopusGrpcClientId = clientId
	c.Gateway.OctopusGrpcAuthenticationToken = authenticationToken

	// TODO ensure only the config values are only written to their respective yaml file
	// Currently we end up with secret values in the octopus-argocd-gateway.yaml file when they should only be in the octopus-argocd-gateway-secret.yaml file
	if err := writeConfigWithFallback(c.configManager.config); err != nil {
		return err
	}

	if err := writeConfigWithFallback(c.configManager.secrets); err != nil {
		return err
	}

	return nil
}

func (c *Configuration) writePostRegistrationConfigToCluster(
	ctx context.Context,
	clientId string,
	authenticationToken string,
	certificateThumbprint string,
	registeredSpaces map[string]string,
	kubernetesClient k8sClient.Interface,
) error {
	secretUpdateErr := updateGatewaySecretConfiguration(
		ctx,
		clientId,
		authenticationToken,
		certificateThumbprint,
		registeredSpaces,
		&c.Registration,
		kubernetesClient,
	)

	if secretUpdateErr != nil {
		return secretUpdateErr
	}

	return nil
}

func writeConfigWithFallback(v safeConfigWriter) error {
	err := v.WriteConfig()
	if err == nil {
		return nil
	}

	var configFileNotFoundError viper.ConfigFileNotFoundError
	if errors.As(err, &configFileNotFoundError) {
		return v.SafeWriteConfig()
	}

	return err
}

func updateGatewaySecretConfiguration(
	ctx context.Context,
	clientId string,
	authenticationToken string,
	certificateThumbprint string,
	registeredSpaces map[string]string,
	config *RegistrationConfig,
	kubernetesClient k8sClient.Interface,
) error {
	secretsClient := kubernetesClient.CoreV1().Secrets(config.Namespace)
	gatewaySecret, secretGetError := getGatewaySecret(ctx, config, kubernetesClient)

	isNewSecret := false
	existingConfig := make(map[string]string)

	if secretGetError != nil {
		if !k8serrors.IsNotFound(secretGetError) {
			return secretGetError
		}
		isNewSecret = true
		gatewaySecret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.SecretName,
				Namespace: config.Namespace,
			},
			Type: v1.SecretTypeOpaque,
		}
	} else {
		// Older versions of Octopus Server don't return the thumbprint in the registration response,
		// so to avoid overriding an existing thumbprint we need to check
		existingYamlData := gatewaySecret.Data[gatewayOctopusAuthenticationSecretConfigurationFileName]
		if err := yaml.Unmarshal(existingYamlData, &existingConfig); err != nil {
			return err
		}
	}

	thumbprintToStore := ""
	if secretThumbprint, exists := existingConfig[certificateThumbprintKey]; exists && secretThumbprint != "" {
		thumbprintToStore = secretThumbprint
	} else if certificateThumbprint != "" {
		thumbprintToStore = certificateThumbprint
	}

	existingConfig[authenticationTokenKey] = authenticationToken
	existingConfig[clientIdKey] = clientId
	existingConfig[certificateThumbprintKey] = thumbprintToStore
	existingConfig[registeredSpacesKey] = FormatRegisteredSpaces(registeredSpaces)

	updatedYaml, err := yaml.Marshal(existingConfig)
	if err != nil {
		return err
	}

	gatewaySecret.StringData = map[string]string{
		gatewayOctopusAuthenticationSecretConfigurationFileName: string(updatedYaml),
	}

	if isNewSecret {
		_, err = secretsClient.Create(ctx, gatewaySecret, metav1.CreateOptions{})
	} else {
		_, err = secretsClient.Update(ctx, gatewaySecret, metav1.UpdateOptions{})
	}
	return err
}

func getGatewaySecret(
	ctx context.Context,
	config *RegistrationConfig,
	kubernetesClient k8sClient.Interface,
) (*v1.Secret, error) {
	existingSecret, err := kubernetesClient.CoreV1().
		Secrets(config.Namespace).
		Get(ctx, config.SecretName, metav1.GetOptions{})

	var statusErr *k8serrors.StatusError
	isStatusErr := errors.As(err, &statusErr)
	if isStatusErr && statusErr.ErrStatus.Reason == "NotFound" {
		return nil, statusErr
	}

	if err != nil {
		return nil, err
	}

	return existingSecret, nil
}

func readSecretConfiguration(secretViper *secretViper) {
	// Config files are stored in /octopus-argo-gateway when running in a cluster
	if isRunningInCluster() {
		secretViper.AddConfigPath("octopus-argo-gateway")
	} else {
		secretViper.AddConfigPath(".")
	}

	secretViper.SetConfigName("octopus-argocd-gateway-octopus-authentication-secret")
	secretViper.SetConfigType("yaml")
	readInSecretConfig(secretViper)
}

func readConfiguration(viperInstance *viper.Viper) {
	// Config files are stored in /octopus-argo-gateway when running in a cluster
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		viperInstance.AddConfigPath("octopus-argo-gateway")
	} else {
		viperInstance.AddConfigPath(".")
	}

	viperInstance.SetConfigName("octopus-argocd-gateway")
	viperInstance.SetConfigType("yaml")
	readInConfig(viperInstance)
}

func readInConfig(viperInstance *viper.Viper) {
	if err := viperInstance.MergeInConfig(); err != nil {
		fmt.Printf("Error reading config file: %v", err)
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Printf("Failed to find config file\n")
		} else {
			fmt.Printf("Failed to read config file correctly %v\n", err)
		}
	}
	fmt.Printf("Using config file: %v\n", viperInstance.ConfigFileUsed())
}

func readInSecretConfig(viperInstance *secretViper) {
	if err := viperInstance.MergeInConfig(); err != nil {
		fmt.Printf("Error reading secret config file: %v", err)
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error if desired
			fmt.Printf("Failed to find secret config file\n")
		} else {
			fmt.Printf("Failed to read secret config file correctly %v\n", err)
		}
	}
	fmt.Printf("Using secret config file: %v\n", viperInstance.ConfigFileUsed())
}

func (c *Configuration) IsGatewayRegistered(ctx context.Context) (bool, error) {
	if isRunningInCluster() {
		k8sClientSet, err := kubernetes.NewKubernetesClient()
		if err != nil {
			return false, err
		}

		return c.isGatewayRegisteredInCluster(ctx, k8sClientSet)
	}

	return c.isGatewayRegisteredInFile(), nil
}

func (c *Configuration) isGatewayRegisteredInCluster(
	ctx context.Context,
	kubernetesClient k8sClient.Interface,
) (bool, error) {
	secret, err := getGatewaySecret(ctx, &c.Registration, kubernetesClient)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	existingYamlData := secret.Data[gatewayOctopusAuthenticationSecretConfigurationFileName]
	if len(existingYamlData) == 0 {
		return false, nil
	}

	existingConfig := make(map[string]string)
	if err := yaml.Unmarshal(existingYamlData, &existingConfig); err != nil {
		return false, err
	}

	return existingConfig[clientIdKey] != "" && existingConfig[authenticationTokenKey] != "", nil
}

func (c *Configuration) isGatewayRegisteredInFile() bool {
	return c.configManager.secrets.GetString(clientIdKey) != "" &&
		c.configManager.secrets.GetString(authenticationTokenKey) != ""
}

func isRunningInCluster() bool {
	_, exists := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	return exists
}
