package commands

import (
	"fmt"
	"os"

	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// ReorderAccountsCommand sets the rotation order of the enrolled accounts in one
// step.
type ReorderAccountsCommand struct {
	accounts repositories.AccountsRepository
}

// NewReorderAccountsCommand creates a ReorderAccountsCommand.
func NewReorderAccountsCommand(accounts repositories.AccountsRepository) *ReorderAccountsCommand {
	return &ReorderAccountsCommand{accounts: accounts}
}

// Execute puts the named accounts first, in the order given, and keeps every other
// account behind them in its current relative order. Nothing is saved unless every
// named account is enrolled and named once. Like `update`, it changes no
// credentials: a running daemon applies the new order on its next poll.
func (c *ReorderAccountsCommand) Execute(emails []string) error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	if err = store.Reorder(emails); err != nil {
		return err
	}
	if err = c.accounts.Save(store); err != nil {
		return err
	}

	fmt.Fprintln(os.Stdout, "[ccswitch] rotation order:")
	for i, account := range store.Ordered() {
		fmt.Fprintf(os.Stdout, "  %d. %s\n", i+1, account.Email)
	}
	return nil
}
