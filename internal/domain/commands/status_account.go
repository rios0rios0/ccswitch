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

// StatusCommand prints the active account of every provider it is given, and its
// current usage.
type StatusCommand struct {
	config *entities.Config
	// pools are the providers whose active account is printed, in the order given: the
	// first is the one the constructor was given, and the rest come from WithPool.
	pools []Pool
	out   io.Writer
	now   func() time.Time
}

// NewStatusCommand creates a StatusCommand over the accounts of one provider that
// prints to standard output. More providers join through WithPool.
func NewStatusCommand(
	config *entities.Config,
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
	usage repositories.UsageRepository,
	tokens repositories.TokensRepository,
) *StatusCommand {
	return &StatusCommand{
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

// WithPool prints another provider's active account as well, after those of the
// pools it already has.
func (c *StatusCommand) WithPool(pool Pool) *StatusCommand {
	c.pools = append(c.pools, pool)
	return c
}

// WithOutput redirects what the command prints, which goes to standard output
// unless this is called.
func (c *StatusCommand) WithOutput(out io.Writer) *StatusCommand {
	c.out = out
	return c
}

// Execute prints the current account of each provider that has accounts enrolled,
// and its usage summary, under the provider's name once a provider other than Claude
// has accounts (see namesProviders). Reading usage refreshes a spent token, so the
// resulting credentials are persisted before the summary is rendered.
func (c *StatusCommand) Execute() error {
	stores, err := loadStores(c.pools)
	if err != nil {
		return err
	}
	if enrolledCount(stores) == 0 {
		fmt.Fprintln(c.out, "[ccswitch] no accounts enrolled; run `ccswitch enroll`")
		return nil
	}

	named := namesProviders(stores)
	failures := make([]error, 0, len(stores))
	for i, store := range stores {
		if len(store.Accounts) == 0 {
			continue
		}
		if named {
			fmt.Fprintln(c.out, store.Provider.Name())
		}
		if err = c.printCurrent(c.pools[i], store); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// printCurrent prints one provider's current account and its usage summary.
func (c *StatusCommand) printCurrent(pool Pool, store *entities.Store) error {
	warnAPIKeyOverride(store.Provider)

	current := store.FindAccount(store.Rotation.CurrentEmail)
	if current == nil {
		fmt.Fprintln(c.out, "[ccswitch] no current account set; run `ccswitch use <email>`")
		return nil
	}

	fmt.Fprintf(c.out, "current account: %s\n", current.Email)

	if !current.SupportsUsagePolling() {
		fmt.Fprintln(c.out,
			"  usage:   not available (enrolled from a long-lived token, which lacks the "+
				"`user:profile` scope the usage endpoint requires)")
		return nil
	}

	now := c.now()
	previous := current.Credentials
	usage, creds, pollErr := pollUsage(pool.Usage, pool.Tokens, store.Provider, &current.Credentials, now.UnixMilli())
	// Persist before handling the poll error: a refresh that already succeeded
	// rotated the token server-side and cannot be undone, so discarding it would
	// strand both the store and the CLI on a token the server has invalidated.
	if err := persistRefreshed(pool, store, current, previous, creds); err != nil {
		return err
	}

	if pollErr != nil {
		fmt.Fprintf(os.Stderr, "[ccswitch] could not fetch usage: %v\n", pollErr)
		return nil
	}

	printUsage(c.out, usage, c.config.ResolveThreshold(store.Settings), now)
	return nil
}

// persistRefreshed saves credentials a poll refreshed, both into the store and back
// to the credentials store the provider's CLI reads. It is a no-op when nothing
// changed.
func persistRefreshed(
	pool Pool,
	store *entities.Store,
	current *entities.Account,
	previous, creds entities.OAuthCredentials,
) error {
	if creds.AccessToken == previous.AccessToken && creds.RefreshToken == previous.RefreshToken {
		return nil
	}
	current.Credentials = creds
	publishRefreshed(pool.Credentials, previous, current)
	return pool.Accounts.Save(store)
}
