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

func TestMonitorCommandTick(t *testing.T) {
	t.Parallel()

	t.Run("should rotate to a backup when the current account is exhausted", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra"},
		}
		usage := &doubles.StubUsageRepository{Usage: exhaustedUsage()}
		sessions := &doubles.StubSessionsRepository{Running: false}
		command := commands.NewMonitorCommand(monitorConfig(), accounts, credentials, usage, nil, sessions)

		// when
		err := command.Tick(now)

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.True(t, accounts.Store.Rotation.IsExhausted("a@example.com", now))
	})

	t.Run("should not rotate when the current account is healthy", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
	})

	t.Run("should capture refreshed on-disk credentials into the matching account", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "refreshed-a", RefreshToken: "ra"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, "refreshed-a", accounts.Store.Accounts[0].Credentials.AccessToken)
	})

	t.Run("should defer the switch when exhausted but a claude session is running", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra"},
		}
		usage := &doubles.StubUsageRepository{Usage: exhaustedUsage()}
		sessions := &doubles.StubSessionsRepository{Running: true}
		command := commands.NewMonitorCommand(monitorConfig(), accounts, credentials, usage, nil, sessions)

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Equal(t, 0, credentials.WriteCalls)
	})
}
