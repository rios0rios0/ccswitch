package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// EnrollAccountCommand captures the currently logged-in Claude account into the
// ccswitch store so it can take part in rotation.
type EnrollAccountCommand struct {
	accounts    repositories.AccountsRepository
	credentials repositories.CredentialsRepository
}

// NewEnrollAccountCommand creates an EnrollAccountCommand.
func NewEnrollAccountCommand(
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
) *EnrollAccountCommand {
	return &EnrollAccountCommand{accounts: accounts, credentials: credentials}
}

// Execute reads the active Claude credentials, resolves the account email, and
// upserts the account into the store. The first enrolled account becomes current.
func (c *EnrollAccountCommand) Execute() error {
	creds, identity, err := c.credentials.Read()
	if err != nil {
		return fmt.Errorf("failed to read current Claude credentials "+
			"(run `claude` and `/login` first): %w", err)
	}
	if !creds.Valid() {
		return errors.New("current Claude credentials are incomplete; log in with `claude` first")
	}

	email := accountEmail(identity)
	if email == "" {
		return errors.New("could not determine the account email; " +
			"ensure ~/.claude.json contains an oauthAccount block")
	}

	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	upsertAccount(store, email, identity, creds)

	if err = c.accounts.Save(store); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "[ccswitch] enrolled %s (%d account(s), current: %s)\n",
		email, len(store.Accounts), store.Rotation.CurrentEmail)
	return nil
}

// upsertAccount inserts or updates the account for the given email, assigning a
// rotation order to new accounts and setting the first account as current.
func upsertAccount(
	store *entities.Store,
	email string,
	identity *entities.AccountIdentity,
	creds *entities.OAuthCredentials,
) {
	uuid := ""
	if identity != nil {
		uuid = identity.AccountUUID
	}

	if existing := store.FindAccount(email); existing != nil {
		existing.Credentials = *creds
		existing.AccountUUID = uuid
		if identity != nil {
			existing.Identity = *identity
		}
		return
	}

	account := entities.Account{
		Email:       email,
		AccountUUID: uuid,
		Order:       store.NextOrder(),
		Credentials: *creds,
	}
	if identity != nil {
		account.Identity = *identity
	}
	store.Accounts = append(store.Accounts, account)

	if store.Rotation.CurrentEmail == "" {
		store.Rotation.CurrentEmail = email
	}
}
