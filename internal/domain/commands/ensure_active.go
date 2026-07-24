package commands

import (
	"fmt"
	"os"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// EnsureActiveCommand makes sure the credentials on disk belong to the current
// account, using no network calls. It is the fast pre-launch consistency guard
// used by the `claude` shell wrapper.
type EnsureActiveCommand struct {
	accounts    repositories.AccountsRepository
	credentials repositories.CredentialsRepository
}

// NewEnsureActiveCommand creates an EnsureActiveCommand.
func NewEnsureActiveCommand(
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
) *EnsureActiveCommand {
	return &EnsureActiveCommand{accounts: accounts, credentials: credentials}
}

// Execute installs the current account's credentials when the on-disk credentials
// belong to a different account. It is a no-op when nothing is enrolled or the
// current account is already active.
func (c *EnsureActiveCommand) Execute(quiet bool) error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	if store.Rotation.CurrentEmail == "" {
		return nil
	}

	account := store.FindAccount(store.Rotation.CurrentEmail)
	if account == nil {
		return nil
	}

	// Resolve the installed credentials by identity, not by refresh token: Claude
	// Code rotates the refresh token on every refresh, and treating a rotated
	// token as a different account would overwrite freshly refreshed credentials
	// with the stale stored ones, breaking the very account being launched.
	onDisk, identity, readErr := c.credentials.Read()
	if readErr == nil && store.MatchAccount(*onDisk, identity) == account {
		return c.capture(store, account, onDisk)
	}

	if err = c.credentials.Write(&account.Credentials, &account.Identity); err != nil {
		return err
	}

	if !quiet {
		fmt.Fprintf(os.Stdout, "[ccswitch] ensured active account: %s\n", account.Email)
	}
	return nil
}

// capture folds credentials that Claude Code refreshed on disk back into the
// stored account. Without this the store keeps a refresh token that the server
// has already rotated away, which fails every later refresh with 401; doing it
// here means the store stays current even when the monitor daemon is not
// running, since the shell wrapper calls `ensure` on every launch.
func (c *EnsureActiveCommand) capture(
	store *entities.Store,
	account *entities.Account,
	onDisk *entities.OAuthCredentials,
) error {
	if account.Credentials.AccessToken == onDisk.AccessToken &&
		account.Credentials.RefreshToken == onDisk.RefreshToken {
		return nil
	}
	account.Credentials = *onDisk
	account.LongLived = onDisk.RefreshToken == ""
	return c.accounts.Save(store)
}
