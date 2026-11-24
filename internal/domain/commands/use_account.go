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
// An account the monitor last read exhausted is installed and pinned through that
// exhaustion: the person picking it overrules a reading that cannot tell an account at
// its limit that is still served from one that is refused (see
// RotationState.PinnedThrough). The command warns that the monitor will not rotate away
// from it until that exhaustion ends, so it is the person who has to move on should the
// provider's CLI refuse the account after all. An account its provider refused outright
// is no such reading (see Store.PinHolds): it is still installed and pinned, since the
// refusal may have ended since, but the command warns that the monitor's next poll
// releases the pin unless it has.
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

	now := c.now()
	store.Rotation.Pin(email)
	if err = c.accounts.Save(store); err != nil {
		return err
	}

	holds := store.PinHolds(now)
	fmt.Fprintf(c.out, "[ccswitch] switched to %s\n", email)
	fmt.Fprintln(c.out, pinNote(account, holds))
	switch {
	case holds:
		c.warnOverride(store, email, now)
	case store.Rotation.IsExhausted(email, now):
		c.warnExhausted(store, email, now)
	}
	return nil
}

// warnExhausted warns that the account is pinned in vain when the monitor last saw its
// provider refuse it: unless the refusal has ended since, the next poll releases the pin
// and rotates away from it.
func (c *UseAccountCommand) warnExhausted(store *entities.Store, email string, now time.Time) {
	fmt.Fprintf(c.errOut,
		"[ccswitch] WARN: %s is exhausted, available again %s; unless its limits have reset since, "+
			"the monitor's next poll releases the pin and rotates away from it\n",
		email, formatMoment(now, store.Rotation.ExhaustedUntil[email]))
}

// warnOverride warns that the account was picked over the exhaustion the monitor last
// read: the pin holds until that exhaustion ends, so the monitor no longer rotates away
// from the account on its own in the meantime, and should the provider's CLI refuse it
// after all, the person who overruled the reading has to pick another.
func (c *UseAccountCommand) warnOverride(store *entities.Store, email string, now time.Time) {
	fmt.Fprintf(c.errOut,
		"[ccswitch] WARN: %s last read exhausted, available again %s; it stays pinned until then "+
			"regardless, since an account at its limit may still be served -- if %s refuses it, "+
			"pick another account or run `ccswitch unpin`\n",
		email, formatMoment(now, store.Rotation.PinnedThrough), store.Provider.Command())
}

// pinNote tells the person who just picked an account by hand how long it stays
// active: until it is exhausted; through the exhaustion the monitor last read and until
// the next one, for an account picked over it; or, for an account whose usage cannot be
// polled and whose exhaustion ccswitch therefore never sees, until another account is
// picked.
func pinNote(account *entities.Account, holdsThroughExhaustion bool) string {
	const release = "`ccswitch unpin` lets the rotation policy choose again"
	switch {
	case !account.SupportsUsagePolling():
		return "[ccswitch] pinned until you pick another account, since its usage cannot be polled; " + release
	case holdsThroughExhaustion:
		return "[ccswitch] pinned through the exhaustion the monitor last read, and after that until it is " +
			"exhausted again; " + release
	default:
		return "[ccswitch] pinned until it is exhausted; " + release
	}
}
