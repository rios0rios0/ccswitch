package commands

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// ListAccountsCommand lists all enrolled accounts with their live usage.
type ListAccountsCommand struct {
	config      *entities.Config
	accounts    repositories.AccountsRepository
	credentials repositories.CredentialsRepository
	usage       repositories.UsageRepository
	tokens      repositories.TokensRepository
	out         io.Writer
	now         func() time.Time
}

// NewListAccountsCommand creates a ListAccountsCommand that prints to standard
// output.
func NewListAccountsCommand(
	config *entities.Config,
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
	usage repositories.UsageRepository,
	tokens repositories.TokensRepository,
) *ListAccountsCommand {
	return &ListAccountsCommand{
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
func (c *ListAccountsCommand) WithOutput(out io.Writer) *ListAccountsCommand {
	c.out = out
	return c
}

// Execute prints the enrolled accounts in rotation order, numbered from 1 for the
// primary, marking the current account with an asterisk. Under each account it
// prints every limit the usage endpoint reported, with its utilization and when it
// resets. Listing polls usage, which refreshes any spent token, so the store is
// saved whenever a poll produced new credentials.
func (c *ListAccountsCommand) Execute() error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	if len(store.Accounts) == 0 {
		fmt.Fprintln(c.out, "[ccswitch] no accounts enrolled; run `ccswitch enroll`")
		return nil
	}

	warnAPIKeyOverride()

	threshold := c.config.ResolveThreshold(store.Settings)
	fmt.Fprintf(c.out, "rotation threshold: %.0f%%\n", threshold)

	now := c.now()
	ordered := store.Ordered()
	refreshed := false
	for i := range ordered {
		if c.printAccount(i+1, &ordered[i], store, now, threshold) {
			refreshed = true
		}
	}
	if !refreshed {
		return nil
	}
	return c.accounts.Save(store)
}

// printAccount renders one account, polling its live usage, and reports whether
// the poll refreshed that account's credentials.
func (c *ListAccountsCommand) printAccount(
	position int,
	account *entities.Account,
	store *entities.Store,
	now time.Time,
	threshold float64,
) bool {
	marker := " "
	if account.Email == store.Rotation.CurrentEmail {
		marker = "*"
	}
	heading := fmt.Sprintf("%s %d. %s", marker, position, account.Email)

	if !account.SupportsUsagePolling() {
		fmt.Fprintf(c.out, "%s [manual only] long-lived token; its usage cannot be polled\n", heading)
		return false
	}

	previous := account.Credentials
	usage, creds, err := pollUsage(c.usage, c.tokens, &account.Credentials, now.UnixMilli())
	// Capture before handling the error: a refresh that already succeeded rotated
	// the token server-side and cannot be undone, so its result has to be kept even
	// when the usage call that followed it failed.
	refreshed := captureRefreshed(c.credentials, store, account.Email, previous, creds)

	if err != nil {
		fmt.Fprintf(c.out, "%s [%s]\n", heading, markerState(store, account.Email, now))
		c.printLastKnown(account, now)
		return refreshed
	}

	fmt.Fprintf(c.out, "%s [%s]\n", heading, usageState(usage, threshold, now))
	printReadings(c.out, usage, now)
	return refreshed
}

// printLastKnown stands in for a live reading that could not be taken, such as
// when the usage endpoint is rate-limiting. The monitor records the usage every
// successful poll sees, and the reset times in it are absolute, so they still
// answer when each limit comes back even though the percentages may have moved.
func (c *ListAccountsCommand) printLastKnown(account *entities.Account, now time.Time) {
	if account.LastUsage == nil {
		fmt.Fprintln(c.out, readingIndent+"usage unavailable")
		return
	}
	fmt.Fprintln(c.out, readingIndent+"usage unavailable; last known reading:")
	printReadings(c.out, account.LastUsage, now)
}
