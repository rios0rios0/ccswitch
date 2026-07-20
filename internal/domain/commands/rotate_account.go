package commands

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

const minAccountsForRotation = 2

// RotateAccountCommand advances the active account to the next healthy account in
// rotation order.
type RotateAccountCommand struct {
	accounts    repositories.AccountsRepository
	credentials repositories.CredentialsRepository
	sessions    repositories.SessionsRepository
	now         func() time.Time
}

// NewRotateAccountCommand creates a RotateAccountCommand.
func NewRotateAccountCommand(
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
	sessions repositories.SessionsRepository,
) *RotateAccountCommand {
	return &RotateAccountCommand{
		accounts:    accounts,
		credentials: credentials,
		sessions:    sessions,
		now:         time.Now,
	}
}

// Execute switches to the next healthy account. When a claude session is running
// and force is false, the switch is deferred to the next launch.
func (c *RotateAccountCommand) Execute(force bool) error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	if len(store.Accounts) < minAccountsForRotation {
		return errors.New("enroll at least two accounts before rotating (see `ccswitch enroll`)")
	}

	next, ok := store.NextHealthyAccount(c.now())
	if !ok || next.Email == store.Rotation.CurrentEmail {
		fmt.Fprintln(os.Stderr, "[ccswitch] no other healthy account available to rotate to")
		return nil
	}

	if c.sessions != nil && c.sessions.ClaudeRunning() && !force {
		store.Rotation.CurrentEmail = next.Email
		if err = c.accounts.Save(store); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout,
			"[ccswitch] claude is running; will switch to %s on next launch (use --force to switch now)\n",
			next.Email)
		return nil
	}

	if err = c.credentials.Write(&next.Credentials, &next.Identity); err != nil {
		return err
	}
	store.Rotation.CurrentEmail = next.Email
	if err = c.accounts.Save(store); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "[ccswitch] rotated to %s\n", next.Email)
	return nil
}
