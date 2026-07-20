package commands

import (
	"fmt"
	"os"

	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// UseAccountCommand switches the active Claude account to a specific enrolled one.
type UseAccountCommand struct {
	accounts    repositories.AccountsRepository
	credentials repositories.CredentialsRepository
}

// NewUseAccountCommand creates a UseAccountCommand.
func NewUseAccountCommand(
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
) *UseAccountCommand {
	return &UseAccountCommand{accounts: accounts, credentials: credentials}
}

// Execute installs the credentials for the named account and records it as the
// current account.
func (c *UseAccountCommand) Execute(email string) error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}

	account := store.FindAccount(email)
	if account == nil {
		return fmt.Errorf("account %q is not enrolled; run `ccswitch list` to see enrolled accounts", email)
	}

	if err = c.credentials.Write(&account.Credentials, &account.Identity); err != nil {
		return err
	}

	store.Rotation.CurrentEmail = email
	if err = c.accounts.Save(store); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "[ccswitch] switched to %s\n", email)
	return nil
}
