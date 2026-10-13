package controllers_test

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/controllers"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

// exhaustedStorePath persists two pollable accounts that are both exhausted, the
// primary recovering first, and returns the store's path.
func exhaustedStorePath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "store.json")
	recovers := time.Now().Add(time.Hour)
	store := &entities.Store{
		Accounts: []entities.Account{
			{
				Email:       primaryEmail,
				Order:       0,
				Credentials: entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra"},
			},
			{
				Email:       backupEmail,
				Order:       1,
				Credentials: entities.OAuthCredentials{AccessToken: "b", RefreshToken: "rb"},
			},
		},
		Rotation: entities.RotationState{CurrentEmail: primaryEmail},
	}
	store.Rotation.MarkExhausted(primaryEmail, recovers)
	store.Rotation.MarkExhausted(backupEmail, recovers.Add(time.Hour))
	require.NoError(t, repositories.NewJSONAccountsRepository(path).Save(store))
	return path
}

func TestCapacityCLI(t *testing.T) {
	t.Parallel()

	t.Run("should exit 3 and name the account that recovers first when no account has capacity", func(t *testing.T) {
		t.Parallel()
		// given
		path := exhaustedStorePath(t)
		var out, errOut bytes.Buffer

		// when
		err := executeCLIWith(path, &doubles.StubSelfUpdateRepository{}, &out, &errOut, "capacity")

		// then
		require.ErrorIs(t, err, commands.ErrNoCapacity)
		assert.Equal(t, 3, controllers.ExitCode(err))
		assert.Contains(t, out.String(), primaryEmail+" recovers first")
		assert.Empty(t, errOut.String(), "the answer is not a failure for cobra to report")
	})

	t.Run("should answer with the exit status alone when quiet", func(t *testing.T) {
		t.Parallel()
		// given
		path := exhaustedStorePath(t)
		var out, errOut bytes.Buffer

		// when
		err := executeCLIWith(path, &doubles.StubSelfUpdateRepository{}, &out, &errOut, "capacity", "--quiet")

		// then
		require.ErrorIs(t, err, commands.ErrNoCapacity)
		assert.Empty(t, out.String())
		assert.Empty(t, errOut.String())
	})

	t.Run("should exit 0 and print nothing while an account has capacity", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		enrollFixture(t, path, primaryEmail)
		var out, errOut bytes.Buffer

		// when
		err := executeCLIWith(path, &doubles.StubSelfUpdateRepository{}, &out, &errOut, "capacity")

		// then
		require.NoError(t, err)
		assert.Empty(t, out.String())
		assert.Empty(t, errOut.String())
	})
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	t.Run("should exit 3 for the no-capacity answer, even when wrapped", func(t *testing.T) {
		t.Parallel()
		// given
		err := fmt.Errorf("checking: %w", commands.ErrNoCapacity)

		// when
		code := controllers.ExitCode(err)

		// then
		assert.Equal(t, 3, code)
	})

	t.Run("should exit 1 for any failure", func(t *testing.T) {
		t.Parallel()
		// given
		err := errors.New("store unreadable")

		// when
		code := controllers.ExitCode(err)

		// then
		assert.Equal(t, 1, code)
	})
}
