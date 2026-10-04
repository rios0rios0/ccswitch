package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

func TestReorderAccountsCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("should put the named accounts first in the order given", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: threeAccountStore()}
		command := commands.NewReorderAccountsCommand(accounts)

		// when
		err := command.Execute([]string{"c@example.com", "b@example.com"})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, accounts.SaveCalls)
		assert.Equal(t, []string{"c@example.com", "b@example.com", "a@example.com"},
			emailsInOrder(accounts.Store))
	})

	t.Run("should not save when a named account is not enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: threeAccountStore()}
		command := commands.NewReorderAccountsCommand(accounts)

		// when
		err := command.Execute([]string{"c@example.com", "missing@example.com"})

		// then
		require.ErrorIs(t, err, entities.ErrAccountNotEnrolled)
		assert.Zero(t, accounts.SaveCalls)
	})
}
