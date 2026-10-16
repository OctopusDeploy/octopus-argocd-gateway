package argocd

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/utilities"
)

// --- helpers --------------------------------------------------------------

func createTestToken(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("test-secret"))
	return tokenString
}

type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

func newMockClock(_ *gomock.Controller) utilities.Clock {
	return &mockClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

const nearExpiryThreshold = 7 * 24 * time.Hour

func findIssue(issues []TokenIssue, label string) *TokenIssue {
	for i := range issues {
		if issues[i].Label == label {
			return &issues[i]
		}
	}
	return nil
}

// capturedRecord captures both the slog.Record and the effective attrs
// (preAttrs from any WithAttrs chain merged with the record's own attrs).
// Using a shared *[]capturedRecord lets cloned handlers from WithAttrs
// append to the same backing slice.
type capturedRecord struct {
	record slog.Record
	attrs  []slog.Attr
}

type captureHandler struct {
	records  *[]capturedRecord
	preAttrs []slog.Attr
}

func newCaptureHandler() *captureHandler {
	return &captureHandler{records: &[]capturedRecord{}}
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := append([]slog.Attr{}, h.preAttrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	*h.records = append(*h.records, capturedRecord{record: r, attrs: attrs})
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := append([]slog.Attr{}, h.preAttrs...)
	merged = append(merged, attrs...)
	return &captureHandler{records: h.records, preAttrs: merged}
}

func (h *captureHandler) WithGroup(_ string) slog.Handler { return h }

func (h *captureHandler) findRecord(message string) *capturedRecord {
	for i := range *h.records {
		if (*h.records)[i].record.Message == message {
			return &(*h.records)[i]
		}
	}
	return nil
}

func attrValue(rec *capturedRecord, key string) any {
	for _, a := range rec.attrs {
		if a.Key == key {
			return a.Value.Any()
		}
	}
	return nil
}

// findStatusRecord finds the one "token status" record emitted for a given
// project. With the per-token logging shape there is exactly one such record
// per project per LogValidationReport call.
func (h *captureHandler) findStatusRecord(project string) *capturedRecord {
	const msg = "Argo CD authentication token status"
	for i := range *h.records {
		rec := &(*h.records)[i]
		if rec.record.Message != msg {
			continue
		}
		if attrValue(rec, "project") == project {
			return rec
		}
	}
	return nil
}

// --- ValidateAuthToken (single-token JWT parsing) ------------------------

func TestValidateAuthToken_ValidToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	expectedExpiry := clock.Now().Add(1 * time.Hour)
	token := createTestToken(jwt.MapClaims{
		"exp": expectedExpiry.Unix(),
		"sub": "test-user",
	})

	expiresAt, err := ValidateAuthToken(token, clock)
	assert.NoError(t, err)
	require.NotNil(t, expiresAt)
	assert.True(t, expiresAt.Equal(expectedExpiry), "expected %v, got %v", expectedExpiry, *expiresAt)
}

func TestValidateAuthToken_ExpiredToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	expectedExpiry := clock.Now().Add(-1 * time.Hour)
	token := createTestToken(jwt.MapClaims{
		"exp": expectedExpiry.Unix(),
		"sub": "test-user",
	})

	expiresAt, err := ValidateAuthToken(token, clock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth token is expired. It expired at")
	require.NotNil(t, expiresAt)
	assert.True(t, expiresAt.Equal(expectedExpiry))
}

func TestValidateAuthToken_EmptyToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	expiresAt, err := ValidateAuthToken("", clock)
	require.Error(t, err)
	assert.Equal(t, "an argo cd authentication has not been provided", err.Error())
	assert.Nil(t, expiresAt)
}

func TestValidateAuthToken_InvalidJWTFormat(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	expiresAt, err := ValidateAuthToken("not-a-valid-jwt", clock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse auth token")
	assert.Nil(t, expiresAt)
}

func TestValidateAuthToken_MalformedJWT(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	expiresAt, err := ValidateAuthToken("header.payload", clock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse auth token")
	assert.Nil(t, expiresAt)
}

func TestValidateAuthToken_MissingExpirationClaim(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	// ArgoCD tokens can be configured to last forever, so missing exp claim is valid
	token := createTestToken(jwt.MapClaims{
		"sub": "test-user",
	})

	expiresAt, err := ValidateAuthToken(token, clock)
	assert.NoError(t, err)
	assert.Nil(t, expiresAt)
}

func TestValidateAuthToken_TokenWithInvalidExpirationFormat(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	token := createTestToken(jwt.MapClaims{
		"exp": "not-a-timestamp",
		"sub": "test-user",
	})

	expiresAt, err := ValidateAuthToken(token, clock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expiration claim is unable to be parsed")
	assert.Nil(t, expiresAt)
}

// --- ValidateAuthTokens (multi-token report) -----------------------------

func TestValidateAuthTokens_GlobalOnly_Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	token := createTestToken(jwt.MapClaims{
		"exp": clock.Now().Add(30 * 24 * time.Hour).Unix(),
	})

	report := ValidateAuthTokens(token, nil, nearExpiryThreshold, clock)
	assert.Empty(t, report.Invalid)
	assert.Empty(t, report.NearExpiry)
}

func TestValidateAuthTokens_GlobalOnly_Expired(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	token := createTestToken(jwt.MapClaims{
		"exp": clock.Now().Add(-1 * time.Hour).Unix(),
	})

	report := ValidateAuthTokens(token, nil, nearExpiryThreshold, clock)
	require.Len(t, report.Invalid, 1)
	assert.Equal(t, GlobalTokenLabel, report.Invalid[0].Label)
	assert.ErrorContains(t, report.Invalid[0].Err, "auth token is expired")
	assert.Empty(t, report.NearExpiry, "expired token should not also appear in NearExpiry")
}

func TestValidateAuthTokens_GlobalOnly_NearExpiry(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	expiry := clock.Now().Add(3 * 24 * time.Hour) // within 7-day threshold
	token := createTestToken(jwt.MapClaims{
		"exp": expiry.Unix(),
	})

	report := ValidateAuthTokens(token, nil, nearExpiryThreshold, clock)
	assert.Empty(t, report.Invalid)
	require.Len(t, report.NearExpiry, 1)
	assert.Equal(t, GlobalTokenLabel, report.NearExpiry[0].Label)
	require.NotNil(t, report.NearExpiry[0].ExpiresAt)
	assert.True(t, report.NearExpiry[0].ExpiresAt.Equal(expiry))
}

func TestValidateAuthTokens_GlobalOnly_NoExpiryClaim(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	token := createTestToken(jwt.MapClaims{
		"sub": "test-user",
	})

	report := ValidateAuthTokens(token, nil, nearExpiryThreshold, clock)
	assert.Empty(t, report.Invalid)
	assert.Empty(t, report.NearExpiry)
}

func TestValidateAuthTokens_ProjectTokens_Mixed(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)

	validExpiry := clock.Now().Add(30 * 24 * time.Hour)
	nearExpiry := clock.Now().Add(2 * 24 * time.Hour)
	expiredAt := clock.Now().Add(-1 * time.Hour)

	projectTokens := map[string]string{
		"healthy":     createTestToken(jwt.MapClaims{"exp": validExpiry.Unix()}),
		"near-expiry": createTestToken(jwt.MapClaims{"exp": nearExpiry.Unix()}),
		"expired":     createTestToken(jwt.MapClaims{"exp": expiredAt.Unix()}),
		"malformed":   "not-a-jwt",
	}

	report := ValidateAuthTokens("", projectTokens, nearExpiryThreshold, clock)

	require.Len(t, report.Invalid, 2)
	expired := findIssue(report.Invalid, "expired")
	require.NotNil(t, expired)
	assert.ErrorContains(t, expired.Err, "auth token is expired")
	malformed := findIssue(report.Invalid, "malformed")
	require.NotNil(t, malformed)
	assert.ErrorContains(t, malformed.Err, "failed to parse auth token")

	require.Len(t, report.NearExpiry, 1)
	assert.Equal(t, "near-expiry", report.NearExpiry[0].Label)
	require.NotNil(t, report.NearExpiry[0].ExpiresAt)
	assert.True(t, report.NearExpiry[0].ExpiresAt.Equal(nearExpiry))
}

func TestValidateAuthTokens_GlobalAndProjectTokens(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)

	globalToken := createTestToken(jwt.MapClaims{"exp": clock.Now().Add(2 * 24 * time.Hour).Unix()})
	projectTokens := map[string]string{
		"team-a": createTestToken(jwt.MapClaims{"exp": clock.Now().Add(30 * 24 * time.Hour).Unix()}),
		"team-b": createTestToken(jwt.MapClaims{"exp": clock.Now().Add(-1 * time.Hour).Unix()}),
	}

	report := ValidateAuthTokens(globalToken, projectTokens, nearExpiryThreshold, clock)

	require.Len(t, report.NearExpiry, 1)
	assert.Equal(t, GlobalTokenLabel, report.NearExpiry[0].Label)

	require.Len(t, report.Invalid, 1)
	assert.Equal(t, "team-b", report.Invalid[0].Label)
}

func TestValidateAuthTokens_NoTokensConfigured(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)

	report := ValidateAuthTokens("", nil, nearExpiryThreshold, clock)

	require.Len(t, report.Invalid, 1)
	assert.Equal(t, GlobalTokenLabel, report.Invalid[0].Label)
	assert.ErrorContains(t, report.Invalid[0].Err, "an argo cd authentication has not been provided")
}

func TestValidateAuthTokens_ProjectTokensOnly_EmptyGlobalIgnored(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)

	projectTokens := map[string]string{
		"team-a": createTestToken(jwt.MapClaims{"exp": clock.Now().Add(30 * 24 * time.Hour).Unix()}),
	}

	report := ValidateAuthTokens("", projectTokens, nearExpiryThreshold, clock)
	assert.Empty(t, report.Invalid, "empty global token should be ignored when project tokens are configured")
	assert.Empty(t, report.NearExpiry)
}

func TestValidateAuthTokens_ExpiredWithinThresholdLandsInInvalidOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	// expired 1 hour ago — also within any positive near-expiry window
	token := createTestToken(jwt.MapClaims{"exp": clock.Now().Add(-1 * time.Hour).Unix()})

	report := ValidateAuthTokens(token, nil, nearExpiryThreshold, clock)
	require.Len(t, report.Invalid, 1)
	assert.Empty(t, report.NearExpiry)
}

// --- TokenValidationReport.Tokens (always-on status table) ---------------

func TestValidateAuthTokens_TokensSlice_PopulatedForEveryValidatedToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)

	healthyExp := clock.Now().Add(30 * 24 * time.Hour)
	nearExp := clock.Now().Add(2 * 24 * time.Hour)
	expiredExp := clock.Now().Add(-1 * time.Hour)

	projectTokens := map[string]string{
		"healthy":     createTestToken(jwt.MapClaims{"exp": healthyExp.Unix()}),
		"near-expiry": createTestToken(jwt.MapClaims{"exp": nearExp.Unix()}),
		"expired":     createTestToken(jwt.MapClaims{"exp": expiredExp.Unix()}),
		"forever":     createTestToken(jwt.MapClaims{"sub": "no-exp-claim"}),
	}

	report := ValidateAuthTokens("", projectTokens, nearExpiryThreshold, clock)

	require.Len(t, report.Tokens, len(projectTokens),
		"Tokens slice must contain every validated token, including healthy ones")

	healthy := findIssue(report.Tokens, "healthy")
	require.NotNil(t, healthy)
	require.NotNil(t, healthy.ExpiresAt)
	assert.True(t, healthy.ExpiresAt.Equal(healthyExp))
	assert.NoError(t, healthy.Err)

	forever := findIssue(report.Tokens, "forever")
	require.NotNil(t, forever)
	assert.Nil(t, forever.ExpiresAt, "non-expiring tokens have nil ExpiresAt")
	assert.NoError(t, forever.Err)

	near := findIssue(report.Tokens, "near-expiry")
	require.NotNil(t, near)
	require.NotNil(t, near.ExpiresAt)
	assert.NoError(t, near.Err, "near-expiry tokens are not errored")

	expired := findIssue(report.Tokens, "expired")
	require.NotNil(t, expired)
	assert.ErrorContains(t, expired.Err, "auth token is expired",
		"expired tokens still appear in Tokens, with Err set")
}

func TestValidateAuthTokens_TokensSlice_IncludesGlobalAndProjects(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	globalToken := createTestToken(jwt.MapClaims{"exp": clock.Now().Add(10 * 24 * time.Hour).Unix()})
	projectTokens := map[string]string{
		"team-a": createTestToken(jwt.MapClaims{"exp": clock.Now().Add(10 * 24 * time.Hour).Unix()}),
	}

	report := ValidateAuthTokens(globalToken, projectTokens, nearExpiryThreshold, clock)

	require.Len(t, report.Tokens, 2)
	assert.NotNil(t, findIssue(report.Tokens, GlobalTokenLabel))
	assert.NotNil(t, findIssue(report.Tokens, "team-a"))
}

// --- LogValidationReport (table behaviour) -------------------------------

func TestLogValidationReport_EmitsOneRecordPerProject(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	projectTokens := map[string]string{
		"team-a": createTestToken(jwt.MapClaims{"exp": clock.Now().Add(30 * 24 * time.Hour).Unix()}),
		"team-b": createTestToken(jwt.MapClaims{"exp": clock.Now().Add(30 * 24 * time.Hour).Unix()}),
		"team-c": createTestToken(jwt.MapClaims{"exp": clock.Now().Add(30 * 24 * time.Hour).Unix()}),
	}

	report := ValidateAuthTokens("", projectTokens, nearExpiryThreshold, clock)

	handler := newCaptureHandler()
	LogValidationReport(slog.New(handler), report)

	// One status record per project; no Invalid/NearExpiry warnings expected.
	statusCount := 0
	for _, r := range *handler.records {
		if r.record.Message == "Argo CD authentication token status" {
			statusCount++
		}
	}
	assert.Equal(t, 3, statusCount, "one status log record must be emitted per project")
}

func TestLogValidationReport_StatusRecordIncludesProjectExpiryAndStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)
	healthyExp := clock.Now().Add(30 * 24 * time.Hour)
	projectTokens := map[string]string{
		"team-a": createTestToken(jwt.MapClaims{"exp": healthyExp.Unix()}),
	}

	report := ValidateAuthTokens("", projectTokens, nearExpiryThreshold, clock)
	require.Empty(t, report.Invalid, "precondition: only healthy tokens")
	require.Empty(t, report.NearExpiry)

	handler := newCaptureHandler()
	LogValidationReport(slog.New(handler), report)

	rec := handler.findStatusRecord("team-a")
	require.NotNil(t, rec, "status record must be emitted even when the token is healthy")
	assert.Equal(t, tokenStatusOK, attrValue(rec, "status"))
	assert.Equal(t, healthyExp.UTC().Format(time.RFC3339), attrValue(rec, "expires_at"))
	assert.Nil(t, attrValue(rec, "error"), "healthy tokens carry no error attr")
}

func TestLogValidationReport_StatusAttrReflectsTokenClassification(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := newMockClock(ctrl)

	projectTokens := map[string]string{
		"healthy":     createTestToken(jwt.MapClaims{"exp": clock.Now().Add(30 * 24 * time.Hour).Unix()}),
		"near-expiry": createTestToken(jwt.MapClaims{"exp": clock.Now().Add(2 * 24 * time.Hour).Unix()}),
		"expired":     createTestToken(jwt.MapClaims{"exp": clock.Now().Add(-1 * time.Hour).Unix()}),
		"forever":     createTestToken(jwt.MapClaims{"sub": "no-exp"}),
	}

	report := ValidateAuthTokens("", projectTokens, nearExpiryThreshold, clock)

	handler := newCaptureHandler()
	LogValidationReport(slog.New(handler), report)

	assert.Equal(t, tokenStatusOK, attrValue(handler.findStatusRecord("healthy"), "status"))
	assert.Equal(t, tokenStatusOK, attrValue(handler.findStatusRecord("forever"), "status"))
	assert.Equal(t, "never", attrValue(handler.findStatusRecord("forever"), "expires_at"),
		"non-expiring tokens render expires_at as 'never'")
	assert.Equal(t, tokenStatusNearExpiry, attrValue(handler.findStatusRecord("near-expiry"), "status"))

	expiredRec := handler.findStatusRecord("expired")
	require.NotNil(t, expiredRec)
	assert.Equal(t, tokenStatusInvalid, attrValue(expiredRec, "status"))
	assert.NotNil(t, attrValue(expiredRec, "error"),
		"invalid tokens carry an `error` attr on their status record")

	// Warnings still fire alongside the per-project status records.
	assert.NotNil(t, handler.findRecord("One or more Argo CD authentication tokens failed validation"))
	assert.NotNil(t, handler.findRecord("One or more Argo CD authentication tokens are close to expiry"))
}

func TestLogValidationReport_EmptyReport_EmitsNothing(t *testing.T) {
	handler := newCaptureHandler()
	LogValidationReport(slog.New(handler), TokenValidationReport{})

	assert.Empty(t, *handler.records,
		"with no validated tokens there is nothing to report — don't spam logs")
}
