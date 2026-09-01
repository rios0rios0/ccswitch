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
	config      *entities.Config
	accounts    repositories.AccountsRepository
	credentials repositories.CredentialsRepository
	usage       repositories.UsageRepository
	tokens      repositories.TokensRepository
	now         func() time.Time
}

// NewListAccountsCommand creates a ListAccountsCommand.
func NewListAccountsCommand(
	config *entities.Config,
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
	usage repositories.UsageRepository,
	tokens repositories.TokensRepository,
) *ListAccountsCommand {
	return &ListAccountsCommand{
		config:      config,
		accounts:    accounts,
		credentials: credentials,
		usage:       usage,
		tokens:      tokens,
		now:         time.Now,
	}
}

// Execute prints one line per enrolled account, marking the current account with
// an asterisk and showing each account's 5-hour and 7-day utilization. Listing
// polls usage, which refreshes any spent token, so the store is saved whenever a
// poll produced new credentials.
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

	fmt.Fprintf(os.Stdout, "rotation threshold: %.0f%%\n",
		c.config.ResolveThreshold(store.Settings))

	now := c.now()
	ordered := store.Ordered()
	refreshed := false
	for i := range ordered {
		if c.printAccount(&ordered[i], store, now) {
			refreshed = true
		}
	}
	if !refreshed {
		return nil
	}
	return c.accounts.Save(store)
}

// printAccount renders a single account row, polling its live usage. It reports
// whether the poll refreshed that account's credentials.
func (c *ListAccountsCommand) printAccount(
	account *entities.Account,
	store *entities.Store,
	now time.Time,
) bool {
	marker := " "
	if account.Email == store.Rotation.CurrentEmail {
		marker = "*"
	}
	state := "ok"
	if store.Rotation.IsExhausted(account.Email, now) {
		state = "exhausted"
	}

	if !account.SupportsUsagePolling() {
		fmt.Fprintf(os.Stdout, "%s %d. %-32s [%-9s] long-lived token (usage not pollable; manual only)\n",
			marker, account.Order, account.Email, state)
		return false
	}

	previous := account.Credentials
	usage, creds, err := pollUsage(c.usage, c.tokens, &account.Credentials, now.UnixMilli())
	// Capture before handling the error: a refresh that already succeeded rotated
	// the token server-side and cannot be undone, so its result has to be kept even
	// when the usage call that followed it failed.
	refreshed := c.capture(store, account.Email, previous, creds)

	if err != nil {
		fmt.Fprintf(os.Stdout, "%s %d. %-32s [%-9s] usage unavailable\n",
			marker, account.Order, account.Email, state)
		return refreshed
	}

	binding, _ := usage.BindingLimit()
	fmt.Fprintf(os.Stdout, "%s %d. %-32s [%-9s] 5h=%3.0f%% 7d=%3.0f%% binding=%s:%.0f%%\n",
		marker, account.Order, account.Email, state,
		usage.FiveHour.Utilization, usage.SevenDay.Utilization, binding.Kind, binding.Percent)
	return refreshed
}

// capture writes credentials a poll refreshed back into the stored account and
// reports whether it did.
//
// Store.Ordered hands out copies, so the refreshed pair has to be looked up on
// the store itself to survive. Dropping it instead would pin the store to a
// refresh token the server invalidated the instant the refresh rotated it,
// leaving the account permanently unreadable until it is enrolled again.
func (c *ListAccountsCommand) capture(
	store *entities.Store,
	email string,
	previous, creds entities.OAuthCredentials,
) bool {
	if creds.AccessToken == previous.AccessToken && creds.RefreshToken == previous.RefreshToken {
		return false
	}
	stored := store.FindAccount(email)
	if stored == nil {
		return false
	}
	stored.Credentials = creds
	publishRefreshed(c.credentials, previous, stored)
	return true
}
