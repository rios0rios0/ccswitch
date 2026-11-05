package commands_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

func TestUnpinAccountCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("should release the pin and hand over to the primary at once", func(t *testing.T) {
		t.Parallel()
		// given: the backup was picked by hand while the primary has capacity
		accounts := &doubles.InMemoryAccountsRepository{Store: pinnedBackupStore()}
		credentials := installedCreds("b", "b@example.com")
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		var out bytes.Buffer

		// when
		err := commands.NewUnpinAccountCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{},
		).WithOutput(&out).Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.False(t, accounts.Store.Rotation.Pinned())
		require.NotNil(t, credentials.Written)
		assert.Equal(t, "a", credentials.Written.AccessToken)
		assert.Equal(t, "[ccswitch] unpinned b@example.com\n[ccswitch] active account: a@example.com\n",
			out.String())
	})

	t.Run("should leave the switch for the next launch while a session is running", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: pinnedBackupStore()}
		credentials := installedCreds("b", "b@example.com")
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		var out bytes.Buffer

		// when
		err := commands.NewUnpinAccountCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{Active: true},
		).WithOutput(&out).Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Zero(t, credentials.WriteCalls, "a live claude session must not be swapped underneath")
		assert.Contains(t, out.String(),
			"[ccswitch] active account: a@example.com (installed on next claude launch; a session is running)\n")
	})

	t.Run("should keep the pinned account when the rotation policy selects it anyway", func(t *testing.T) {
		t.Parallel()
		// given: the primary itself was picked by hand
		store := livePairStore()
		store.Rotation.Pin("a@example.com")
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := installedCreds("a", "a@example.com")
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}

		// when
		err := commands.NewUnpinAccountCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{},
		).WithOutput(&bytes.Buffer{}).Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.False(t, accounts.Store.Rotation.Pinned())
		assert.Zero(t, credentials.WriteCalls)
	})

	t.Run("should hand a long-lived account back to a pollable one with capacity", func(t *testing.T) {
		t.Parallel()
		// given
		store := longLivedFallbackStore()
		store.Rotation.Pin("long@example.com")
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{Creds: &entities.OAuthCredentials{AccessToken: "long"}}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}

		// when
		err := commands.NewUnpinAccountCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{},
		).WithOutput(&bytes.Buffer{}).Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
		require.NotNil(t, credentials.Written)
		assert.Equal(t, "a", credentials.Written.AccessToken)
	})

	t.Run("should say so and touch nothing when no account is pinned", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		credentials := installedCreds("a", "a@example.com")
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		var out bytes.Buffer

		// when
		err := commands.NewUnpinAccountCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{},
		).WithOutput(&out).Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, "[ccswitch] no account is pinned\n", out.String())
		assert.Zero(t, accounts.SaveCalls)
		assert.Zero(t, usage.FetchCalls, "with nothing to release there is no policy to re-apply")
		assert.Zero(t, credentials.WriteCalls)
	})

	t.Run("should release the pins of every provider it is given, each among its own accounts",
		func(t *testing.T) {
			t.Parallel()
			// given: Codex's backup "y" was picked by hand as well, and both primaries are
			// healthy
			codexAccounts := codexStore()
			codexAccounts.Rotation.Pin("y@example.com")
			codex := codexPool(codexAccounts, &doubles.StubUsageRepository{
				Usage: codexUsage(lowPct, time.Now().Add(monthlyResetIn)),
			})
			accounts := &doubles.InMemoryAccountsRepository{Store: pinnedBackupStore()}
			var out bytes.Buffer

			// when
			err := commands.NewUnpinAccountCommand(
				monitorConfig(), accounts, installedCreds("b", "b@example.com"),
				&doubles.StubUsageRepository{Usage: healthyUsage()}, nil, &doubles.StubSessionsRepository{},
			).WithPool(codex).WithOutput(&out).Execute()

			// then
			require.NoError(t, err)
			assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
			assert.Equal(t, "x@example.com", codexAccounts.Rotation.CurrentEmail)
			assert.False(t, codexAccounts.Rotation.Pinned())
			assert.Contains(t, out.String(), "[ccswitch] active Claude account: a@example.com\n")
			assert.Contains(t, out.String(), "[ccswitch] active Codex account: x@example.com\n")
		})

	t.Run("should leave a provider that has no pin alone", func(t *testing.T) {
		t.Parallel()
		// given: only Claude's account was picked by hand, and Codex's active account is
		// exhausted -- which a tick of Codex's accounts would rotate away from
		codex := exhaustedCodexPool()
		accounts := &doubles.InMemoryAccountsRepository{Store: pinnedBackupStore()}

		// when
		err := commands.NewUnpinAccountCommand(
			monitorConfig(), accounts, installedCreds("b", "b@example.com"),
			&doubles.StubUsageRepository{Usage: healthyUsage()}, nil, &doubles.StubSessionsRepository{},
		).WithPool(codex).WithOutput(&bytes.Buffer{}).Execute()

		// then
		require.NoError(t, err)
		codexAccounts, ok := codex.Accounts.(*doubles.InMemoryAccountsRepository)
		require.True(t, ok)
		assert.Equal(t, "x@example.com", codexAccounts.Store.Rotation.CurrentEmail)
		assert.Zero(t, codexAccounts.SaveCalls)
	})
}
