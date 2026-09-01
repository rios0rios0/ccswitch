// Package commands implements the ccswitch application logic: enrolling accounts,
// switching credentials, and the monitor loop that rotates on exhaustion.
package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

const (
	envAPIKey       = "ANTHROPIC_API_KEY"    //nolint:gosec // env var name, not a secret
	envAuthToken    = "ANTHROPIC_AUTH_TOKEN" //nolint:gosec // env var name, not a secret
	resetTimeLayout = "Mon 15:04"
)

// pollUsage fetches usage for the given credentials, refreshing the access token
// when needed. It returns the usage and the (possibly refreshed) credentials so
// the caller can persist them — including on the error paths, because a refresh
// that already succeeded rotated the token server-side and cannot be undone.
//
// A refresh is attempted up front when the recorded expiry has passed, and again
// when the server rejects the token despite that expiry still being in the
// future: tokens are invalidated server-side on their own schedule, so the
// timestamp alone is not enough to tell a live token from a dead one. Without the
// second attempt an account whose token was invalidated early stays unreadable
// until it is enrolled again.
func pollUsage(
	usageRepo repositories.UsageRepository,
	tokensRepo repositories.TokensRepository,
	creds *entities.OAuthCredentials,
	nowMillis int64,
) (*entities.Usage, entities.OAuthCredentials, error) {
	current := *creds
	canRefresh := tokensRepo != nil && current.RefreshToken != ""

	// A degraded set is refreshed even though its access token is still live: it
	// is the repair path for accounts an earlier ccswitch stripped, and leaving
	// them alone until the token ages out means the next rotation still installs a
	// credential document Claude Code will not keep.
	if (current.AccessTokenExpired(nowMillis) || current.Degraded()) && canRefresh {
		refreshed, err := tokensRepo.Refresh(current)
		if err != nil {
			return nil, current, fmt.Errorf("failed to refresh access token: %w", err)
		}
		if refreshed == nil {
			return nil, current, errors.New("token refresh returned no credentials")
		}
		current = *refreshed
		// The token is as fresh as it gets; a rejection below is not staleness, and
		// retrying would spend the new refresh token to no purpose.
		canRefresh = false
	}

	usage, err := usageRepo.Fetch(current.AccessToken)
	if err == nil {
		return usage, current, nil
	}
	if !canRefresh || !errors.Is(err, repositories.ErrUnauthorized) {
		return nil, current, err
	}

	refreshed, refreshErr := tokensRepo.Refresh(current)
	if refreshErr != nil {
		return nil, current, fmt.Errorf("failed to refresh rejected access token: %w", refreshErr)
	}
	if refreshed == nil {
		return nil, current, errors.New("token refresh returned no credentials")
	}
	current = *refreshed

	usage, err = usageRepo.Fetch(current.AccessToken)
	if err != nil {
		return nil, current, err
	}
	return usage, current, nil
}

// publishRefreshed writes credentials a poll just refreshed back to the
// credentials store when that store still holds the pair the refresh consumed.
//
// The server rotates the refresh token on every refresh and invalidates the
// previous one, so keeping the new pair only in the ccswitch store leaves Claude
// Code holding a token the server has already killed: its next refresh fails with
// invalid_grant and the session is logged out. Because a refresh only happens
// once the access token is spent, this bites idle sessions in particular.
//
// Publishing the same account's newer tokens is not an account switch, so it is
// safe while a session is running -- the running-session guard in switchTo exists
// to avoid swapping a live process onto a different account, which this is not.
func publishRefreshed(
	credentials repositories.CredentialsRepository,
	previous entities.OAuthCredentials,
	account *entities.Account,
) {
	if credentials == nil {
		return
	}
	if account.Credentials.AccessToken == previous.AccessToken &&
		account.Credentials.RefreshToken == previous.RefreshToken {
		return
	}

	onDisk, _, err := credentials.Read()
	if err != nil || onDisk == nil {
		return
	}
	// Only publish when the store still carries exactly what the refresh consumed.
	// Anything else means another writer got there first, and its tokens are at
	// least as fresh as these -- including the case where a different account is
	// installed, which must not be overwritten here.
	if onDisk.AccessToken != previous.AccessToken || onDisk.RefreshToken != previous.RefreshToken {
		return
	}

	if err = credentials.Write(&account.Credentials, &account.Identity); err != nil {
		logger.Warnf("[ccswitch] failed to publish refreshed credentials for %s: %v",
			account.Email, err)
		return
	}
	logger.Debugf("[ccswitch] published refreshed credentials for %s", account.Email)
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
