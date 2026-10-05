package commands_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

func TestSelfUpdateCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("should report the release installed when the channel installed one", func(t *testing.T) {
		t.Parallel()
		// given
		updates := &doubles.StubSelfUpdateRepository{Installed: true}
		command := commands.NewSelfUpdateCommand(updates)

		// when
		installed, err := command.Execute(false, true)

		// then
		require.NoError(t, err)
		assert.True(t, installed)
		assert.Equal(t, 1, updates.UpdateCalls)
		assert.False(t, updates.DryRun)
		assert.True(t, updates.Force)
	})

	t.Run("should report nothing installed when the channel installed nothing", func(t *testing.T) {
		t.Parallel()
		// given
		updates := &doubles.StubSelfUpdateRepository{}
		command := commands.NewSelfUpdateCommand(updates)

		// when
		installed, err := command.Execute(true, false)

		// then
		require.NoError(t, err)
		assert.False(t, installed)
		assert.True(t, updates.DryRun)
		assert.False(t, updates.Force)
	})

	t.Run("should return an error when the channel fails", func(t *testing.T) {
		t.Parallel()
		// given
		failure := errors.New("fixture release failure")
		command := commands.NewSelfUpdateCommand(&doubles.StubSelfUpdateRepository{UpdateErr: failure})

		// when
		installed, err := command.Execute(false, true)

		// then
		require.ErrorIs(t, err, failure)
		assert.False(t, installed)
	})
}
