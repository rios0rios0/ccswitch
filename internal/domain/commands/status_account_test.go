package commands_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

func TestStatusPersistsRefreshedCredentials(t *testing.T) {
	t.Parallel()

	t.Run("should save and publish the pair minted while reading usage", func(t *testing.T) {
		t.Parallel()

		// given
		store := livePairStore()
		store.Accounts[0].Credentials.ExpiresAt = 1
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "a2", RefreshToken: "ra2"},
		}

		// when
		err := commands.NewStatusCommand(
			monitorConfig(), accounts, credentials, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		assert.Positive(t, accounts.SaveCalls)

		current := accounts.Store.FindAccount("a@example.com")
		require.NotNil(t, current)
		assert.Equal(t, "ra2", current.Credentials.RefreshToken)

		require.NotNil(t, credentials.Written)
		assert.Equal(t, "ra2", credentials.Written.RefreshToken)
	})

	t.Run("should not save when the token was still good", func(t *testing.T) {
		t.Parallel()

		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		tokens := &doubles.StubTokensRepository{}

		// when
		err := commands.NewStatusCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		assert.Zero(t, tokens.RefreshCalls)
		assert.Zero(t, accounts.SaveCalls)
	})

	t.Run("should recover a token the server rejected before its expiry", func(t *testing.T) {
		t.Parallel()

		// given
		// The recorded expiry is far off, so nothing but the server's answer reveals
		// that the token is dead.
		store := livePairStore()
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		usage := &doubles.StubUsageRepository{
			ErrByToken: map[string]error{
				"a": fmt.Errorf("wrapped: %w", repositories.ErrUnauthorized),
			},
			ByToken: map[string]*entities.Usage{"a2": healthyUsage()},
		}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "a2", RefreshToken: "ra2"},
		}

		// when
		err := commands.NewStatusCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, tokens.RefreshCalls)
		assert.Equal(t, []string{"a", "a2"}, usage.Tokens)

		current := accounts.Store.FindAccount("a@example.com")
		require.NotNil(t, current)
		assert.Equal(t, "a2", current.Credentials.AccessToken)
	})

	t.Run("should not retry a rejection that follows a refresh it just made", func(t *testing.T) {
		t.Parallel()

		// given
		store := livePairStore()
		// Expired, so pollUsage refreshes up front. The freshly minted token is then
		// rejected too — spending the new refresh token again would gain nothing.
		store.Accounts[0].Credentials.ExpiresAt = 1
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		usage := &doubles.StubUsageRepository{
			ErrByToken: map[string]error{
				"a2": fmt.Errorf("wrapped: %w", repositories.ErrUnauthorized),
			},
		}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "a2", RefreshToken: "ra2"},
		}

		// when
		err := commands.NewStatusCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, tokens.RefreshCalls, "one refresh per poll at most")
		assert.Equal(t, []string{"a2"}, usage.Tokens)
	})
}
