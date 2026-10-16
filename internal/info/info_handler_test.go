package info

import (
	"errors"
	"testing"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/version"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/argocd"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/configuration"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/testutilities"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/utilities"
)

type mockVersionService struct {
	getVersionFunc func() (*version.VersionMessage, error)
}

func (m *mockVersionService) GetVersion() (*version.VersionMessage, error) {
	return m.getVersionFunc()
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func newJWT(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return signed
}

func newHandler(
	t *testing.T,
	clock utilities.Clock,
	globalToken string,
	projectTokens map[string]string,
	versionSvc argocd.VersionService,
) *Handler {
	t.Helper()
	return &Handler{
		logger:         testutilities.CreateTestLogger(),
		ctx:            t.Context(),
		versionService: versionSvc,
		config: &configuration.Configuration{
			Gateway: configuration.GatewayConfig{
				ArgoCdAuthToken:         globalToken,
				ArgoCdProjectAuthTokens: projectTokens,
			},
		},
		clock: clock,
	}
}

func TestHandler_ProcessRequest_PopulatesResponseFromArgoVersion(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	validToken := newJWT(t, jwt.MapClaims{"exp": clock.now.Add(30 * 24 * time.Hour).Unix()})

	versionSvc := &mockVersionService{
		getVersionFunc: func() (*version.VersionMessage, error) {
			return &version.VersionMessage{Version: "v2.10.0"}, nil
		},
	}
	h := newHandler(t, clock, validToken, nil, versionSvc)

	resp, err := h.ProcessRequest(&pb.GetInfoRequest{RequestId: "req-1"})

	require.NoError(t, err)
	assert.Equal(t, "req-1", resp.RequestId)
	assert.True(t, resp.CanConnectToArgoCd)
	assert.Equal(t, "", resp.ArgoCdConnectionError)
	assert.Equal(t, "v2.10.0", resp.ArgoCdVersion)
}

func TestHandler_ProcessRequest_ReportsArgoConnectionError(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	validToken := newJWT(t, jwt.MapClaims{"exp": clock.now.Add(30 * 24 * time.Hour).Unix()})

	versionSvc := &mockVersionService{
		getVersionFunc: func() (*version.VersionMessage, error) {
			return nil, errors.New("dial tcp: i/o timeout")
		},
	}
	h := newHandler(t, clock, validToken, nil, versionSvc)

	resp, err := h.ProcessRequest(&pb.GetInfoRequest{RequestId: "req-1"})

	require.NoError(
		t,
		err,
		"ProcessRequest should not propagate the connection error — it's surfaced via response fields",
	)
	assert.False(t, resp.CanConnectToArgoCd)
	assert.Contains(t, resp.ArgoCdConnectionError, "i/o timeout")
	assert.Equal(t, "", resp.ArgoCdVersion)
}

func TestHandler_ProcessRequest_RunsValidation_GlobalNearExpiry(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	// 3-day expiry is inside the 7-day warning threshold.
	nearExpiry := newJWT(t, jwt.MapClaims{"exp": clock.now.Add(3 * 24 * time.Hour).Unix()})

	versionSvc := &mockVersionService{
		getVersionFunc: func() (*version.VersionMessage, error) {
			return &version.VersionMessage{Version: "v2.10.0"}, nil
		},
	}
	h := newHandler(t, clock, nearExpiry, nil, versionSvc)

	resp, err := h.ProcessRequest(&pb.GetInfoRequest{RequestId: "req-1"})

	require.NoError(t, err)
	assert.True(t, resp.CanConnectToArgoCd)
	// Validation runs but issues are emitted via the logger only; the
	// response shape is unchanged. This confirms the validation path
	// neither short-circuits the request nor mutates the response.
}

func TestHandler_ProcessRequest_RunsValidation_InvalidGlobalToken(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}

	versionSvc := &mockVersionService{
		getVersionFunc: func() (*version.VersionMessage, error) {
			return &version.VersionMessage{Version: "v2.10.0"}, nil
		},
	}
	h := newHandler(t, clock, "not-a-jwt", nil, versionSvc)

	resp, err := h.ProcessRequest(&pb.GetInfoRequest{RequestId: "req-1"})

	require.NoError(t, err, "invalid auth token must not block the info response")
	assert.True(t, resp.CanConnectToArgoCd, "Argo CD connectivity is independent of token validation")
}

func TestHandler_ProcessRequest_RunsValidation_ProjectTokens(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	healthy := newJWT(t, jwt.MapClaims{"exp": clock.now.Add(30 * 24 * time.Hour).Unix()})
	nearExpiry := newJWT(t, jwt.MapClaims{"exp": clock.now.Add(2 * 24 * time.Hour).Unix()})
	expired := newJWT(t, jwt.MapClaims{"exp": clock.now.Add(-1 * time.Hour).Unix()})

	versionSvc := &mockVersionService{
		getVersionFunc: func() (*version.VersionMessage, error) {
			return &version.VersionMessage{Version: "v2.10.0"}, nil
		},
	}
	h := newHandler(t, clock, "", map[string]string{
		"team-a": healthy,
		"team-b": nearExpiry,
		"team-c": expired,
	}, versionSvc)

	resp, err := h.ProcessRequest(&pb.GetInfoRequest{RequestId: "req-1"})

	require.NoError(t, err)
	assert.True(t, resp.CanConnectToArgoCd)
}

func TestHandler_ProcessRequest_HandlesNilArgoVersionMessage(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	validToken := newJWT(t, jwt.MapClaims{"exp": clock.now.Add(30 * 24 * time.Hour).Unix()})

	// Defensive: GetVersion returns (nil, nil) — handler must not panic.
	versionSvc := &mockVersionService{
		getVersionFunc: func() (*version.VersionMessage, error) {
			return nil, nil
		},
	}
	h := newHandler(t, clock, validToken, nil, versionSvc)

	resp, err := h.ProcessRequest(&pb.GetInfoRequest{RequestId: "req-1"})

	require.NoError(t, err)
	assert.True(t, resp.CanConnectToArgoCd)
	assert.Equal(t, "", resp.ArgoCdVersion)
}
