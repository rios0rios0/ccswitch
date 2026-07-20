package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

func TestUseAccountCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("should switch to the named account", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{}
		command := commands.NewUseAccountCommand(accounts, credentials)

		// when
		err := command.Execute("b@example.com")

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Equal(t, 1, credentials.WriteCalls)
	})

	t.Run("should return an error when the account is not enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		command := commands.NewUseAccountCommand(accounts, &doubles.StubCredentialsRepository{})

		// when
		err := command.Execute("missing@example.com")

		// then
		require.Error(t, err)
	})
}
