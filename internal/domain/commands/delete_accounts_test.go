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

func TestDeleteAccountsCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("should remove a backup without touching the installed credentials", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: threeAccountStore()}
		credentials := &doubles.StubCredentialsRepository{}
		command := commands.NewDeleteAccountsCommand(accounts, credentials, &doubles.StubSessionsRepository{})

		// when
		err := command.Execute([]string{"b@example.com"})

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"a@example.com", "c@example.com"}, emailsInOrder(accounts.Store))
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Zero(t, credentials.WriteCalls)
	})

	t.Run("should hand the active account over to the best one with capacity", func(t *testing.T) {
		t.Parallel()
		// given: "b" outranks "c" but is exhausted
		store := threeAccountStore()
		store.Rotation.MarkExhausted("b@example.com", time.Now().Add(time.Hour))
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{}
		command := commands.NewDeleteAccountsCommand(accounts, credentials, &doubles.StubSessionsRepository{})

		// when
		err := command.Execute([]string{"a@example.com"})

		// then
		require.NoError(t, err)
		assert.Equal(t, "c@example.com", accounts.Store.Rotation.CurrentEmail)
		require.NotNil(t, credentials.Written)
		assert.Equal(t, "rc", credentials.Written.RefreshToken)
	})

	t.Run("should defer installing the successor while a claude session is running", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: threeAccountStore()}
		credentials := &doubles.StubCredentialsRepository{}
		sessions := &doubles.StubSessionsRepository{Running: true}
		command := commands.NewDeleteAccountsCommand(accounts, credentials, sessions)

		// when
		err := command.Execute([]string{"a@example.com"})

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Zero(t, credentials.WriteCalls, "a running session must never be swapped underneath")
	})

	t.Run("should save the hand-over even when installing the successor fails", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: threeAccountStore()}
		credentials := &doubles.StubCredentialsRepository{WriteErr: assert.AnError}
		command := commands.NewDeleteAccountsCommand(accounts, credentials, &doubles.StubSessionsRepository{})

		// when
		err := command.Execute([]string{"a@example.com"})

		// then: `ensure` or the monitor can finish the switch from the saved store
		require.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, 1, accounts.SaveCalls)
		assert.Nil(t, accounts.Store.FindAccount("a@example.com"))
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
	})

	t.Run("should remove nothing when any named account is not enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: threeAccountStore()}
		command := commands.NewDeleteAccountsCommand(
			accounts, &doubles.StubCredentialsRepository{}, &doubles.StubSessionsRepository{})

		// when
		err := command.Execute([]string{"b@example.com", "missing@example.com"})

		// then
		require.ErrorIs(t, err, entities.ErrAccountNotEnrolled)
		assert.Zero(t, accounts.SaveCalls)
		assert.Len(t, accounts.Store.Accounts, 3)
	})

	t.Run("should not select a long-lived account when no other account remains", func(t *testing.T) {
		t.Parallel()
		// given
		store := longLivedFallbackStore()
		store.Rotation.CurrentEmail = "a@example.com"
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{}
		command := commands.NewDeleteAccountsCommand(accounts, credentials, &doubles.StubSessionsRepository{})

		// when
		err := command.Execute([]string{"a@example.com"})

		// then
		require.NoError(t, err)
		assert.Empty(t, accounts.Store.Rotation.CurrentEmail,
			"a long-lived account is only ever selected with `ccswitch use`")
		assert.Zero(t, credentials.WriteCalls)
	})

	t.Run("should clear the current account when the last one is removed", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		credentials := &doubles.StubCredentialsRepository{}
		command := commands.NewDeleteAccountsCommand(accounts, credentials, &doubles.StubSessionsRepository{})

		// when
		err := command.Execute([]string{"a@example.com", "b@example.com"})

		// then
		require.NoError(t, err)
		assert.Empty(t, accounts.Store.Accounts)
		assert.Empty(t, accounts.Store.Rotation.CurrentEmail)
		assert.Zero(t, credentials.WriteCalls)
	})
}
