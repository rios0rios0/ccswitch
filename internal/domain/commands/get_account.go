package commands

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// GetAccountCommand prints everything ccswitch knows about one enrolled account,
// including its live usage.
type GetAccountCommand struct {
	config      *entities.Config
	accounts    repositories.AccountsRepository
	credentials repositories.CredentialsRepository
	usage       repositories.UsageRepository
	tokens      repositories.TokensRepository
	out         io.Writer
	now         func() time.Time
}

// NewGetAccountCommand creates a GetAccountCommand that prints to standard output.
func NewGetAccountCommand(
	config *entities.Config,
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
	usage repositories.UsageRepository,
	tokens repositories.TokensRepository,
) *GetAccountCommand {
	return &GetAccountCommand{
		config:      config,
		accounts:    accounts,
		credentials: credentials,
		usage:       usage,
		tokens:      tokens,
		out:         os.Stdout,
		now:         time.Now,
	}
}

// WithOutput redirects what the command prints, which goes to standard output
// unless this is called.
func (c *GetAccountCommand) WithOutput(out io.Writer) *GetAccountCommand {
	c.out = out
	return c
}

// Execute prints the account enrolled under the given email: where it stands in
// the rotation order, whether it has capacity and when each of its limits resets,
// its plan, and when its tokens expire. Reading its usage can refresh a spent
// token, so the store is saved whenever the poll produced new credentials.
//
// On a terminal the account is dressed up as Config.Output says, the way list
// dresses up every account: a meter beside every figure, colored by how close the
// figure stands to the threshold, the reset times in columns, its state in color,
// and its email in red when it is exhausted. Anywhere else it is plain text.
func (c *GetAccountCommand) Execute(email string) error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	account := store.FindAccount(email)
	if account == nil {
		return notEnrolled(email)
	}

	now := c.now()
	var usage *entities.Usage
	var pollErr error
	refreshed := false
	if account.SupportsUsagePolling() {
		previous := account.Credentials
		var creds entities.OAuthCredentials
		usage, creds, pollErr = pollUsage(c.usage, c.tokens, &account.Credentials, now.UnixMilli())
		// Capture before handling the error: a refresh that already succeeded rotated
		// the token server-side and cannot be undone.
		refreshed = captureRefreshed(c.credentials, store, email, previous, creds)
	}
	if pollErr != nil {
		fmt.Fprintf(os.Stderr, "[ccswitch] could not fetch usage for %s: %v\n", email, pollErr)
	}

	threshold := c.config.ResolveThreshold(store.Settings)
	c.printSummary(store, account, usage, now, threshold)
	c.printCredentials(account, now)
	c.printUsage(account, usage, now, threshold)

	if !refreshed {
		return nil
	}
	return c.accounts.Save(store)
}

// printSummary prints the account's heading, its place in the rotation order, and
// whether it has capacity, with the email in red when it has none. The state comes
// from the live reading when there is one, and from the exhaustion marker the
// monitor keeps otherwise.
func (c *GetAccountCommand) printSummary(
	store *entities.Store,
	account *entities.Account,
	usage *entities.Usage,
	now time.Time,
	threshold float64,
) {
	look := c.palette()
	available := markerAvailability(store, account.Email, now)
	if usage != nil {
		available = usageAvailability(usage, threshold)
	}

	color, state := toneStrong, look.state(available, now)
	switch {
	case !account.SupportsUsagePolling():
		state = look.paint(toneManual, "manual only") + " " + look.paint(toneMuted,
			"(long-lived token; its usage cannot be polled, so select it with `ccswitch use`)")
	case available.exhausted:
		color = toneExhausted
	}

	heading := look.paint(color, account.Email)
	if account.Email == store.Rotation.CurrentEmail {
		heading += " " + look.paint(toneActive, "(active)")
	}
	fmt.Fprintln(c.out, heading)
	fmt.Fprintf(c.out, "  priority:       %s\n",
		describePosition(store.Position(account.Email), len(store.Accounts)))
	fmt.Fprintf(c.out, "  state:          %s\n", state)
}

// printCredentials prints what the stored credentials say about the account,
// never the tokens themselves: its plan, when its tokens expire, and whether the
// credentials have lost the scope Claude Code insists on.
func (c *GetAccountCommand) printCredentials(account *entities.Account, now time.Time) {
	look := c.palette()
	creds := account.Credentials
	if plan := describePlan(creds); plan != "" {
		fmt.Fprintf(c.out, "  plan:           %s\n", plan)
	}
	fmt.Fprintf(c.out, "  access token:   %s\n", describeExpiry(now, creds.ExpiresAt, look))
	if creds.RefreshTokenExpiresAt != 0 {
		fmt.Fprintf(c.out, "  refresh token:  %s\n", describeExpiry(now, creds.RefreshTokenExpiresAt, look))
	}
	if creds.Degraded() {
		fmt.Fprintf(c.out, "  warning:        %s\n", look.paint(toneCaution, fmt.Sprintf(
			"the credentials lack the %q scope, so Claude Code will discard them; "+
				"log in again with `claude` and re-enroll", entities.ScopeInference)))
	}
}

// printUsage prints when the monitor last polled the account and every limit the
// live reading reported, falling back to the last reading the monitor recorded
// when no live one could be taken.
func (c *GetAccountCommand) printUsage(
	account *entities.Account,
	usage *entities.Usage,
	now time.Time,
	threshold float64,
) {
	if !account.SupportsUsagePolling() {
		return
	}

	look := c.palette()
	polled := "never by the monitor"
	if !account.LastPolledAt.IsZero() {
		polled = look.moment(now, account.LastPolledAt) + " by the monitor"
	}
	fmt.Fprintf(c.out, "  last polled:    %s\n", polled)

	switch {
	case usage != nil:
		fmt.Fprintln(c.out, "  usage:")
		printReadings(c.out, usage, now, threshold, look)
	case account.LastUsage != nil:
		fmt.Fprintln(c.out, "  usage:          "+look.paint(toneCaution, "unavailable; last known reading:"))
		printReadings(c.out, account.LastUsage, now, threshold, look)
	default:
		fmt.Fprintln(c.out, "  usage:          "+look.paint(toneCaution, "unavailable"))
	}
}

// palette dresses the account up in the output style the configuration resolved.
func (c *GetAccountCommand) palette() palette {
	return palette{style: c.config.Output}
}

// describePosition phrases a place in the rotation order, naming the primary.
func describePosition(position, count int) string {
	if position == 1 {
		return fmt.Sprintf("1 of %d (primary)", count)
	}
	return fmt.Sprintf("%d of %d", position, count)
}

// describePlan names the subscription the credentials were minted for, with its
// rate-limit tier when known, or returns empty when the credentials carry neither.
func describePlan(creds entities.OAuthCredentials) string {
	if creds.SubscriptionType == "" {
		return creds.RateLimitTier
	}
	if creds.RateLimitTier == "" {
		return creds.SubscriptionType
	}
	return fmt.Sprintf("%s (%s)", creds.SubscriptionType, creds.RateLimitTier)
}

// describeExpiry phrases when a token expires, given its expiry in epoch
// milliseconds as the credentials store it, with the moment in the palette's
// tones.
func describeExpiry(now time.Time, expiresAtMillis int64, look palette) string {
	if expiresAtMillis == 0 {
		return "no expiry recorded"
	}
	expires := time.UnixMilli(expiresAtMillis)
	if expires.After(now) {
		return "expires " + look.moment(now, expires)
	}
	return "expired " + look.moment(now, expires)
}
