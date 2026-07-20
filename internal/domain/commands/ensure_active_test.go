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
