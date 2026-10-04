package commands

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// DeleteAccountsCommand removes enrolled accounts from the store.
type DeleteAccountsCommand struct {
	accounts    repositories.AccountsRepository
	credentials repositories.CredentialsRepository
	sessions    repositories.SessionsRepository
	now         func() time.Time
}

// NewDeleteAccountsCommand creates a DeleteAccountsCommand.
func NewDeleteAccountsCommand(
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
	sessions repositories.SessionsRepository,
) *DeleteAccountsCommand {
	return &DeleteAccountsCommand{
		accounts:    accounts,
		credentials: credentials,
		sessions:    sessions,
		now:         time.Now,
	}
}

// Execute removes every named account, or none of them when any is not enrolled.
// The stored tokens go with an account, so enrolling it again takes a login as
// that account first.
//
// Removing the current account hands over to the account the monitor would pick
// (see Store.Successor). The store is saved before that account's credentials are
// installed, so a failed install still leaves a consistent store for `ensure` or
// the monitor to finish from; and while a claude session is running the install
// waits for the next launch, as any switch does.
func (c *DeleteAccountsCommand) Execute(emails []string) error {
	if len(emails) == 0 {
		return errors.New("name at least one account to remove")
	}
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	for _, email := range emails {
		if store.FindAccount(email) == nil {
			return notEnrolled(email)
		}
	}

	removedCurrent := false
	for _, email := range emails {
		// An email named twice was already removed by its first mention.
		if !store.Remove(email) {
			continue
		}
		if email == store.Rotation.CurrentEmail {
			removedCurrent = true
		}
		fmt.Fprintf(os.Stdout, "[ccswitch] removed %s\n", email)
	}

	if !removedCurrent {
		return c.accounts.Save(store)
	}
	return c.handOver(store)
}

// handOver points the store at the account that takes over from a removed
// current one, saves the store, and installs that account's credentials.
func (c *DeleteAccountsCommand) handOver(store *entities.Store) error {
	successor, ok := store.Successor(c.now())
	if !ok {
		store.Rotation.CurrentEmail = ""
		if err := c.accounts.Save(store); err != nil {
			return err
		}
		if len(store.Accounts) == 0 {
			fmt.Fprintln(os.Stdout, "[ccswitch] no accounts remain enrolled")
			return nil
		}
		fmt.Fprintln(os.Stdout, "[ccswitch] only long-lived accounts remain, and those are never "+
			"selected automatically; pick one with `ccswitch use <email>`")
		return nil
	}

	store.Rotation.CurrentEmail = successor.Email
	if err := c.accounts.Save(store); err != nil {
		return err
	}
	if c.sessions != nil && c.sessions.ClaudeRunning() {
		fmt.Fprintf(os.Stdout, "[ccswitch] claude is running; %s takes over on the next launch\n",
			successor.Email)
		return nil
	}
	if err := c.credentials.Write(&successor.Credentials, &successor.Identity); err != nil {
		return fmt.Errorf("failed to install %s, which takes over as the active account: %w",
			successor.Email, err)
	}
	fmt.Fprintf(os.Stdout, "[ccswitch] switched to %s\n", successor.Email)
	return nil
}
