package repositories_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

// subscriptionCreds returns a credential set shaped like a real Claude Code
// subscription login, i.e. one carrying every field the token endpoint omits.
func subscriptionCreds(refreshToken string) entities.OAuthCredentials {
	return entities.OAuthCredentials{
		AccessToken:           "old-access",
		RefreshToken:          refreshToken,
		RefreshTokenExpiresAt: 1790689141292,
		Scopes: []string{
			"user:profile", "user:inference", "user:sessions:claude_code",
			"user:mcp_servers", "user:file_upload",
		},
		SubscriptionType: "max",
		RateLimitTier:    "default_claude_max_20x",
		ExpiresAt:        1788309265292,
	}
}

func TestAnthropicTokensRepositoryRefresh(t *testing.T) {
	t.Parallel()

	t.Run("should mint credentials from a refresh token", func(t *testing.T) {
		t.Parallel()
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), "old-refresh")
			assert.Contains(t, string(body), "client-xyz")
			_, _ = io.WriteString(w,
				`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`)
		}))
		defer server.Close()
		repo := repositories.NewAnthropicTokensRepository(server.URL, "client-xyz", server.Client())

		// when
		creds, err := repo.Refresh(subscriptionCreds("old-refresh"))

		// then
		require.NoError(t, err)
		assert.Equal(t, "new-access", creds.AccessToken)
		assert.Equal(t, "new-refresh", creds.RefreshToken)
		assert.Positive(t, creds.ExpiresAt)
	})

	t.Run("should carry the prior refresh token forward when the server omits it", func(t *testing.T) {
		t.Parallel()
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"access_token":"new-access","expires_in":3600}`)
		}))
		defer server.Close()
		repo := repositories.NewAnthropicTokensRepository(server.URL, "client", server.Client())

		// when
		creds, err := repo.Refresh(subscriptionCreds("kept-refresh"))

		// then
		require.NoError(t, err)
		assert.Equal(t, "kept-refresh", creds.RefreshToken)
	})

	t.Run("should request the scopes the account already carries", func(t *testing.T) {
		t.Parallel()
		// given: the endpoint mints a token for the scopes the request names, so a
		// refresh that names none can hand back a narrower token than it replaced
		var received string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			received = string(body)
			_, _ = io.WriteString(w, `{"access_token":"new-access","expires_in":3600}`)
		}))
		defer server.Close()
		repo := repositories.NewAnthropicTokensRepository(server.URL, "client", server.Client())

		// when
		_, err := repo.Refresh(subscriptionCreds("refresh"))

		// then
		require.NoError(t, err)
		assert.Contains(t, received,
			`"scope":"user:profile user:inference user:sessions:claude_code `+
				`user:mcp_servers user:file_upload"`)
	})

	t.Run("should request Claude Code's scopes when the account has none recorded", func(t *testing.T) {
		t.Parallel()
		// given: an account enrolled before scopes were persisted
		var received string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			received = string(body)
			_, _ = io.WriteString(w, `{"access_token":"new-access","expires_in":3600}`)
		}))
		defer server.Close()
		repo := repositories.NewAnthropicTokensRepository(server.URL, "client", server.Client())

		// when
		_, err := repo.Refresh(entities.OAuthCredentials{RefreshToken: "refresh"})

		// then
		require.NoError(t, err)
		assert.Contains(t, received, `"scope":"user:profile user:inference`)
	})

	t.Run("should adopt the scopes the endpoint reports", func(t *testing.T) {
		t.Parallel()
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w,
				`{"access_token":"new-access","expires_in":3600,`+
					`"scope":"user:profile user:inference","refresh_token_expires_in":600}`)
		}))
		defer server.Close()
		repo := repositories.NewAnthropicTokensRepository(server.URL, "client", server.Client())

		// when
		creds, err := repo.Refresh(subscriptionCreds("refresh"))

		// then
		assert.Equal(t, []string{"user:profile", "user:inference"}, creds.Scopes)
		require.NoError(t, err)
		assert.Positive(t, creds.RefreshTokenExpiresAt)
		assert.NotEqual(t, int64(1790689141292), creds.RefreshTokenExpiresAt)
	})

	t.Run("should keep the fields the endpoint does not restate", func(t *testing.T) {
		t.Parallel()
		// given: the response the real endpoint sends -- tokens and nothing else.
		// Rebuilding credentials from it drops the scopes, and Claude Code discards
		// its own refresh of a credential set whose scopes omit `user:inference`,
		// which strands the pair on disk until it 401s and the user is logged out.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w,
				`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`)
		}))
		defer server.Close()
		repo := repositories.NewAnthropicTokensRepository(server.URL, "client", server.Client())
		prior := subscriptionCreds("old-refresh")

		// when
		creds, err := repo.Refresh(prior)

		// then
		require.NoError(t, err)
		assert.Equal(t, prior.Scopes, creds.Scopes)
		assert.Equal(t, prior.SubscriptionType, creds.SubscriptionType)
		assert.Equal(t, prior.RateLimitTier, creds.RateLimitTier)
		assert.Equal(t, prior.RefreshTokenExpiresAt, creds.RefreshTokenExpiresAt)
	})
}

func TestAnthropicTokensRepositoryRepairsDegradedCredentials(t *testing.T) {
	t.Parallel()

	t.Run("should record the requested scopes when nothing else names any", func(t *testing.T) {
		t.Parallel()
		// given: an account an earlier ccswitch stripped of its scopes, refreshed
		// against an endpoint that does not echo the scope back. Leaving the scopes
		// empty would keep the account looking degraded and refresh it every poll.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w,
				`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`)
		}))
		defer server.Close()
		repo := repositories.NewAnthropicTokensRepository(server.URL, "client", server.Client())

		// when
		creds, err := repo.Refresh(entities.OAuthCredentials{RefreshToken: "stripped"})

		// then
		require.NoError(t, err)
		assert.Contains(t, creds.Scopes, "user:inference")
		assert.False(t, creds.Degraded(), "one refresh must be enough to repair the account")
	})
}
