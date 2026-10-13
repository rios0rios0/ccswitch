package commands_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

func TestCheckCapacityCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("should return nil and print nothing while an account has capacity", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		var out bytes.Buffer
		command := commands.NewCheckCapacityCommand(accounts).WithOutput(&out)

		// when
		err := command.Execute(false)

		// then
		require.NoError(t, err)
		assert.Empty(t, out.String())
	})

	t.Run("should return ErrNoCapacity and name the account that recovers first", func(t *testing.T) {
		t.Parallel()
		// given
		store := twoAccountStore()
		recovers := time.Now().Add(recoveryIn)
		store.Rotation.MarkExhausted("a@example.com", recovers.Add(time.Hour))
		store.Rotation.MarkExhausted("b@example.com", recovers)
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		var out bytes.Buffer
		command := commands.NewCheckCapacityCommand(accounts).WithOutput(&out)

		// when
		err := command.Execute(false)

		// then
		require.ErrorIs(t, err, commands.ErrNoCapacity)
		assert.Contains(t, out.String(), "no enrolled account has capacity")
		assert.Contains(t, out.String(), "b@example.com recovers first, in 31m")
		assert.Contains(t, out.String(), momentIn(recovers))
	})

	t.Run("should print nothing when quiet and still return ErrNoCapacity", func(t *testing.T) {
		t.Parallel()
		// given
		store := twoAccountStore()
		store.Rotation.MarkExhausted("a@example.com", time.Now().Add(time.Hour))
		store.Rotation.MarkExhausted("b@example.com", time.Now().Add(time.Hour))
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		var out bytes.Buffer
		command := commands.NewCheckCapacityCommand(accounts).WithOutput(&out)

		// when
		err := command.Execute(true)

		// then
		require.ErrorIs(t, err, commands.ErrNoCapacity)
		assert.Empty(t, out.String())
	})

	t.Run("should return the error when the store cannot be read", func(t *testing.T) {
		t.Parallel()
		// given
		loadErr := errors.New("store unreadable")
		accounts := &doubles.InMemoryAccountsRepository{LoadErr: loadErr}
		var out bytes.Buffer
		command := commands.NewCheckCapacityCommand(accounts).WithOutput(&out)

		// when
		err := command.Execute(false)

		// then
		require.ErrorIs(t, err, loadErr)
		require.NotErrorIs(t, err, commands.ErrNoCapacity)
		assert.Empty(t, out.String())
	})
}
