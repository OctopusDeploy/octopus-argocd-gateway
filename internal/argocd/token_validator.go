package argocd

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/utilities"
)

// GlobalTokenLabel is the label used in TokenIssue.Label for the global
// (non-project-scoped) Argo CD auth token.
const GlobalTokenLabel = "global"

// DefaultTokenExpiryWarningThreshold is the default lead time used when warning
// that an Argo CD auth token is approaching its expiry.
const DefaultTokenExpiryWarningThreshold = 7 * 24 * time.Hour

const (
	tokenStatusOK         = "ok"
	tokenStatusNearExpiry = "near_expiry"
	tokenStatusInvalid    = "invalid"
)

type TokenIssue struct {
	Label     string
	ExpiresAt *time.Time
	Err       error
}

type TokenValidationReport struct {
	Tokens     []TokenIssue
	Invalid    []TokenIssue
	NearExpiry []TokenIssue
}

// ValidateAuthToken parses an Argo CD auth JWT and returns its expiration time
// alongside any validation error
func ValidateAuthToken(token string, clock utilities.Clock) (*time.Time, error) {
	if token == "" {
		return nil, errors.New("an argo cd authentication has not been provided")
	}

	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	parsedToken, _, err := parser.ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse auth token: %w", err)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("failed to parse JWT claims, token is potentially malformed")
	}

	exp, err := claims.GetExpirationTime()
	if err != nil {
		return nil, fmt.Errorf("expiration claim is unable to be parsed: %w", err)
	}

	// Argo CD tokens don't always have an expiry claim as they can be configured to last forever
	if exp == nil {
		return nil, nil
	}

	expiresAt := exp.Time
	if clock.Now().After(expiresAt) {
		return &expiresAt, fmt.Errorf("auth token is expired. It expired at %v", expiresAt)
	}

	return &expiresAt, nil
}

// ValidateAuthTokens parses every configured auth token and returns a
// TokenValidationReport describing the outcome. The global token (if any) is
// validated first; project tokens follow in alphabetical order so the report
// is deterministic.
func ValidateAuthTokens(
	globalToken string,
	projectTokens map[string]string,
	nearExpiryThreshold time.Duration,
	clock utilities.Clock,
) TokenValidationReport {
	report := TokenValidationReport{}

	classify := func(label, token string) {
		expiresAt, err := ValidateAuthToken(token, clock)
		issue := TokenIssue{Label: label, ExpiresAt: expiresAt, Err: err}
		report.Tokens = append(report.Tokens, issue)

		if err != nil {
			report.Invalid = append(report.Invalid, issue)
			return
		}
		if expiresAt == nil {
			return
		}
		if expiresAt.Sub(clock.Now()) <= nearExpiryThreshold {
			report.NearExpiry = append(report.NearExpiry, issue)
		}
	}

	// Validate the global token when configured, or when no project tokens are
	// configured either — in the latter case ValidateAuthToken surfaces the
	// "no auth token provided" error.
	if globalToken != "" || len(projectTokens) == 0 {
		classify(GlobalTokenLabel, globalToken)
	}

	projectNames := make([]string, 0, len(projectTokens))
	for name := range projectTokens {
		projectNames = append(projectNames, name)
	}
	sort.Strings(projectNames)
	for _, name := range projectNames {
		classify(name, projectTokens[name])
	}

	return report
}

func LogValidationReport(logger *slog.Logger, report TokenValidationReport) {
	nearExpiry := labelSet(report.NearExpiry)
	invalid := labelSet(report.Invalid)

	for _, t := range report.Tokens {
		status := statusForToken(t, invalid, nearExpiry)
		attrs := []any{
			slog.String("project", t.Label),
			slog.String("expires_at", formatExpiry(t.ExpiresAt)),
			slog.String("status", status),
		}
		if t.Err != nil {
			attrs = append(attrs, slog.String("error", t.Err.Error()))
		}
		logger.Info("Argo CD authentication token status", attrs...)
	}

	if len(report.Invalid) > 0 {
		logger.With(slog.Any("tokens", formatInvalidTokenIssues(report.Invalid))).
			Warn("One or more Argo CD authentication tokens failed validation")
	}
	if len(report.NearExpiry) > 0 {
		logger.With(slog.Any("tokens", formatNearExpiryTokenIssues(report.NearExpiry))).
			Warn("One or more Argo CD authentication tokens are close to expiry")
	}
}

func statusForToken(t TokenIssue, invalid, nearExpiry map[string]struct{}) string {
	if t.Err != nil {
		return tokenStatusInvalid
	}
	if _, ok := invalid[t.Label]; ok {
		return tokenStatusInvalid
	}
	if _, ok := nearExpiry[t.Label]; ok {
		return tokenStatusNearExpiry
	}
	return tokenStatusOK
}

func formatExpiry(expiresAt *time.Time) string {
	if expiresAt == nil {
		return "never"
	}
	return expiresAt.UTC().Format(time.RFC3339)
}

func labelSet(issues []TokenIssue) map[string]struct{} {
	out := make(map[string]struct{}, len(issues))
	for _, issue := range issues {
		out[issue.Label] = struct{}{}
	}
	return out
}

func formatInvalidTokenIssues(issues []TokenIssue) []map[string]any {
	out := make([]map[string]any, 0, len(issues))
	for _, issue := range issues {
		entry := map[string]any{
			"label": issue.Label,
			"error": issue.Err.Error(),
		}
		if issue.ExpiresAt != nil {
			entry["expires_at"] = issue.ExpiresAt.UTC().Format(time.RFC3339)
		}
		out = append(out, entry)
	}
	return out
}

func formatNearExpiryTokenIssues(issues []TokenIssue) []map[string]any {
	out := make([]map[string]any, 0, len(issues))
	for _, issue := range issues {
		entry := map[string]any{"label": issue.Label}
		if issue.ExpiresAt != nil {
			entry["expires_at"] = issue.ExpiresAt.UTC().Format(time.RFC3339)
		}
		out = append(out, entry)
	}
	return out
}
