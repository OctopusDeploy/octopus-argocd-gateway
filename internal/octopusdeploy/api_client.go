package octopusdeploy

import (
	"crypto/tls"
	"net/http"
	"net/url"

	"github.com/OctopusDeploy/go-octopusdeploy/v2/pkg/client"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/certificate"
)

type OctopusClient struct {
	Client  *client.Client
	SpaceId string
}

func NewOctopusApiClient(serverUrl *url.URL, accessToken string, spaceId string) (*OctopusClient, error) {
	credentials, err := NewOctopusCredential(accessToken)
	if err != nil {
		return nil, err
	}

	httpClient, err := getHttpClient()
	if err != nil {
		return nil, err
	}

	c, err := client.NewClientWithCredentials(httpClient, serverUrl, credentials, "", "OAG")
	if err != nil {
		return nil, err
	}

	return &OctopusClient{
		Client:  c,
		SpaceId: spaceId,
	}, nil
}

func NewOctopusCredential(credential string) (client.ICredential, error) {
	if client.IsAPIKey(credential) {
		return client.NewApiKey(credential)
	} else {
		return client.NewAccessToken(credential)
	}
}

func getHttpClient() (*http.Client, error) {
	rootCertificates, err := certificate.GetRootCertificatePool()
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{
		RootCAs: rootCertificates,
	}
	tr := &http.Transport{TLSClientConfig: tlsConfig}
	return &http.Client{Transport: tr}, nil
}
