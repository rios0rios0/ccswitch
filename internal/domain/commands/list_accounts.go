package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// ListAccountsCommand lists the enrolled accounts of every provider it is given, with
// their live usage.
type ListAccountsCommand struct {
	config *entities.Config
	// pools are the providers whose accounts are listed, in the order given: the first
	// is the one the constructor was given, and the rest come from WithPool.
	pools []Pool
	out   io.Writer
	now   func() time.Time
}

// NewListAccountsCommand creates a ListAccountsCommand over the accounts of one
// provider that prints to standard output. More providers join through WithPool.
func NewListAccountsCommand(
	config *entities.Config,
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
	usage repositories.UsageRepository,
	tokens repositories.TokensRepository,
) *ListAccountsCommand {
	return &ListAccountsCommand{
		config: config,
		pools: []Pool{{
			Accounts:    accounts,
			Credentials: credentials,
			Usage:       usage,
			Tokens:      tokens,
		}},
		out: os.Stdout,
		now: time.Now,
	}
}

// WithPool lists another provider's accounts as well, after those of the pools it
// already has.
func (c *ListAccountsCommand) WithPool(pool Pool) *ListAccountsCommand {
	c.pools = append(c.pools, pool)
	return c
}

// WithOutput redirects what the command prints, which goes to standard output
// unless this is called.
func (c *ListAccountsCommand) WithOutput(out io.Writer) *ListAccountsCommand {
	c.out = out
	return c
}

// Execute prints the enrolled accounts of each provider in rotation order, numbered
// from 1 for that provider's primary, marking the account its CLI runs on with an
// asterisk. Under each account it prints every limit the usage endpoint reported,
// with its utilization and when it resets. Listing polls usage, which refreshes any
// spent token, so a provider's store is saved whenever a poll produced new
// credentials for one of its accounts.
//
// On a terminal the listing is dressed up as Config.Output says: each provider's
// accounts under its logo and name in its brand color, a meter beside every figure,
// colored by how close the figure stands to the threshold, the reset times in
// columns, and exhausted accounts in red. Anywhere else it is plain text, which
// names the providers only once a provider other than Claude has accounts enrolled
// (see namesProviders).
func (c *ListAccountsCommand) Execute() error {
	stores, err := loadStores(c.pools)
	if err != nil {
		return err
	}
	if enrolledCount(stores) == 0 {
		fmt.Fprintln(c.out, "[ccswitch] no accounts enrolled; run `ccswitch enroll`")
		return nil
	}
	for _, store := range stores {
		if len(store.Accounts) > 0 {
			warnAPIKeyOverride(store.Provider)
		}
	}

	look := c.palette()
	threshold := c.config.ResolveThreshold(stores[0].Settings)
	fmt.Fprintln(c.out, "rotation threshold: "+look.paint(toneStrong, fmt.Sprintf("%.0f%%", threshold)))

	named := look.style.Decorated() || namesProviders(stores)
	now := c.now()
	failures := make([]error, 0, len(stores))
	for i, store := range stores {
		if err = c.printPool(c.pools[i], store, named, now, threshold); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// printPool prints one provider's accounts, under the provider's name when named,
// and saves the provider's store when a poll refreshed one of its accounts.
func (c *ListAccountsCommand) printPool(
	pool Pool,
	store *entities.Store,
	named bool,
	now time.Time,
	threshold float64,
) error {
	look := c.palette()
	ordered := store.Ordered()
	refreshed := false
	for i := range ordered {
		// The meters under each account would run into the next one's heading, so a
		// decorated listing sets every account off with a blank line.
		if look.style.Decorated() {
			fmt.Fprintln(c.out)
		}
		if i == 0 && named {
			fmt.Fprintln(c.out, look.providerName(store.Provider))
		}
		if c.printAccount(pool, i+1, &ordered[i], store, now, threshold) {
			refreshed = true
		}
	}
	if !refreshed {
		return nil
	}
	return pool.Accounts.Save(store)
}

// printAccount renders one account, polling its live usage, and reports whether
// the poll refreshed that account's credentials.
func (c *ListAccountsCommand) printAccount(
	pool Pool,
	position int,
	account *entities.Account,
	store *entities.Store,
	now time.Time,
	threshold float64,
) bool {
	look := c.palette()
	active := account.Email == store.Rotation.CurrentEmail

	if !account.SupportsUsagePolling() {
		fmt.Fprintf(c.out, "%s [%s] %s\n",
			look.heading(position, account.Email, active, toneStrong),
			look.paint(toneManual, "manual only"),
			look.paint(toneMuted, "long-lived token; its usage cannot be polled"))
		return false
	}

	previous := account.Credentials
	usage, creds, err := pollUsage(pool.Usage, pool.Tokens, store.Provider, &account.Credentials, now.UnixMilli())
	// Capture before handling the error: a refresh that already succeeded rotated
	// the token server-side and cannot be undone, so its result has to be kept even
	// when the usage call that followed it failed.
	refreshed := captureRefreshed(pool.Credentials, store, account.Email, previous, creds)

	if err != nil {
		c.printHeading(position, account.Email, active, markerAvailability(store, account.Email, now), now)
		c.printLastKnown(account, now, threshold)
		return refreshed
	}

	c.printHeading(position, account.Email, active, usageAvailability(usage, threshold), now)
	printReadings(c.out, usage, now, threshold, look)
	return refreshed
}

// printHeading prints the line an account's readings go under: its position, its
// email, and whether it can be used, with the email in red when it cannot.
func (c *ListAccountsCommand) printHeading(
	position int,
	email string,
	active bool,
	account availability,
	now time.Time,
) {
	look := c.palette()
	color := toneStrong
	if account.exhausted {
		color = toneExhausted
	}
	fmt.Fprintf(c.out, "%s [%s]\n", look.heading(position, email, active, color), look.state(account, now))
}

// printLastKnown stands in for a live reading that could not be taken, such as
// when the usage endpoint is rate-limiting. The monitor records the usage every
// successful poll sees, and the reset times in it are absolute, so they still
// answer when each limit comes back even though the percentages may have moved.
func (c *ListAccountsCommand) printLastKnown(account *entities.Account, now time.Time, threshold float64) {
	look := c.palette()
	if account.LastUsage == nil {
		fmt.Fprintln(c.out, readingIndent+look.paint(toneCaution, "usage unavailable"))
		return
	}
	fmt.Fprintln(c.out, readingIndent+look.paint(toneCaution, "usage unavailable; last known reading:"))
	printReadings(c.out, account.LastUsage, now, threshold, look)
}

// palette dresses the listing up in the output style the configuration resolved.
func (c *ListAccountsCommand) palette() palette {
	return palette{style: c.config.Output}
}

// loadStores loads the store of every pool, in order.
func loadStores(pools []Pool) ([]*entities.Store, error) {
	stores := make([]*entities.Store, 0, len(pools))
	for _, pool := range pools {
		store, err := pool.Accounts.Load()
		if err != nil {
			return nil, err
		}
		stores = append(stores, store)
	}
	return stores, nil
}

// enrolledCount returns how many accounts the stores hold together.
func enrolledCount(stores []*entities.Store) int {
	count := 0
	for _, store := range stores {
		count += len(store.Accounts)
	}
	return count
}

// namesProviders reports whether output that is not dressed up names the provider of
// each store's accounts, which it does once a provider other than Claude has accounts
// enrolled. Until then every account is Claude's, and the plain output stays exactly
// what it was before ccswitch knew of another provider, since scripts read it.
func namesProviders(stores []*entities.Store) bool {
	for _, store := range stores {
		if len(store.Accounts) > 0 && store.Provider.Resolved() != entities.ProviderClaude {
			return true
		}
	}
	return false
}
