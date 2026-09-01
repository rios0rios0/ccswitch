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
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
		}
		usage := exhaustedPrimaryUsage()
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
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
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
			Creds: &entities.OAuthCredentials{AccessToken: "refreshed-a", RefreshToken: "ra", Scopes: loginScopes()},
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
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
		}
		usage := exhaustedPrimaryUsage()
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
			Creds: &entities.OAuthCredentials{AccessToken: "b", RefreshToken: "rb", Scopes: loginScopes()},
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
			Creds: &entities.OAuthCredentials{AccessToken: "b", RefreshToken: "rb", Scopes: loginScopes()},
		}
		usage := exhaustedPrimaryUsage()
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
			Creds: &entities.OAuthCredentials{AccessToken: "b", RefreshToken: "rb", Scopes: loginScopes()},
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
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
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
			Creds: &entities.OAuthCredentials{
				AccessToken:  "a-new",
				RefreshToken: "ra-rotated",
				Scopes:       loginScopes(),
			},
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

	t.Run("should publish refreshed credentials back to the credentials file", func(t *testing.T) {
		t.Parallel()
		// given: the stored access token has expired (the idle case), and the file
		// holds the very pair the refresh is about to consume
		accounts := &doubles.InMemoryAccountsRepository{Store: expiredPrimaryStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "a-new", RefreshToken: "ra-new", Scopes: loginScopes()},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, tokens, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then: leaving the rotated-away token on disk would log Claude Code out
		require.NoError(t, err)
		assert.Equal(t, 1, tokens.RefreshCalls)
		require.Equal(t, 1, credentials.WriteCalls, "the refreshed pair must reach the credentials file")
		assert.Equal(t, "a-new", credentials.Written.AccessToken)
		assert.Equal(t, "ra-new", credentials.Written.RefreshToken)
	})

	t.Run("should keep refreshed credentials when the usage call afterwards fails", func(t *testing.T) {
		t.Parallel()
		// given: the refresh succeeds -- rotating the token server-side, which cannot
		// be undone -- and only the usage call that follows it fails
		accounts := &doubles.InMemoryAccountsRepository{Store: expiredPrimaryStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "a-new", RefreshToken: "ra-new", Scopes: loginScopes()},
		}
		usage := &doubles.StubUsageRepository{Err: assert.AnError}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, tokens, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then: dropping the rotated pair would leave both the store and the file on
		// a token the server has already invalidated
		require.NoError(t, err)
		assert.Equal(t, 1, tokens.RefreshCalls)
		assert.Equal(t, "ra-new", accounts.Store.Accounts[0].Credentials.RefreshToken,
			"the rotated pair must survive in the store")
		require.Equal(t, 1, credentials.WriteCalls,
			"the rotated pair must still reach the credentials file")
		assert.Equal(t, "ra-new", credentials.Written.RefreshToken)
	})

	t.Run("should not write credentials when the refresh itself fails", func(t *testing.T) {
		t.Parallel()
		// given: nothing was rotated, so there is nothing new to persist or publish
		accounts := &doubles.InMemoryAccountsRepository{Store: expiredPrimaryStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		tokens := &doubles.StubTokensRepository{Err: assert.AnError}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, tokens, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, credentials.WriteCalls)
		assert.Equal(t, "ra", accounts.Store.Accounts[0].Credentials.RefreshToken)
	})

	t.Run("should publish refreshed credentials even while a claude session is running", func(t *testing.T) {
		t.Parallel()
		// given: a live session, which must not block refreshing the account it is
		// already using -- this is not an account switch
		accounts := &doubles.InMemoryAccountsRepository{Store: expiredPrimaryStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "a-new", RefreshToken: "ra-new", Scopes: loginScopes()},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(monitorConfig(), accounts, credentials, usage, tokens,
			&doubles.StubSessionsRepository{Running: true})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		require.Equal(t, 1, credentials.WriteCalls)
		assert.Equal(t, "ra-new", credentials.Written.RefreshToken)
	})

	t.Run("should not touch the credentials file when no refresh happened", func(t *testing.T) {
		t.Parallel()
		// given: an access token that is still valid, so no refresh is attempted
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken:  "a",
				RefreshToken: "ra",
				ExpiresAt:    time.Now().Add(time.Hour).UnixMilli(),
				Scopes:       loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		tokens := &doubles.StubTokensRepository{}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, tokens, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, tokens.RefreshCalls)
		assert.Equal(t, 0, credentials.WriteCalls)
	})

	t.Run("should not publish over a different account's installed credentials", func(t *testing.T) {
		t.Parallel()
		// given: a tick polls every account, so "a" is refreshed while the file holds
		// "b" -- the account the CLI is actually authenticating with. Round-robin
		// keeps "b" selected, so nothing else has a reason to write the file either.
		store := expiredPrimaryStore()
		store.Rotation.CurrentEmail = "b@example.com"
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "b", RefreshToken: "rb", ExpiresAt: farFuture,
				Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "b@example.com"},
		}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "a-new", RefreshToken: "ra-new", Scopes: loginScopes()},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			roundRobinConfig(), accounts, credentials, usage, tokens, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, tokens.RefreshCalls, "the backup is still refreshed")
		assert.Equal(t, 0, credentials.WriteCalls, "another account's credentials must be left alone")
	})

	t.Run("should capture rotated credentials for the current account with no identity", func(t *testing.T) {
		t.Parallel()
		// given: ~/.claude.json is missing, so nothing attributes the credentials,
		// and their refresh token has already rotated past the stored one
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken:  "a-new",
				RefreshToken: "ra-rotated",
				Scopes:       loginScopes(),
			},
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
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
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
		accounts := &doubles.InMemoryAccountsRepository{Store: expiredPrimaryStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
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
		assert.NotContains(t, usage.Tokens, "a", "the spent token must never reach the usage endpoint")
	})

	t.Run("should refresh a backup's spent token so rotating to it does not log out", func(t *testing.T) {
		t.Parallel()
		// given: the active account is fine while the idle backup's access token has
		// expired. A backup nobody touches goes stale in the store, and installing a
		// stale pair hands Claude Code a token the server has forgotten -- which
		// answers invalid_grant, which is the logout.
		store := livePairStore()
		store.Accounts[1].Credentials.ExpiresAt = 0
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "a", RefreshToken: "ra", ExpiresAt: farFuture,
				Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "b-new", RefreshToken: "rb-new", Scopes: loginScopes()},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, tokens, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, tokens.RefreshCalls, "the idle backup must be refreshed too")
		assert.Equal(t, "rb-new", accounts.Store.Accounts[1].Credentials.RefreshToken)
		assert.Equal(t, 0, credentials.WriteCalls,
			"refreshing a backup must not disturb the installed account")
	})

	t.Run("should release an exhausted account whose limits have actually reset", func(t *testing.T) {
		t.Parallel()
		// given: a marker recorded from an earlier poll, and a primary that polls
		// healthy now. The marker is a cache of the last poll, not a lease.
		now := time.Now()
		store := livePairStore()
		store.Rotation.CurrentEmail = "b@example.com"
		store.Rotation.MarkExhausted("a@example.com", now.Add(longRecovery))
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "b", RefreshToken: "rb", ExpiresAt: farFuture,
				Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "b@example.com"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(now)

		// then
		require.NoError(t, err)
		assert.False(t, accounts.Store.Rotation.IsExhausted("a@example.com", now))
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
	})

	t.Run("should not capture the blank credentials Claude Code writes on invalid_grant", func(t *testing.T) {
		t.Parallel()
		// given: Claude Code answered a dead refresh token by blanking the tokens in
		// place. Capturing that would replace the account's last good pair with the
		// marker saying it is gone, and flip it to long-lived so it is never polled.
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "", RefreshToken: "", ExpiresAt: 0},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, "a", accounts.Store.Accounts[0].Credentials.AccessToken)
		assert.Equal(t, "ra", accounts.Store.Accounts[0].Credentials.RefreshToken)
		assert.False(t, accounts.Store.Accounts[0].LongLived)
	})

	t.Run("should install a switch an earlier tick could only defer", func(t *testing.T) {
		t.Parallel()
		// given: a previous tick moved the pointer to "b" while a session held "a",
		// and the session has since exited. Nothing used to retry the write, so the
		// swap waited on the shell wrapper -- and never happened without it.
		store := livePairStore()
		store.Rotation.CurrentEmail = "b@example.com"
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "a", RefreshToken: "ra", ExpiresAt: farFuture,
				Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := perAccountUsage(map[string]*entities.Usage{
			"a": exhaustedUsage(),
			"b": healthyUsage(),
		})
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
		require.NotNil(t, credentials.Written)
		assert.Equal(t, "b", credentials.Written.AccessToken)
	})

	t.Run("should not install a deferred switch while a session is still running", func(t *testing.T) {
		t.Parallel()
		// given
		store := livePairStore()
		store.Rotation.CurrentEmail = "b@example.com"
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "a", RefreshToken: "ra", ExpiresAt: farFuture,
				Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := perAccountUsage(map[string]*entities.Usage{
			"a": exhaustedUsage(),
			"b": healthyUsage(),
		})
		command := commands.NewMonitorCommand(monitorConfig(), accounts, credentials, usage, nil,
			&doubles.StubSessionsRepository{Running: true})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, 0, credentials.WriteCalls)
	})

	t.Run("should apply the threshold stored by `ccswitch threshold` in flight", func(t *testing.T) {
		t.Parallel()
		// given: a daemon configured with the default threshold, and a store retuned
		// under it. The daemon reloads the store every tick precisely so this lands
		// without a restart.
		store := livePairStore()
		store.Settings.SetThreshold(fullPct)
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "a", RefreshToken: "ra", ExpiresAt: farFuture,
				Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := perAccountUsage(map[string]*entities.Usage{
			"a": borderlineUsage(),
			"b": healthyUsage(),
		})
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then: 95% is spent at the configured 90% but fine at the stored 100%
		require.NoError(t, err)
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
	})

	t.Run("should let an explicit --threshold outrank the stored one", func(t *testing.T) {
		t.Parallel()
		// given
		store := livePairStore()
		store.Settings.SetThreshold(fullPct)
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "a", RefreshToken: "ra", ExpiresAt: farFuture,
				Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := perAccountUsage(map[string]*entities.Usage{
			"a": borderlineUsage(),
			"b": healthyUsage(),
		})
		config := monitorConfig()
		config.ThresholdExplicit = true
		command := commands.NewMonitorCommand(
			config, accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
	})

	t.Run("should not repoll a backup seen moments ago", func(t *testing.T) {
		t.Parallel()
		// given: a backup polled just now, with a live token. The usage endpoint
		// rate-limits, so a backup is only re-read on the slow cadence.
		now := time.Now()
		store := livePairStore()
		store.Accounts[1].LastPolledAt = now.Add(-time.Minute)
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "a", RefreshToken: "ra", ExpiresAt: farFuture, Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(now)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"a"}, usage.Tokens, "only the active account is polled every tick")
	})

	t.Run("should repoll a backup once the slow cadence has elapsed", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := livePairStore()
		store.Accounts[1].LastPolledAt = now.Add(-time.Hour)
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "a", RefreshToken: "ra", ExpiresAt: farFuture, Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(now)

		// then
		require.NoError(t, err)
		assert.Contains(t, usage.Tokens, "b")
	})

	t.Run("should poll a recently seen backup anyway when its token is spent", func(t *testing.T) {
		t.Parallel()
		// given: a backup polled a minute ago whose access token has since expired.
		// It is the account the next rotation installs, so refreshing it cannot wait
		// for the slow cadence -- installing a stale pair is the logout.
		now := time.Now()
		store := livePairStore()
		store.Accounts[1].LastPolledAt = now.Add(-time.Minute)
		store.Accounts[1].Credentials.ExpiresAt = 0
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "a", RefreshToken: "ra", ExpiresAt: farFuture, Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{
				AccessToken: "b-new", RefreshToken: "rb-new", Scopes: loginScopes(),
			},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, tokens, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(now)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, tokens.RefreshCalls)
		assert.Equal(t, "rb-new", accounts.Store.Accounts[1].Credentials.RefreshToken)
	})

	t.Run("should advance the poll clock even when the poll failed", func(t *testing.T) {
		t.Parallel()
		// given: a backup whose usage call fails, which is what a 429 looks like.
		// LastPolledAt is the cadence's only input, so leaving it behind on failure
		// made a failing account the one polled on every tick -- the endpoint
		// pushing back being exactly the condition the cadence exists to survive.
		now := time.Now()
		store := livePairStore()
		store.Accounts[1].LastPolledAt = now.Add(-time.Hour)
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "a", RefreshToken: "ra", ExpiresAt: farFuture, Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := &doubles.StubUsageRepository{
			ByToken:    map[string]*entities.Usage{"a": healthyUsage()},
			ErrByToken: map[string]error{"b": assert.AnError},
		}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Tick(now)

		// then
		require.NoError(t, err)
		assert.Equal(t, now, accounts.Store.Accounts[1].LastPolledAt,
			"a failed attempt still counts against the cadence")
	})

	t.Run("should hold the cadence after a failed poll instead of retrying every tick", func(t *testing.T) {
		t.Parallel()
		// given: the tick above, followed by another one two minutes later
		now := time.Now()
		store := livePairStore()
		store.Accounts[1].LastPolledAt = now.Add(-time.Hour)
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "a", RefreshToken: "ra", ExpiresAt: farFuture, Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := &doubles.StubUsageRepository{
			ByToken:    map[string]*entities.Usage{"a": healthyUsage()},
			ErrByToken: map[string]error{"b": assert.AnError},
		}
		command := commands.NewMonitorCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		require.NoError(t, command.Tick(now))
		before := len(usage.Tokens)
		require.NoError(t, command.Tick(now.Add(2*time.Minute)))

		// then
		assert.NotContains(t, usage.Tokens[before:], "b",
			"the failing backup must wait out the cadence, not retry on the next tick")
	})
}
