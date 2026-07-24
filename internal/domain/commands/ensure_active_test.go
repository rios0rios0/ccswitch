package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

func TestEnsureActiveCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("should install the current account when on-disk credentials differ", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "b", RefreshToken: "rb"},
		}
		command := commands.NewEnsureActiveCommand(accounts, credentials)

		// when
		err := command.Execute(true)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, credentials.WriteCalls)
		require.NotNil(t, credentials.Written)
		assert.Equal(t, "ra", credentials.Written.RefreshToken)
	})

	t.Run("should be a no-op when the current account is already active", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra"},
		}
		command := commands.NewEnsureActiveCommand(accounts, credentials)

		// when
		err := command.Execute(true)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, credentials.WriteCalls)
	})

	t.Run("should not overwrite credentials Claude Code refreshed for the same account", func(t *testing.T) {
		t.Parallel()
		// given: the store still holds the pre-rotation refresh token for the current
		// account, while disk carries the freshly refreshed pair for that same account
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "a-new", RefreshToken: "ra-rotated"},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		command := commands.NewEnsureActiveCommand(accounts, credentials)

		// when
		err := command.Execute(true)

		// then: the fresh credentials survive and are folded back into the store,
		// instead of being replaced by the stale stored ones
		require.NoError(t, err)
		assert.Equal(t, 0, credentials.WriteCalls, "must not clobber refreshed credentials")
		assert.Equal(t, "a-new", accounts.Store.Accounts[0].Credentials.AccessToken)
		assert.Equal(t, "ra-rotated", accounts.Store.Accounts[0].Credentials.RefreshToken)
	})

	t.Run("should not overwrite rotated credentials when no identity is available", func(t *testing.T) {
		t.Parallel()
		// given: ~/.claude.json is missing, so the credentials cannot be attributed,
		// and their refresh token has already rotated past the stored one
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "a-new", RefreshToken: "ra-rotated"},
			Identity: nil,
		}
		command := commands.NewEnsureActiveCommand(accounts, credentials)

		// when
		err := command.Execute(true)

		// then: guessing would destroy the pair Claude Code just refreshed
		require.NoError(t, err)
		assert.Equal(t, 0, credentials.WriteCalls,
			"unattributable credentials must never be overwritten on a guess")
	})

	t.Run("should still install the current account when another account is recognized", func(t *testing.T) {
		t.Parallel()
		// given: no identity, but the refresh token still identifies the backup, so
		// the installed credentials are known to belong to a different account
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "b", RefreshToken: "rb"},
			Identity: nil,
		}
		command := commands.NewEnsureActiveCommand(accounts, credentials)

		// when
		err := command.Execute(true)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, credentials.WriteCalls)
		require.NotNil(t, credentials.Written)
		assert.Equal(t, "ra", credentials.Written.RefreshToken)
	})

	t.Run("should be a no-op when nothing is enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{}
		credentials := &doubles.StubCredentialsRepository{}
		command := commands.NewEnsureActiveCommand(accounts, credentials)

		// when
		err := command.Execute(true)

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, credentials.WriteCalls)
	})
}
