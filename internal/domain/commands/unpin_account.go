package commands

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// UnpinAccountCommand hands the choice of account back to the rotation policy: it
// releases the pin that `use` or `rotate` set, in every provider it is given, and
// applies the policy to those providers at once.
type UnpinAccountCommand struct {
	config *entities.Config
	// pools are the providers whose pin is released, in the order given: the first is
	// the one the constructor was given, and the rest come from WithPool.
	pools []Pool
	out   io.Writer
	now   func() time.Time
}

// NewUnpinAccountCommand creates an UnpinAccountCommand over the accounts of one
// provider that prints to standard output. More providers join through WithPool.
func NewUnpinAccountCommand(
	config *entities.Config,
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
	usage repositories.UsageRepository,
	tokens repositories.TokensRepository,
	sessions repositories.SessionsRepository,
) *UnpinAccountCommand {
	return &UnpinAccountCommand{
		config: config,
		pools: []Pool{{
			Accounts:    accounts,
			Credentials: credentials,
			Usage:       usage,
			Tokens:      tokens,
			Sessions:    sessions,
		}},
		out: os.Stdout,
		now: time.Now,
	}
}

// WithPool releases another provider's pin as well, after those of the pools it
// already has.
func (c *UnpinAccountCommand) WithPool(pool Pool) *UnpinAccountCommand {
	c.pools = append(c.pools, pool)
	return c
}

// WithOutput redirects what the command prints, which goes to standard output
// unless this is called.
func (c *UnpinAccountCommand) WithOutput(out io.Writer) *UnpinAccountCommand {
	c.out = out
	return c
}

// Execute releases the pin of every provider whose current account is pinned and
// re-applies the rotation policy to those providers right away, then reports which
// account each of them has active. With nothing pinned it says so and changes
// nothing.
//
// The policy is applied by running a monitor cycle over the providers whose pin was
// released, as `threshold` does, rather than by choosing an account here: that is
// what keeps `unpin` and the daemon agreeing on which account should be active. The
// switch it makes waits for the next launch while a session of the provider's CLI is
// running, as every switch does.
func (c *UnpinAccountCommand) Execute() error {
	stores, err := loadStores(c.pools)
	if err != nil {
		return err
	}

	released := make([]Pool, 0, len(c.pools))
	for i, store := range stores {
		if !store.Rotation.Pinned() {
			continue
		}
		email := store.Rotation.PinnedEmail
		store.Rotation.Unpin()
		if err = c.pools[i].Accounts.Save(store); err != nil {
			return err
		}
		fmt.Fprintf(c.out, "[ccswitch] unpinned %s\n", email)
		released = append(released, c.pools[i])
	}
	if len(released) == 0 {
		fmt.Fprintln(c.out, "[ccswitch] no account is pinned")
		return nil
	}

	first := released[0]
	monitor := NewMonitorCommand(c.config, first.Accounts, first.Credentials, first.Usage, first.Tokens, first.Sessions)
	for _, pool := range released[1:] {
		monitor.WithPool(pool)
	}
	if err = monitor.Tick(c.now()); err != nil {
		return fmt.Errorf("failed to re-apply the rotation policy: %w", err)
	}
	return c.report(released, namesProviders(stores))
}

// report prints which account each provider whose pin was released has active now,
// naming the provider when named.
func (c *UnpinAccountCommand) report(released []Pool, named bool) error {
	stores, err := loadStores(released)
	if err != nil {
		return err
	}
	for i, store := range stores {
		reportActive(c.out, released[i], store, named)
	}
	return nil
}
