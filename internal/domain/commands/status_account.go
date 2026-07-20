package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// StatusCommand prints the active account and its current usage.
type StatusCommand struct {
	config   *entities.Config
	accounts repositories.AccountsRepository
	usage    repositories.UsageRepository
	tokens   repositories.TokensRepository
	now      func() time.Time
}

// NewStatusCommand creates a StatusCommand.
func NewStatusCommand(
	config *entities.Config,
	accounts repositories.AccountsRepository,
	usage repositories.UsageRepository,
	tokens repositories.TokensRepository,
) *StatusCommand {
	return &StatusCommand{
		config:   config,
		accounts: accounts,
		usage:    usage,
		tokens:   tokens,
		now:      time.Now,
	}
}

// Execute prints the current account and its usage summary.
func (c *StatusCommand) Execute() error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	if len(store.Accounts) == 0 {
		fmt.Fprintln(os.Stdout, "[ccswitch] no accounts enrolled; run `ccswitch enroll`")
		return nil
	}

	warnAPIKeyOverride()

	current := store.FindAccount(store.Rotation.CurrentEmail)
	if current == nil {
		fmt.Fprintln(os.Stdout, "[ccswitch] no current account set; run `ccswitch use <email>`")
		return nil
	}

	fmt.Fprintf(os.Stdout, "current account: %s\n", current.Email)

	usage, _, pollErr := pollUsage(c.usage, c.tokens, &current.Credentials, c.now().UnixMilli())
	if pollErr != nil {
		fmt.Fprintf(os.Stderr, "[ccswitch] could not fetch usage: %v\n", pollErr)
		return nil
	}

	printUsage(os.Stdout, usage, c.config.Threshold)
	return nil
}
