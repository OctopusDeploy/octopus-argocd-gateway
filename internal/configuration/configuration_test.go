package configuration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/yaml"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type failingConfigWriter struct{}

func (f *failingConfigWriter) WriteConfig() error {
	return os.ErrPermission
}

func (f *failingConfigWriter) SafeWriteConfig() error {
	return nil
}

func copyTestDataToTempDir(t *testing.T) string {
	tempDirPath := t.TempDir()
	testDataPath := os.DirFS("../../testdata")
	err := os.CopyFS(tempDirPath, testDataPath)
	if err != nil {
		t.Fatalf("Failed to copy test data to temp dir")
	}
	return tempDirPath
}

func TestGetConfiguration_ReadsConfigurationValuesSuccessfully(t *testing.T) {
	tempDirPath := copyTestDataToTempDir(t)

	configManager := NewConfigManager()

	// Need to bind the environment variable to viper otherwise it won't be read
	configManager.AddBoolFlag(pflag.CommandLine, "debug", false, "")
	t.Setenv("OCTOPUS_DEBUG", "true")

	// set the test data path
	configManager.config.AddConfigPath(tempDirPath)
	configManager.secrets.AddConfigPath(tempDirPath)

	config := configManager.GetConfiguration()

	if !config.Debug {
		t.Fatalf("Expected debug to be true, got %v", config.Debug)
	}

	if config.Gateway.ArgoCdAuthToken != "IAmNotARealToken" {
		t.Fatalf("Expected ArgoCdAuthToken to be 'IAmNotARealToken', got %v", config.Gateway.ArgoCdAuthToken)
	}

	if config.Gateway.ArgoCdServerUrl != "localhost:30857" {
		t.Fatalf("Expected ArgoCdServerUrl to be 'localhost:30857', got %v", config.Gateway.ArgoCdServerUrl)
	}

	// A give-up duration that failed to decode would read as 0, which the health
	// check silently replaces with the same 5m default.
	if config.Gateway.OctopusHealthCheckGiveUpAfter != 5*time.Minute {
		t.Fatalf(
			"Expected OctopusHealthCheckGiveUpAfter to be 5m, got %v",
			config.Gateway.OctopusHealthCheckGiveUpAfter,
		)
	}
}

func TestGetConfiguration_ProjectAuthTokens_SingleEnvVarPopulatesMap(t *testing.T) {
	t.Setenv("OCTOPUS_ARGOCD_PROJECT_AUTH_TOKEN_MYPROJ", "secret")

	configManager := NewConfigManager()
	configManager.config.AddConfigPath(t.TempDir())
	configManager.secrets.AddConfigPath(t.TempDir())

	config := configManager.GetConfiguration()

	got, ok := config.Gateway.ArgoCdProjectAuthTokens["MYPROJ"]
	if !ok {
		t.Fatalf(
			"Expected ArgoCdProjectAuthTokens to contain key 'MYPROJ', got %#v",
			config.Gateway.ArgoCdProjectAuthTokens,
		)
	}
	if got != "secret" {
		t.Fatalf("Expected ArgoCdProjectAuthTokens[MYPROJ] = 'secret', got %q", got)
	}
}

func TestGetConfiguration_ProjectAuthTokens_MultipleEnvVarsBuildMap(t *testing.T) {
	t.Setenv("OCTOPUS_ARGOCD_PROJECT_AUTH_TOKEN_ALPHA", "t1")
	t.Setenv("OCTOPUS_ARGOCD_PROJECT_AUTH_TOKEN_BETA", "t2")

	configManager := NewConfigManager()
	configManager.config.AddConfigPath(t.TempDir())
	configManager.secrets.AddConfigPath(t.TempDir())

	config := configManager.GetConfiguration()

	want := map[string]string{"ALPHA": "t1", "BETA": "t2"}
	got := config.Gateway.ArgoCdProjectAuthTokens
	if len(got) != len(want) {
		t.Fatalf("Expected %d entries, got %d: %#v", len(want), len(got), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("Expected ArgoCdProjectAuthTokens[%q] = %q, got %q", k, v, got[k])
		}
	}
}

func TestGetConfiguration_ProjectAuthTokens_EnvOverridesFile_FileOnlyEntrySurvives(t *testing.T) {
	tempDirPath := t.TempDir()
	configYaml := "argocd-project-auth-tokens:\n" +
		"  shared: from-file\n" +
		"  fileonly: file-token\n"
	if err := os.WriteFile(tempDirPath+"/octopus-argocd-gateway.yaml", []byte(configYaml), 0o600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	t.Setenv("OCTOPUS_ARGOCD_PROJECT_AUTH_TOKEN_shared", "from-env")

	configManager := NewConfigManager()
	configManager.config.AddConfigPath(tempDirPath)
	configManager.secrets.AddConfigPath(tempDirPath)

	config := configManager.GetConfiguration()
	got := config.Gateway.ArgoCdProjectAuthTokens

	if got["shared"] != "from-env" {
		t.Fatalf("Expected env to win on 'shared', got %q (full map: %#v)", got["shared"], got)
	}
	if got["fileonly"] != "file-token" {
		t.Fatalf(
			"Expected file-only 'fileonly' = 'file-token' to survive, got %q (full map: %#v)",
			got["fileonly"],
			got,
		)
	}
}

func TestGetConfiguration_ProjectAuthTokens_PrefixStrippedCasePreserved(t *testing.T) {
	t.Setenv("OCTOPUS_ARGOCD_PROJECT_AUTH_TOKEN_MixedCase", "mc")
	t.Setenv("OCTOPUS_ARGOCD_PROJECT_AUTH_TOKEN_lower", "lo")
	t.Setenv("OCTOPUS_ARGOCD_PROJECT_AUTH_TOKEN_UPPER", "up")

	configManager := NewConfigManager()
	configManager.config.AddConfigPath(t.TempDir())
	configManager.secrets.AddConfigPath(t.TempDir())

	config := configManager.GetConfiguration()
	got := config.Gateway.ArgoCdProjectAuthTokens

	want := map[string]string{"MixedCase": "mc", "lower": "lo", "UPPER": "up"}
	if len(got) != len(want) {
		t.Fatalf("Expected %d entries, got %d: %#v", len(want), len(got), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("Expected ArgoCdProjectAuthTokens[%q] = %q, got %q (full map: %#v)", k, v, got[k], got)
		}
	}
}

func TestGetConfiguration_ProjectAuthTokens_UnrelatedEnvVarsIgnored(t *testing.T) {
	t.Setenv("OCTOPUS_ARGOCD_PROJECT_AUTH_TOKEN_KEEP", "yes")
	t.Setenv("OCTOPUS_ARGOCD_PROJECT_AUTH_TOKENS", "not-a-prefix-match")
	t.Setenv("OCTOPUS_ARGOCD_AUTH_TOKEN", "other-token")
	t.Setenv("UNRELATED_PROJECT_AUTH_TOKEN_NOPE", "no")

	configManager := NewConfigManager()
	configManager.config.AddConfigPath(t.TempDir())
	configManager.secrets.AddConfigPath(t.TempDir())

	config := configManager.GetConfiguration()
	got := config.Gateway.ArgoCdProjectAuthTokens

	if len(got) != 1 || got["KEEP"] != "yes" {
		t.Fatalf("Expected only {KEEP: yes}, got %#v", got)
	}
}

func TestSetPostRegistrationConfig_UpdatesConfigAndWrites(t *testing.T) {
	tempDirPath := copyTestDataToTempDir(t)

	// factory
	getConfig := func() *Configuration {
		configManager := NewConfigManager()

		// set the test data path
		configManager.config.AddConfigPath(tempDirPath)
		configManager.secrets.AddConfigPath(tempDirPath)

		return configManager.GetConfiguration()
	}

	// arrange
	config := getConfig()

	initialClientId := config.Gateway.OctopusGrpcClientId
	initialAuthToken := config.Gateway.OctopusGrpcAuthenticationToken
	initialThumbprint := config.Gateway.OctopusGrpcThumbprint

	// act
	newClientId := "NEW_CLIENT_ID"
	newAuthToken := "NEW_AUTH_TOKEN"
	newThumbprint := "NEW_THUMBPRINT"
	err := config.writePostRegistrationConfigToFile(
		newClientId,
		newAuthToken,
		newThumbprint,
		map[string]string{"Spaces-1": "ArgoCDGateways-1"},
	)
	if err != nil {
		t.Error(err)
	}

	if initialClientId == config.Gateway.OctopusGrpcClientId {
		t.Fatalf("Expected gRPC client id to have changed, but received %v", config.Gateway.OctopusGrpcClientId)
	}

	if initialAuthToken == config.Gateway.OctopusGrpcAuthenticationToken {
		t.Fatalf(
			"Expected gRPC authentication token to have changed, but received %v",
			config.Gateway.OctopusGrpcAuthenticationToken,
		)
	}

	if initialThumbprint == config.Gateway.OctopusGrpcThumbprint {
		t.Fatalf(
			"Expected server thumbprint to have changed, but received %v",
			config.Gateway.OctopusGrpcThumbprint,
		)
	}

	if config.Gateway.OctopusGrpcClientId != newClientId {
		t.Fatalf("Expected gRPC client id to be new client id, but received %v", config.Gateway.OctopusGrpcClientId)
	}

	if config.Gateway.OctopusGrpcAuthenticationToken != newAuthToken {
		t.Fatalf(
			"Expected gRPC authentication token to be new auth token, but received %v",
			config.Gateway.OctopusGrpcAuthenticationToken,
		)
	}

	if config.Gateway.OctopusGrpcThumbprint != newThumbprint {
		t.Fatalf(
			"Expected server thumbprint to be new auth token, but received %v",
			config.Gateway.OctopusGrpcThumbprint,
		)
	}

	// create a new config manager and re-read the config and we should get the updated values
	config = getConfig()

	if config.Gateway.OctopusGrpcClientId != newClientId {
		t.Fatalf(
			"Expected gRPC client id to be new client id after reloading, but received %v",
			config.Gateway.OctopusGrpcClientId,
		)
	}

	if config.Gateway.OctopusGrpcAuthenticationToken != newAuthToken {
		t.Fatalf(
			"Expected gRPC authentication token to be new auth token after reloading, but received %v",
			config.Gateway.OctopusGrpcAuthenticationToken,
		)
	}

	if config.Gateway.OctopusGrpcThumbprint != newThumbprint {
		t.Fatalf(
			"Expected server thumbprint to be new auth token after reloading, but received %v",
			config.Gateway.OctopusGrpcThumbprint,
		)
	}
}

func TestWriteConfigWithFallback_CreatesConfigWhenMissing(t *testing.T) {
	tempDirPath := t.TempDir()

	viperInstance := viper.New()
	viperInstance.AddConfigPath(tempDirPath)
	viperInstance.SetConfigName("test-config")
	viperInstance.SetConfigType("yaml")
	viperInstance.Set("key", "value")

	err := writeConfigWithFallback(viperInstance)
	if err != nil {
		t.Fatalf("Expected fallback write to succeed, got error: %v", err)
	}

	if _, statErr := os.Stat(tempDirPath + "/test-config.yaml"); statErr != nil {
		t.Fatalf("Expected fallback write to create config file, got error: %v", statErr)
	}
}

func TestWriteConfigWithFallback_ReturnsOriginalErrorWhenNotConfigNotFound(t *testing.T) {
	err := writeConfigWithFallback(&failingConfigWriter{})
	if err == nil {
		t.Fatalf("Expected non-config-not-found error to be returned")
	}
}

func TestWritePostRegistrationConfigToCluster_UpdatesAndWrites(t *testing.T) {
	// factory
	getConfig := func() *Configuration {
		configManager := NewConfigManager()

		return configManager.GetConfiguration()
	}

	// arrange
	config := getConfig()

	// act
	newClientId := "NEW_CLIENT_ID"
	newAuthToken := "NEW_AUTH_TOKEN"
	newThumbprint := "NEW_THUMBPRINT"
	kubernetesClient := fake.NewClientset([]runtime.Object{
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.Registration.SecretName,
				Namespace: config.Registration.Namespace,
			},
			Data: map[string][]byte{},
		},
	}...)
	err := config.writePostRegistrationConfigToCluster(
		context.Background(),
		newClientId,
		newAuthToken,
		newThumbprint,
		map[string]string{"Spaces-1": "ArgoCDGateways-1", "Spaces-2": "ArgoCDGateways-2"},
		kubernetesClient,
	)
	if err != nil {
		t.Error(err)
	}

	updatedSecret, _ := kubernetesClient.CoreV1().
		Secrets(config.Registration.Namespace).
		Get(context.Background(), config.Registration.SecretName, metav1.GetOptions{})
	updatedSecretContent := updatedSecret.StringData["octopus-argocd-gateway-octopus-authentication-secret.yaml"]

	if updatedSecretContent == "" {
		t.Fatalf("Expected secret to have data, but received %v", updatedSecretContent)
	}

	if !strings.Contains(updatedSecretContent, "Spaces-1=ArgoCDGateways-1,Spaces-2=ArgoCDGateways-2") {
		t.Fatalf(
			"Expected registered spaces in secret data, but received %v",
			updatedSecretContent,
		)
	}

	if !strings.Contains(updatedSecretContent, "NEW_CLIENT_ID") {
		t.Fatalf(
			"Expected new client id in secret data, but received %v",
			updatedSecretContent,
		)
	}

	if !strings.Contains(updatedSecretContent, "NEW_AUTH_TOKEN") {
		t.Fatalf(
			"Expected new client id in secret data, but received %v",
			updatedSecretContent,
		)
	}

	if !strings.Contains(updatedSecretContent, "NEW_THUMBPRINT") {
		t.Fatalf(
			"Expected new thumbprint in secret data, but received %v",
			updatedSecretContent,
		)
	}
}

func TestWritePostRegistrationConfigToCluster_UpdatesAndWrites_PreservesExistingThumbprintValue(t *testing.T) {
	// factory
	getConfig := func() *Configuration {
		configManager := NewConfigManager()

		return configManager.GetConfiguration()
	}

	// arrange
	config := getConfig()

	// act
	newThumbprint := "NEW_THUMBPRINT"

	existingConfigYaml := map[string]string{
		certificateThumbprintKey: "OLD_THUMBPRINT",
	}

	existingYamlBytes, _ := yaml.Marshal(existingConfigYaml)
	existingData := map[string][]byte{
		gatewayOctopusAuthenticationSecretConfigurationFileName: existingYamlBytes,
	}

	kubernetesClient := fake.NewClientset([]runtime.Object{
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.Registration.SecretName,
				Namespace: config.Registration.Namespace,
			},
			Data: existingData,
		},
	}...)
	err := config.writePostRegistrationConfigToCluster(
		context.Background(),
		"",
		"",
		newThumbprint,
		nil,
		kubernetesClient,
	)
	if err != nil {
		t.Error(err)
	}

	updatedSecret, _ := kubernetesClient.CoreV1().
		Secrets(config.Registration.Namespace).
		Get(context.Background(), config.Registration.SecretName, metav1.GetOptions{})
	updatedSecretContent := updatedSecret.StringData["octopus-argocd-gateway-octopus-authentication-secret.yaml"]

	if updatedSecretContent == "" {
		t.Fatalf("Expected secret to have data, but received %v", updatedSecretContent)
	}

	if strings.Contains(updatedSecretContent, "NEW_THUMBPRINT") {
		t.Fatalf(
			"Expected old thumbprint in secret data, but received %v",
			updatedSecretContent,
		)
	}
}

func TestGetRegistrationStateFromCluster_ReturnsStoredState(t *testing.T) {
	config := NewConfigManager().GetConfiguration()

	existingConfigYaml := map[string]string{
		clientIdKey:              "CLIENT_ID",
		authenticationTokenKey:   "AUTH_TOKEN",
		certificateThumbprintKey: "THUMBPRINT",
		registeredSpacesKey:      "Spaces-1=ArgoCDGateways-1,Spaces-2=ArgoCDGateways-2",
	}
	existingYamlBytes, _ := yaml.Marshal(existingConfigYaml)

	kubernetesClient := fake.NewClientset([]runtime.Object{
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.Registration.SecretName,
				Namespace: config.Registration.Namespace,
			},
			Data: map[string][]byte{
				gatewayOctopusAuthenticationSecretConfigurationFileName: existingYamlBytes,
			},
		},
	}...)

	state, err := config.getRegistrationStateFromCluster(context.Background(), kubernetesClient)
	if err != nil {
		t.Fatal(err)
	}

	if state == nil {
		t.Fatal("Expected registration state, but received nil")
	}

	if state.ClientId != "CLIENT_ID" || state.AuthenticationToken != "AUTH_TOKEN" ||
		state.CertificateThumbprint != "THUMBPRINT" {
		t.Fatalf("Unexpected registration state: %+v", state)
	}

	if state.RegisteredSpaces["Spaces-1"] != "ArgoCDGateways-1" ||
		state.RegisteredSpaces["Spaces-2"] != "ArgoCDGateways-2" {
		t.Fatalf("Unexpected registered spaces: %v", state.RegisteredSpaces)
	}
}

func TestGetRegistrationStateFromCluster_ReturnsNilWhenNotRegistered(t *testing.T) {
	config := NewConfigManager().GetConfiguration()

	kubernetesClient := fake.NewClientset()

	state, err := config.getRegistrationStateFromCluster(context.Background(), kubernetesClient)
	if err != nil {
		t.Fatal(err)
	}

	if state != nil {
		t.Fatalf("Expected nil registration state, but received %+v", state)
	}
}
