package commands

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// UseAccountCommand switches the active account of a provider to a specific enrolled
// one, and pins it there.
type UseAccountCommand struct {
	accounts    repositories.AccountsRepository
	credentials repositories.CredentialsRepository
	out         io.Writer
	errOut      io.Writer
	now         func() time.Time
}

// NewUseAccountCommand creates a UseAccountCommand that prints to standard output
// and warns on standard error.
func NewUseAccountCommand(
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
) *UseAccountCommand {
	return &UseAccountCommand{
		accounts:    accounts,
		credentials: credentials,
		out:         os.Stdout,
		errOut:      os.Stderr,
		now:         time.Now,
	}
}

// WithOutput redirects what the command prints, which goes to standard output
// unless this is called.
func (c *UseAccountCommand) WithOutput(out io.Writer) *UseAccountCommand {
	c.out = out
	return c
}

// WithErrorOutput redirects the command's warnings, which go to standard error
// unless this is called.
func (c *UseAccountCommand) WithErrorOutput(errOut io.Writer) *UseAccountCommand {
	c.errOut = errOut
	return c
}

// Execute installs the credentials for the named account and records it as the
// current account, pinned: the monitor keeps an account picked by hand active until
// it is exhausted, rather than switching back to the one its policy prefers on its
// next poll (see MonitorCommand.reconcile).
//
// An account the monitor last saw exhausted is still installed and pinned, since its
// limits may have reset since, but the command warns that the monitor's next poll
// releases the pin unless they have.
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

	store.Rotation.Pin(email)
	if err = c.accounts.Save(store); err != nil {
		return err
	}

	fmt.Fprintf(c.out, "[ccswitch] switched to %s\n", email)
	fmt.Fprintln(c.out, pinNote(account))
	c.warnExhausted(store, email)
	return nil
}

// warnExhausted warns that the account is pinned in vain when the monitor last saw it
// exhausted: unless its limits have reset since, the next poll releases the pin and
// rotates away from it.
func (c *UseAccountCommand) warnExhausted(store *entities.Store, email string) {
	now := c.now()
	if !store.Rotation.IsExhausted(email, now) {
		return
	}
	fmt.Fprintf(c.errOut,
		"[ccswitch] WARN: %s is exhausted, available again %s; unless its limits have reset since, "+
			"the monitor's next poll releases the pin and rotates away from it\n",
		email, formatMoment(now, store.Rotation.ExhaustedUntil[email]))
}

// pinNote tells the person who just picked an account by hand how long it stays
// active: until it is exhausted, or, for an account whose usage cannot be polled and
// whose exhaustion ccswitch therefore never sees, until another account is picked.
func pinNote(account *entities.Account) string {
	const release = "`ccswitch unpin` lets the rotation policy choose again"
	if !account.SupportsUsagePolling() {
		return "[ccswitch] pinned until you pick another account, since its usage cannot be polled; " + release
	}
	return "[ccswitch] pinned until it is exhausted; " + release
}
