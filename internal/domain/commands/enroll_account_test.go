package commands_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

func TestEnrollAccountCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("should enroll a new account and set it current when the store is empty", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    validCreds(),
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com", AccountUUID: "uuid-a"},
		}
		command := commands.NewEnrollAccountCommand(accounts, credentials)

		// when
		err := command.Execute("", "")

		// then
		require.NoError(t, err)
		require.Len(t, accounts.Store.Accounts, 1)
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
	})

	t.Run("should update credentials when re-enrolling an existing account", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: &entities.Store{
			Accounts: []entities.Account{{
				Email:       "a@example.com",
				Credentials: entities.OAuthCredentials{AccessToken: "old", RefreshToken: "r"},
			}},
			Rotation: entities.RotationState{CurrentEmail: "a@example.com"},
		}}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "new", RefreshToken: "r"},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		command := commands.NewEnrollAccountCommand(accounts, credentials)

		// when
		err := command.Execute("", "")

		// then
		require.NoError(t, err)
		require.Len(t, accounts.Store.Accounts, 1)
		assert.Equal(t, "new", accounts.Store.Accounts[0].Credentials.AccessToken)
	})

	t.Run("should return an error when reading Claude credentials fails", func(t *testing.T) {
		t.Parallel()
		// given
		credentials := &doubles.StubCredentialsRepository{ReadErr: assert.AnError}
		command := commands.NewEnrollAccountCommand(&doubles.InMemoryAccountsRepository{}, credentials)

		// when
		err := command.Execute("", "")

		// then
		require.Error(t, err)
	})

	t.Run("should return an error when credentials are incomplete", func(t *testing.T) {
		t.Parallel()
		// given
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "only-access"},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		command := commands.NewEnrollAccountCommand(&doubles.InMemoryAccountsRepository{}, credentials)

		// when
		err := command.Execute("", "")

		// then
		require.Error(t, err)
	})

	t.Run("should return an error when the account email is unknown", func(t *testing.T) {
		t.Parallel()
		// given
		credentials := &doubles.StubCredentialsRepository{Creds: validCreds(), Identity: nil}
		command := commands.NewEnrollAccountCommand(&doubles.InMemoryAccountsRepository{}, credentials)

		// when
		err := command.Execute("", "")

		// then
		require.Error(t, err)
	})

	t.Run("should enroll a long-lived token directly without reading the credentials file", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{}
		credentials := &doubles.StubCredentialsRepository{ReadErr: assert.AnError}
		command := commands.NewEnrollAccountCommand(accounts, credentials)
		before := time.Now()

		// when
		err := command.Execute("setup-token-value", "a@example.com")

		// then
		require.NoError(t, err)
		require.Len(t, accounts.Store.Accounts, 1)
		account := accounts.Store.Accounts[0]
		assert.Equal(t, "a@example.com", account.Email)
		assert.Equal(t, "setup-token-value", account.Credentials.AccessToken)
		assert.Empty(t, account.Credentials.RefreshToken)
		assert.Greater(t, account.Credentials.ExpiresAt, before.UnixMilli())
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.True(t, account.LongLived, "the token lacks the scope the usage endpoint needs")
		assert.False(t, account.SupportsUsagePolling())
	})

	t.Run("should mark an account enrolled from the credentials file as pollable", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    validCreds(),
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		command := commands.NewEnrollAccountCommand(accounts, credentials)

		// when
		err := command.Execute("", "")

		// then
		require.NoError(t, err)
		assert.True(t, accounts.Store.Accounts[0].SupportsUsagePolling())
	})

	t.Run("should return an error when --token is given without --email", func(t *testing.T) {
		t.Parallel()
		// given
		command := commands.NewEnrollAccountCommand(
			&doubles.InMemoryAccountsRepository{}, &doubles.StubCredentialsRepository{})

		// when
		err := command.Execute("setup-token-value", "")

		// then
		require.Error(t, err)
	})
}
