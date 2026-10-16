package octopusdeploy

import (
	"fmt"
	"net/http"

	"github.com/OctopusDeploy/go-octopusdeploy/v2/pkg/core"
)

type RegisterArgoCDGatewayCommand struct {
	ClientId                    string
	Name                        string
	WebUIUri                    *string `json:"WebUIUri,omitempty"`
	Environments                []string
	Tenants                     []string
	PreserveAuthenticationToken *bool `json:"PreserveAuthenticationToken,omitempty"`
}

type RegisterArgoCDGatewayResponse struct {
	Resource              ArgoCDGatewayResource
	AuthenticationToken   string
	CertificateThumbprint string
}

type ArgoCDGatewayResource struct {
	Id             string
	Name           string
	WebUIUri       string
	ClientId       string
	EnvironmentIds []string
	TenantIds      []string
}

func (oc *OctopusClient) RegisterGateway(
	clientId string,
	name string,
	webUIUrl string,
	environments []string,
	tenants []string,
	preserveAuthenticationToken bool,
) (*RegisterArgoCDGatewayResponse, error) {
	// To avoid sending an empty string rather than omitting the value entirely from the json payload
	var webUIPtr *string
	if webUIUrl != "" {
		webUIPtr = &webUIUrl
	}

	var preserveTokenPtr *bool
	if preserveAuthenticationToken {
		preserveTokenPtr = &preserveAuthenticationToken
	}

	registerCommand := RegisterArgoCDGatewayCommand{
		ClientId:                    clientId,
		Name:                        name,
		WebUIUri:                    webUIPtr,
		Environments:                environments,
		Tenants:                     tenants,
		PreserveAuthenticationToken: preserveTokenPtr,
	}

	var response RegisterArgoCDGatewayResponse
	var octopusDeployError core.APIError

	path := fmt.Sprintf("%s/argocdgateways", oc.SpaceId)
	resp, err := oc.Client.
		Sling().
		New().
		Post(path).
		BodyJSON(registerCommand).
		Receive(&response, &octopusDeployError)

	apiErrorCheck := core.APIErrorChecker(path, resp, http.StatusOK, err, &octopusDeployError)
	if apiErrorCheck != nil {
		return nil, apiErrorCheck
	}

	// This indicates that the gateway wasn't unmarshalled properly
	// If there was actually an issue, we would have received a non-200 status code
	if response.Resource.Id == "" {
		return nil, ErrorGatewayIdMissing
	}

	return &response, nil
}
