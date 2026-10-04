package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

// priorityOf parses a priority the test knows to be valid.
func priorityOf(t *testing.T, spec string) entities.Priority {
	t.Helper()
	priority, err := entities.ParsePriority(spec)
	require.NoError(t, err)
	return priority
}

func TestUpdateAccountCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("should move the account to the place the priority names", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: threeAccountStore()}
		command := commands.NewUpdateAccountCommand(accounts)

		// when
		err := command.Execute("c@example.com", priorityOf(t, entities.PriorityTop))

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, accounts.SaveCalls)
		assert.Equal(t, []string{"c@example.com", "a@example.com", "b@example.com"},
			emailsInOrder(accounts.Store))
	})

	t.Run("should leave the current account alone, since the monitor applies the new order", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: threeAccountStore()}
		command := commands.NewUpdateAccountCommand(accounts)

		// when
		err := command.Execute("a@example.com", priorityOf(t, entities.PriorityBottom))

		// then
		require.NoError(t, err)
		assert.Equal(t, 3, accounts.Store.Position("a@example.com"))
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
	})

	t.Run("should not save when the account is already in that place", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: threeAccountStore()}
		command := commands.NewUpdateAccountCommand(accounts)

		// when
		err := command.Execute("a@example.com", priorityOf(t, entities.PriorityUp))

		// then
		require.NoError(t, err)
		assert.Zero(t, accounts.SaveCalls)
	})

	t.Run("should return an error when the account is not enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: threeAccountStore()}
		command := commands.NewUpdateAccountCommand(accounts)

		// when
		err := command.Execute("missing@example.com", priorityOf(t, entities.PriorityTop))

		// then
		require.ErrorIs(t, err, entities.ErrAccountNotEnrolled)
		assert.Zero(t, accounts.SaveCalls)
	})

	t.Run("should return an error when the position is past the last account", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: threeAccountStore()}
		command := commands.NewUpdateAccountCommand(accounts)

		// when
		err := command.Execute("a@example.com", priorityOf(t, "4"))

		// then
		require.ErrorIs(t, err, entities.ErrInvalidPriority)
		assert.Zero(t, accounts.SaveCalls)
	})
}
