package commands

import (
	"fmt"
	"os"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// UpdateAccountCommand changes an enrolled account's place in the rotation order,
// which is the one attribute of an account that is not captured from its login or
// read from its usage.
type UpdateAccountCommand struct {
	accounts repositories.AccountsRepository
}

// NewUpdateAccountCommand creates an UpdateAccountCommand.
func NewUpdateAccountCommand(accounts repositories.AccountsRepository) *UpdateAccountCommand {
	return &UpdateAccountCommand{accounts: accounts}
}

// Execute moves the named account to the place the priority names, shifting the
// accounts in between. It changes no credentials: the monitor reloads the store
// on every tick, so a running daemon applies the new order on its next poll.
func (c *UpdateAccountCommand) Execute(email string, priority entities.Priority) error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	current := store.Position(email)
	if current == 0 {
		return notEnrolled(email)
	}

	count := len(store.Accounts)
	position, err := priority.Position(current, count)
	if err != nil {
		return err
	}
	if position == current {
		fmt.Fprintf(os.Stdout, "[ccswitch] %s is already at position %s\n",
			email, describePosition(position, count))
		return nil
	}

	if err = store.Move(email, position); err != nil {
		return err
	}
	if err = c.accounts.Save(store); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "[ccswitch] moved %s from position %d to %s\n",
		email, current, describePosition(position, count))
	return nil
}
