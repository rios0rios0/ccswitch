package commands

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// longLivedTokenTTL stamps the assumed lifetime of a token minted by `claude
// setup-token` for display purposes. It is never used to trigger a refresh: a
// token enrolled this way carries no refresh token, so pollUsage always uses it
// as-is (see helpers.go).
const longLivedTokenTTL = 365 * 24 * time.Hour

// EnrollAccountCommand captures a Claude account into the ccswitch store so it
// can take part in rotation.
type EnrollAccountCommand struct {
	accounts    repositories.AccountsRepository
	credentials repositories.CredentialsRepository
	now         func() time.Time
}

// NewEnrollAccountCommand creates an EnrollAccountCommand.
func NewEnrollAccountCommand(
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
) *EnrollAccountCommand {
	return &EnrollAccountCommand{accounts: accounts, credentials: credentials, now: time.Now}
}

// Execute enrolls an account into the store. When token is non-empty it is
// enrolled directly under email (e.g. a long-lived token from `claude
// setup-token`), bypassing the Claude Code credentials file entirely; such an
// account can only be selected manually, because its token lacks the scope the
// usage endpoint requires. Otherwise the currently logged-in account is read
// from disk. The first enrolled account becomes current.
func (c *EnrollAccountCommand) Execute(token, email string) error {
	creds, identity, err := c.resolve(token, email)
	if err != nil {
		return err
	}
	resolvedEmail := accountEmail(identity)

	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	// A long-lived token lacks the `user:profile` scope, so its usage can never be
	// read; record that so the monitor skips polling it instead of failing.
	upsertAccount(store, resolvedEmail, identity, creds).LongLived = token != ""

	if err = c.accounts.Save(store); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "[ccswitch] enrolled %s (%d account(s), current: %s)\n",
		resolvedEmail, len(store.Accounts), store.Rotation.CurrentEmail)
	if token != "" {
		fmt.Fprintf(os.Stdout,
			"[ccswitch] %s uses a long-lived token: its usage cannot be polled, so it is "+
				"never rotated to automatically; select it with `ccswitch use %s`\n",
			resolvedEmail, resolvedEmail)
	}
	return nil
}

// resolve returns the credentials and identity to enroll, either built directly
// from a supplied long-lived token or read from the active Claude Code session.
func (c *EnrollAccountCommand) resolve(
	token, email string,
) (*entities.OAuthCredentials, *entities.AccountIdentity, error) {
	if token != "" {
		if email == "" {
			return nil, nil, errors.New("--email is required when --token is set")
		}
		creds := &entities.OAuthCredentials{
			AccessToken: token,
			ExpiresAt:   c.now().Add(longLivedTokenTTL).UnixMilli(),
		}
		return creds, &entities.AccountIdentity{EmailAddress: email}, nil
	}

	creds, identity, err := c.credentials.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read current Claude credentials "+
			"(run `claude` and `/login` first): %w", err)
	}
	if !creds.Valid() {
		return nil, nil, errors.New("current Claude credentials are incomplete; log in with `claude` first")
	}
	if accountEmail(identity) == "" {
		return nil, nil, errors.New("could not determine the account email; " +
			"ensure ~/.claude.json contains an oauthAccount block")
	}
	return creds, identity, nil
}

// upsertAccount inserts or updates the account for the given email, assigning a
// rotation order to new accounts and setting the first account as current. It
// returns the stored account so the caller can set enrollment-specific fields.
func upsertAccount(
	store *entities.Store,
	email string,
	identity *entities.AccountIdentity,
	creds *entities.OAuthCredentials,
) *entities.Account {
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
		return existing
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
	return &store.Accounts[len(store.Accounts)-1]
}
