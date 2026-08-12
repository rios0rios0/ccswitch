package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// StatusCommand prints the active account and its current usage.
type StatusCommand struct {
	config      *entities.Config
	accounts    repositories.AccountsRepository
	credentials repositories.CredentialsRepository
	usage       repositories.UsageRepository
	tokens      repositories.TokensRepository
	now         func() time.Time
}

// NewStatusCommand creates a StatusCommand.
func NewStatusCommand(
	config *entities.Config,
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
	usage repositories.UsageRepository,
	tokens repositories.TokensRepository,
) *StatusCommand {
	return &StatusCommand{
		config:      config,
		accounts:    accounts,
		credentials: credentials,
		usage:       usage,
		tokens:      tokens,
		now:         time.Now,
	}
}

// Execute prints the current account and its usage summary. Reading usage
// refreshes a spent token, so the resulting credentials are persisted before the
// summary is rendered.
func (c *StatusCommand) Execute() error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	if len(store.Accounts) == 0 {
		fmt.Fprintln(os.Stdout, "[ccswitch] no accounts enrolled; run `ccswitch enroll`")
		return nil
	}

	warnAPIKeyOverride()

	current := store.FindAccount(store.Rotation.CurrentEmail)
	if current == nil {
		fmt.Fprintln(os.Stdout, "[ccswitch] no current account set; run `ccswitch use <email>`")
		return nil
	}

	fmt.Fprintf(os.Stdout, "current account: %s\n", current.Email)

	if !current.SupportsUsagePolling() {
		fmt.Fprintln(os.Stdout,
			"  usage:   not available (enrolled from a long-lived token, which lacks the "+
				"`user:profile` scope the usage endpoint requires)")
		return nil
	}

	previous := current.Credentials
	usage, creds, pollErr := pollUsage(c.usage, c.tokens, &current.Credentials, c.now().UnixMilli())
	// Persist before handling the poll error: a refresh that already succeeded
	// rotated the token server-side and cannot be undone, so discarding it would
	// strand both the store and Claude Code on a token the server has invalidated.
	if err = c.persist(store, current, previous, creds); err != nil {
		return err
	}

	if pollErr != nil {
		fmt.Fprintf(os.Stderr, "[ccswitch] could not fetch usage: %v\n", pollErr)
		return nil
	}

	printUsage(os.Stdout, usage, c.config.Threshold)
	return nil
}

// persist saves credentials the poll refreshed, both into the store and back to
// the credentials store Claude Code reads. It is a no-op when nothing changed.
func (c *StatusCommand) persist(
	store *entities.Store,
	current *entities.Account,
	previous, creds entities.OAuthCredentials,
) error {
	if creds.AccessToken == previous.AccessToken && creds.RefreshToken == previous.RefreshToken {
		return nil
	}
	current.Credentials = creds
	publishRefreshed(c.credentials, previous, current)
	return c.accounts.Save(store)
}
