// Package commands implements the ccswitch application logic: enrolling accounts,
// switching credentials, and the monitor loop that rotates on exhaustion.
package commands

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

const (
	envAPIKey       = "ANTHROPIC_API_KEY"    //nolint:gosec // env var name, not a secret
	envAuthToken    = "ANTHROPIC_AUTH_TOKEN" //nolint:gosec // env var name, not a secret
	resetTimeLayout = "Mon 15:04"
)

// pollUsage fetches usage for the given credentials, refreshing the access token
// first when it is expired and a refresher is available. It returns the usage and
// the (possibly refreshed) credentials so the caller can persist them.
func pollUsage(
	usageRepo repositories.UsageRepository,
	tokensRepo repositories.TokensRepository,
	creds *entities.OAuthCredentials,
	nowMillis int64,
) (*entities.Usage, entities.OAuthCredentials, error) {
	current := *creds
	if current.AccessTokenExpired(nowMillis) && tokensRepo != nil && current.RefreshToken != "" {
		refreshed, err := tokensRepo.Refresh(current.RefreshToken)
		if err != nil {
			return nil, current, fmt.Errorf("failed to refresh access token: %w", err)
		}
		current = *refreshed
	}
	usage, err := usageRepo.Fetch(current.AccessToken)
	if err != nil {
		return nil, current, err
	}
	return usage, current, nil
}

// apiKeyOverride reports whether an environment variable is set that would cause
// Claude Code to bypass the rotated OAuth credentials, and its name.
func apiKeyOverride() (string, bool) {
	for _, name := range []string{envAPIKey, envAuthToken} {
		if os.Getenv(name) != "" {
			return name, true
		}
	}
	return "", false
}

// warnAPIKeyOverride prints a warning to stderr when an API-key environment
// variable would shadow OAuth credentials.
func warnAPIKeyOverride() {
	if name, ok := apiKeyOverride(); ok {
		fmt.Fprintf(os.Stderr,
			"[ccswitch] WARN: %s is set; Claude Code will ignore rotated OAuth credentials\n", name)
	}
}

// identityKnown reports whether the identity can attribute installed credentials
// to an enrolled account, treating a nil identity as unknown.
func identityKnown(identity *entities.AccountIdentity) bool {
	return identity != nil && identity.Known()
}

// accountEmail returns the email address from an identity, or empty when unknown.
func accountEmail(identity *entities.AccountIdentity) string {
	if identity == nil {
		return ""
	}
	return identity.EmailAddress
}

// printUsage renders a compact usage summary to the writer.
func printUsage(writer io.Writer, usage *entities.Usage, threshold float64) {
	fmt.Fprintf(writer, "  5-hour:  %3.0f%% (resets %s)\n",
		usage.FiveHour.Utilization, formatReset(usage.FiveHour.ResetsAt))
	fmt.Fprintf(writer, "  7-day:   %3.0f%% (resets %s)\n",
		usage.SevenDay.Utilization, formatReset(usage.SevenDay.ResetsAt))
	if binding, ok := usage.BindingLimit(); ok {
		fmt.Fprintf(writer, "  binding: %s %.0f%% (%s, resets %s)\n",
			binding.Kind, binding.Percent, binding.Severity, formatReset(binding.ResetsAt))
	}
	if usage.Exhausted(threshold) {
		fmt.Fprintln(writer, "  status:  EXHAUSTED")
	} else {
		fmt.Fprintln(writer, "  status:  ok")
	}
}

// formatReset formats a reset timestamp in local time, or "unknown" for the zero
// time.
func formatReset(reset time.Time) string {
	if reset.IsZero() {
		return "unknown"
	}
	return reset.Local().Format(resetTimeLayout)
}
