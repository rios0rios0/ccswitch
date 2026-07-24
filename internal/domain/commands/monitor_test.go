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

	t.Run("should switch back to the primary once its limits reset", func(t *testing.T) {
		t.Parallel()
		// given: running on the backup while the primary carries no exhaustion marker
		store := twoAccountStore()
		store.Rotation.CurrentEmail = "b@example.com"
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "b", RefreshToken: "rb"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Equal(t, 1, credentials.WriteCalls)
	})

	t.Run("should stay on the backup while the primary is still exhausted", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := twoAccountStore()
		store.Rotation.CurrentEmail = "b@example.com"
		store.Rotation.MarkExhausted("a@example.com", now.Add(longRecovery))
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "b", RefreshToken: "rb"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(now)

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Equal(t, 0, credentials.WriteCalls)
	})

	t.Run("should not return to the primary when prefer-primary is disabled", func(t *testing.T) {
		t.Parallel()
		// given
		store := twoAccountStore()
		store.Rotation.CurrentEmail = "b@example.com"
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "b", RefreshToken: "rb"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			roundRobinConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Equal(t, 0, credentials.WriteCalls)
	})

	t.Run("should hold the account until its longest exhausted limit resets", func(t *testing.T) {
		t.Parallel()
		// given: a short session window alongside a saturated weekly window
		now := time.Now()
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra"},
		}
		usage := &doubles.StubUsageRepository{Usage: &entities.Usage{Limits: []entities.Limit{
			{
				Kind:     "session",
				Percent:  lowPct,
				Severity: entities.SeverityNormal,
				IsActive: true,
				ResetsAt: now.Add(time.Hour),
			},
			{
				Kind:     "weekly_scoped",
				Percent:  fullPct,
				Severity: entities.SeverityCritical,
				IsActive: true,
				ResetsAt: now.Add(longRecovery),
			},
		}}}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(now)

		// then: still held well past the session reset, so it cannot flap back
		require.NoError(t, err)
		assert.True(t, accounts.Store.Rotation.IsExhausted("a@example.com", now.Add(time.Hour+time.Minute)))
	})

	t.Run("should capture credentials whose refresh token was rotated on disk", func(t *testing.T) {
		t.Parallel()
		// given: disk carries a refreshed pair whose refresh token no longer equals
		// the stored one, so only the identity can tie it back to the account
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "a-new", RefreshToken: "ra-rotated"},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then: the store follows the rotation instead of freezing on a dead token
		require.NoError(t, err)
		assert.Equal(t, "a-new", accounts.Store.Accounts[0].Credentials.AccessToken)
		assert.Equal(t, "ra-rotated", accounts.Store.Accounts[0].Credentials.RefreshToken)
	})

	t.Run("should capture rotated credentials for the current account with no identity", func(t *testing.T) {
		t.Parallel()
		// given: ~/.claude.json is missing, so nothing attributes the credentials,
		// and their refresh token has already rotated past the stored one
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "a-new", RefreshToken: "ra-rotated"},
			Identity: nil,
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then: skipping the capture would pin the store to the rotated-away token
		require.NoError(t, err)
		assert.Equal(t, "a-new", accounts.Store.Accounts[0].Credentials.AccessToken)
		assert.Equal(t, "ra-rotated", accounts.Store.Accounts[0].Credentials.RefreshToken)
	})

	t.Run("should not poll usage for an account enrolled from a long-lived token", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: longLivedOnlyStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "long"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then: polling it would 403, so it is skipped and the account is kept
		require.NoError(t, err)
		assert.Equal(t, 0, usage.FetchCalls)
		assert.Equal(t, "long@example.com", accounts.Store.Rotation.CurrentEmail)
	})

	t.Run("should leave a long-lived fallback once a pollable primary is available", func(t *testing.T) {
		t.Parallel()
		// given: sitting on the manually selected long-lived account
		store := longLivedFallbackStore()
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "long"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Equal(t, 1, credentials.WriteCalls)
	})

	t.Run("should not rotate onto a long-lived account when the primary is exhausted", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := longLivedFallbackStore()
		store.Rotation.CurrentEmail = "a@example.com"
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra"},
		}
		usage := &doubles.StubUsageRepository{Usage: exhaustedUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(now)

		// then: capacity cannot be verified for the long-lived account, so it is not
		// selected automatically; the user picks it with `ccswitch use`
		require.NoError(t, err)
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Equal(t, 0, credentials.WriteCalls)
		assert.True(t, accounts.Store.Rotation.IsExhausted("a@example.com", now))
	})

	t.Run("should not poll usage with a stale token when the refresh call fails", func(t *testing.T) {
		t.Parallel()
		// given: expired (zero ExpiresAt) credentials and a refresh call that fails,
		// e.g. because the refresh token itself was revoked or expired
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		tokens := &doubles.StubTokensRepository{Err: assert.AnError}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, tokens, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then: the tick logs the failure and retries next interval, but must never
		// call the usage endpoint with the known-stale token
		require.NoError(t, err)
		assert.Equal(t, 1, tokens.RefreshCalls)
		assert.Equal(t, 0, usage.FetchCalls)
	})
}
