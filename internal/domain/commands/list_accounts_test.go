package commands_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

// errUsageUnreachable is a plain failure used where the specific cause is
// irrelevant — anything that is not an authentication problem.
var errUsageUnreachable = errors.New("usage endpoint unreachable")

func TestListAccountsPersistsRefreshedCredentials(t *testing.T) {
	t.Parallel()

	t.Run("should save credentials minted while polling a backup account", func(t *testing.T) {
		t.Parallel()

		// given
		store := livePairStore()
		// The backup's access token is spent, so polling it has to refresh first.
		store.Accounts[1].Credentials.ExpiresAt = 1
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "b2", RefreshToken: "rb2", Scopes: loginScopes()},
		}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, tokens.RefreshCalls)
		assert.Positive(t, accounts.SaveCalls,
			"the rotated refresh token is lost unless the store is saved")

		backup := accounts.Store.FindAccount("b@example.com")
		require.NotNil(t, backup)
		assert.Equal(t, "b2", backup.Credentials.AccessToken)
		assert.Equal(t, "rb2", backup.Credentials.RefreshToken,
			"the server invalidated 'rb' when it minted 'rb2'")
	})

	t.Run("should not save when no poll refreshed anything", func(t *testing.T) {
		t.Parallel()

		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		tokens := &doubles.StubTokensRepository{}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		assert.Zero(t, tokens.RefreshCalls)
		assert.Zero(t, accounts.SaveCalls)
	})

	t.Run("should keep credentials refreshed before a failing usage call", func(t *testing.T) {
		t.Parallel()

		// given
		store := livePairStore()
		store.Accounts[1].Credentials.ExpiresAt = 1
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		// The refresh succeeds and rotates 'rb' server-side; the usage call after it
		// still fails, and the new pair must survive that.
		usage := &doubles.StubUsageRepository{
			ByToken:    map[string]*entities.Usage{"a": healthyUsage()},
			ErrByToken: map[string]error{"b2": errUsageUnreachable},
		}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "b2", RefreshToken: "rb2", Scopes: loginScopes()},
		}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		backup := accounts.Store.FindAccount("b@example.com")
		require.NotNil(t, backup)
		assert.Equal(t, "rb2", backup.Credentials.RefreshToken)
	})

	t.Run("should publish the active account's refreshed pair to the credentials store", func(t *testing.T) {
		t.Parallel()

		// given
		store := livePairStore()
		store.Accounts[0].Credentials.ExpiresAt = 1
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		// Claude Code still holds exactly the pair the refresh consumed.
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "a2", RefreshToken: "ra2", Scopes: loginScopes()},
		}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, credentials, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		require.NotNil(t, credentials.Written,
			"leaving Claude Code on the rotated-away token logs the session out")
		assert.Equal(t, "ra2", credentials.Written.RefreshToken)
	})
}

func TestListAccountsSkipsLongLivedAccounts(t *testing.T) {
	t.Parallel()

	t.Run("should never refresh or save an account enrolled from a long-lived token", func(t *testing.T) {
		t.Parallel()

		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: longLivedOnlyStore()}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		tokens := &doubles.StubTokensRepository{}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		assert.Zero(t, usage.FetchCalls)
		assert.Zero(t, tokens.RefreshCalls)
		assert.Zero(t, accounts.SaveCalls)
	})
}

func TestListAccountsRefreshesRejectedToken(t *testing.T) {
	t.Parallel()

	t.Run("should recover an account whose token was invalidated before its expiry", func(t *testing.T) {
		t.Parallel()

		// given
		store := &entities.Store{
			Accounts: []entities.Account{{
				Email: "a@example.com",
				Order: 0,
				// Far in the future: nothing about the timestamp suggests a refresh.
				Credentials: entities.OAuthCredentials{
					AccessToken:  "stale",
					RefreshToken: "ra",
					ExpiresAt:    farFuture,
					Scopes:       loginScopes(),
				},
			}},
			Rotation: entities.RotationState{CurrentEmail: "a@example.com"},
		}
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		usage := &doubles.StubUsageRepository{
			ErrByToken: map[string]error{
				"stale": fmt.Errorf("wrapped: %w", repositories.ErrUnauthorized),
			},
			ByToken: map[string]*entities.Usage{"fresh": healthyUsage()},
		}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "fresh", RefreshToken: "ra2", Scopes: loginScopes()},
		}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, tokens.RefreshCalls)
		assert.Equal(t, []string{"stale", "fresh"}, usage.Tokens)

		account := accounts.Store.FindAccount("a@example.com")
		require.NotNil(t, account)
		assert.Equal(t, "fresh", account.Credentials.AccessToken)
	})
}
