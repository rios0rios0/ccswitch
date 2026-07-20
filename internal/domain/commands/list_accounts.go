package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// ListAccountsCommand lists all enrolled accounts with their live usage.
type ListAccountsCommand struct {
	config   *entities.Config
	accounts repositories.AccountsRepository
	usage    repositories.UsageRepository
	tokens   repositories.TokensRepository
	now      func() time.Time
}

// NewListAccountsCommand creates a ListAccountsCommand.
func NewListAccountsCommand(
	config *entities.Config,
	accounts repositories.AccountsRepository,
	usage repositories.UsageRepository,
	tokens repositories.TokensRepository,
) *ListAccountsCommand {
	return &ListAccountsCommand{
		config:   config,
		accounts: accounts,
		usage:    usage,
		tokens:   tokens,
		now:      time.Now,
	}
}

// Execute prints one line per enrolled account, marking the current account with
// an asterisk and showing each account's 5-hour and 7-day utilization.
func (c *ListAccountsCommand) Execute() error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	if len(store.Accounts) == 0 {
		fmt.Fprintln(os.Stdout, "[ccswitch] no accounts enrolled; run `ccswitch enroll`")
		return nil
	}

	warnAPIKeyOverride()

	now := c.now()
	ordered := store.Ordered()
	for i := range ordered {
		c.printAccount(&ordered[i], store, now)
	}
	return nil
}

// printAccount renders a single account row, polling its live usage.
func (c *ListAccountsCommand) printAccount(account *entities.Account, store *entities.Store, now time.Time) {
	marker := " "
	if account.Email == store.Rotation.CurrentEmail {
		marker = "*"
	}
	state := "ok"
	if store.Rotation.IsExhausted(account.Email, now) {
		state = "exhausted"
	}

	usage, _, err := pollUsage(c.usage, c.tokens, &account.Credentials, now.UnixMilli())
	if err != nil {
		fmt.Fprintf(os.Stdout, "%s %d. %-32s [%-9s] usage unavailable\n",
			marker, account.Order, account.Email, state)
		return
	}

	binding, _ := usage.BindingLimit()
	fmt.Fprintf(os.Stdout, "%s %d. %-32s [%-9s] 5h=%3.0f%% 7d=%3.0f%% binding=%s:%.0f%%\n",
		marker, account.Order, account.Email, state,
		usage.FiveHour.Utilization, usage.SevenDay.Utilization, binding.Kind, binding.Percent)
}
