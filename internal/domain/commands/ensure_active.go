package commands

import (
	"fmt"
	"os"

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

	if onDisk, _, readErr := c.credentials.Read(); readErr == nil &&
		onDisk.SameAccountAs(account.Credentials) {
		return nil
	}

	if err = c.credentials.Write(&account.Credentials, &account.Identity); err != nil {
		return err
	}

	if !quiet {
		fmt.Fprintf(os.Stdout, "[ccswitch] ensured active account: %s\n", account.Email)
	}
	return nil
}
