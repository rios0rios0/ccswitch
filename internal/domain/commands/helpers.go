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
	envAPIKey    = "ANTHROPIC_API_KEY"    //nolint:gosec // env var name, not a secret
	envAuthToken = "ANTHROPIC_AUTH_TOKEN" //nolint:gosec // env var name, not a secret
	// momentLayout spells out the date as well as the weekday: a weekly limit can
	// reset a full week out, on the weekday it is today.
	momentLayout = "Mon Jan 2 15:04"
	hoursPerDay  = 24
	// readingIndent lines a reading up under the email of the account it belongs
	// to, past the "* 1. " marker and position that start the account's line.
	readingIndent = "     "
	// readingLabelWidth fits the longest label readingLabel produces.
	readingLabelWidth = 12
	stateOK           = "ok"
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

// notEnrolled reports an email that names no enrolled account, pointing at the
// command that lists the ones that are.
func notEnrolled(email string) error {
	return fmt.Errorf("%w: %s; run `ccswitch list` to see enrolled accounts",
		entities.ErrAccountNotEnrolled, email)
}

// accountEmail returns the email address from an identity, or empty when unknown.
func accountEmail(identity *entities.AccountIdentity) string {
	if identity == nil {
		return ""
	}
	return identity.EmailAddress
}

// captureRefreshed writes credentials a poll refreshed back into the stored
// account and publishes them to the credentials store, reporting whether it did.
//
// The account is looked up on the store rather than taken from the caller,
// because Store.Ordered hands out copies. Dropping the refreshed pair instead would
// pin the store to a refresh token the server invalidated the instant the refresh
// rotated it, leaving the account unreadable until it is enrolled again.
func captureRefreshed(
	credentials repositories.CredentialsRepository,
	store *entities.Store,
	email string,
	previous, creds entities.OAuthCredentials,
) bool {
	if creds.AccessToken == previous.AccessToken && creds.RefreshToken == previous.RefreshToken {
		return false
	}
	stored := store.FindAccount(email)
	if stored == nil {
		return false
	}
	stored.Credentials = creds
	publishRefreshed(credentials, previous, stored)
	return true
}

// printUsage renders a compact usage summary to the writer.
func printUsage(writer io.Writer, usage *entities.Usage, threshold float64, now time.Time) {
	fmt.Fprintf(writer, "  5-hour:  %3.0f%%, resets %s\n",
		usage.FiveHour.Utilization, formatMoment(now, usage.FiveHour.ResetsAt))
	fmt.Fprintf(writer, "  7-day:   %3.0f%%, resets %s\n",
		usage.SevenDay.Utilization, formatMoment(now, usage.SevenDay.ResetsAt))
	if binding, ok := usage.BindingLimit(); ok {
		fmt.Fprintf(writer, "  binding: %s %.0f%% (%s), resets %s\n",
			binding.Kind, binding.Percent, binding.Severity, formatMoment(now, binding.ResetsAt))
	}
	if usage.Exhausted(threshold) {
		fmt.Fprintln(writer, "  status:  EXHAUSTED")
	} else {
		fmt.Fprintln(writer, "  status:  ok")
	}
}

// printReadings writes one line per utilization figure in the usage: its label,
// its percentage, and when it resets. A figure the endpoint reported no reset time
// for is printed without one, rather than with a reset reading "unknown".
func printReadings(writer io.Writer, usage *entities.Usage, now time.Time) {
	for _, reading := range usage.Readings() {
		line := fmt.Sprintf("%s%-*s %4.0f%%",
			readingIndent, readingLabelWidth, readingLabel(reading.Kind), reading.Percent)
		if !reading.ResetsAt.IsZero() {
			line += "  " + describeReset(now, reading.ResetsAt)
		}
		fmt.Fprintln(writer, line)
	}
}

// readingLabel names a reading the way Claude Code's own usage screen describes
// it, falling back to the raw kind for a limit this version does not know.
func readingLabel(kind string) string {
	labels := map[string]string{
		entities.LimitKindSession:      "5-hour",
		entities.LimitKindWeeklyAll:    "7-day",
		entities.LimitKindWeeklyScoped: "7-day scoped",
	}
	if label, ok := labels[kind]; ok {
		return label
	}
	return kind
}

// usageState sums up what a live reading says about the account: "ok", or
// exhausted together with when it is available again, which is when every limit
// over the threshold has reset (see Usage.RecoversAt).
func usageState(usage *entities.Usage, threshold float64, now time.Time) string {
	if !usage.Exhausted(threshold) {
		return stateOK
	}
	return exhaustedState(now, usage.RecoversAt(threshold))
}

// markerState sums up the account from its exhaustion marker, for when no live
// reading is at hand.
func markerState(store *entities.Store, email string, now time.Time) string {
	if !store.Rotation.IsExhausted(email, now) {
		return stateOK
	}
	return exhaustedState(now, store.Rotation.ExhaustedUntil[email])
}

// exhaustedState phrases an exhausted account and, when known, when it is
// available again.
func exhaustedState(now, recovers time.Time) string {
	if recovers.IsZero() {
		return "exhausted"
	}
	return "exhausted, available again " + formatMoment(now, recovers)
}

// describeReset phrases when a limit resets: "resets in 2h 13m (Tue Sep 29
// 17:47)", or "reset 5m ago (...)" for a moment already past, which only a stale
// reading shows.
func describeReset(now, reset time.Time) string {
	if reset.After(now) {
		return "resets " + formatMoment(now, reset)
	}
	return "reset " + formatMoment(now, reset)
}

// formatMoment renders a moment relative to now and in local time: "in 2h 13m
// (Tue Sep 29 17:47)" ahead of now, "5m ago (Tue Sep 29 17:29)" behind it, or
// "unknown" for the zero time.
func formatMoment(now, moment time.Time) string {
	if moment.IsZero() {
		return "unknown"
	}
	when := moment.Local().Format(momentLayout)
	if moment.After(now) {
		return fmt.Sprintf("in %s (%s)", formatDuration(moment.Sub(now)), when)
	}
	return fmt.Sprintf("%s ago (%s)", formatDuration(now.Sub(moment)), when)
}

// formatDuration renders a duration at the precision a countdown needs: days and
// hours, hours and minutes, or minutes alone, leaving out a smaller unit that
// reads zero. Whatever falls below the smallest unit shown is dropped, the way a
// countdown clock reads.
func formatDuration(duration time.Duration) string {
	day := hoursPerDay * time.Hour
	days := int64(duration / day)
	hours := int64(duration % day / time.Hour)
	minutes := int64(duration % time.Hour / time.Minute)

	switch {
	case days > 0:
		return joinUnits(days, "d", hours, "h")
	case hours > 0:
		return joinUnits(hours, "h", minutes, "m")
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return "<1m"
	}
}

// joinUnits renders a count in a larger unit followed by one in a smaller unit,
// leaving the smaller one out when it is zero: "2d 4h", but "2d".
func joinUnits(major int64, majorUnit string, minor int64, minorUnit string) string {
	if minor == 0 {
		return fmt.Sprintf("%d%s", major, majorUnit)
	}
	return fmt.Sprintf("%d%s %d%s", major, majorUnit, minor, minorUnit)
}
