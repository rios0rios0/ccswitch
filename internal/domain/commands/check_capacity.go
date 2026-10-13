package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// ErrNoCapacity reports that every enrolled account the rotation can select is
// exhausted. It is an answer rather than a failure: the controllers turn it into
// an exit status of its own, which the shell integration acts on.
var ErrNoCapacity = errors.New("no enrolled account has capacity")

// CheckCapacityCommand tells whether an enrolled account has capacity left. It
// reads the store and nothing else -- no network, no credentials -- because the
// shell integration runs it before every claude launch, alongside `ensure`.
type CheckCapacityCommand struct {
	accounts repositories.AccountsRepository
	out      io.Writer
	now      func() time.Time
}

// NewCheckCapacityCommand creates a CheckCapacityCommand that prints to standard
// output.
func NewCheckCapacityCommand(accounts repositories.AccountsRepository) *CheckCapacityCommand {
	return &CheckCapacityCommand{accounts: accounts, out: os.Stdout, now: time.Now}
}

// WithOutput redirects what the command prints, which goes to standard output
// unless this is called.
func (c *CheckCapacityCommand) WithOutput(out io.Writer) *CheckCapacityCommand {
	c.out = out
	return c
}

// Execute returns ErrNoCapacity when every account the rotation can select is
// exhausted, after printing which one recovers first and when, unless quiet. It
// prints nothing and returns nil while an account has capacity, and also when
// ccswitch cannot tell (see entities.Store.Capacity).
func (c *CheckCapacityCommand) Execute(quiet bool) error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}

	now := c.now()
	capacity := store.Capacity(now)
	if capacity.Available {
		return nil
	}

	if !quiet {
		fmt.Fprintf(c.out, "[ccswitch] no enrolled account has capacity; %s recovers first, %s\n",
			capacity.Next.Email, formatMoment(now, capacity.Recovers))
	}
	return ErrNoCapacity
}
