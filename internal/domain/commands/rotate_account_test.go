package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

func TestRotateAccountCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("should rotate to the next healthy account", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{}
		sessions := &doubles.StubSessionsRepository{Running: false}
		command := commands.NewRotateAccountCommand(accounts, credentials, sessions)

		// when
		err := command.Execute(false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Equal(t, 1, credentials.WriteCalls)
	})

	t.Run("should defer the on-disk switch when a claude session is running", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{}
		sessions := &doubles.StubSessionsRepository{Running: true}
		command := commands.NewRotateAccountCommand(accounts, credentials, sessions)

		// when
		err := command.Execute(false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Equal(t, 0, credentials.WriteCalls)
	})

	t.Run("should return an error when fewer than two accounts are enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: &entities.Store{
			Accounts: []entities.Account{{Email: "a@example.com"}},
		}}
		command := commands.NewRotateAccountCommand(
			accounts, &doubles.StubCredentialsRepository{}, &doubles.StubSessionsRepository{})

		// when
		err := command.Execute(false)

		// then
		require.Error(t, err)
	})
}
