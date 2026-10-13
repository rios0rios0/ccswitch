package repositories_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
	"github.com/rios0rios0/ccswitch/test/builders"
)

const codexClientID = "fixture-client-id"

// codexSignInCreds returns the credentials of a Codex sign-in about to be refreshed.
func codexSignInCreds() entities.OAuthCredentials {
	return entities.OAuthCredentials{
		AccessToken:      "old-access",
		RefreshToken:     "old-refresh",
		IDToken:          "old-id",
		AccountID:        codexAccountID,
		SubscriptionType: codexPlan,
	}
}

// serveTokens starts a token endpoint that answers every refresh with the body, and
// hands the decoded request to the channel.
func serveTokens(t *testing.T, status int, body string, requests chan<- map[string]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var request map[string]string
		_ = json.Unmarshal(data, &request)
		request["content-type"] = r.Header.Get("Content-Type")
		requests <- request
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

// tokenResponse encodes a refresh response.
func tokenResponse(t *testing.T, fields map[string]any) string {
	t.Helper()
	data, err := json.Marshal(fields)
	require.NoError(t, err)
	return string(data)
}

func TestOpenAITokensRepositoryRefresh(t *testing.T) {
	t.Parallel()

	t.Run("should send the refresh grant Codex sends, as JSON", func(t *testing.T) {
		t.Parallel()
		// given
		requests := make(chan map[string]string, 1)
		access := builders.NewOpenAITokenBuilder().WithExpiry(time.Now().Add(accessLifetime)).Build()
		server := serveTokens(t, http.StatusOK,
			tokenResponse(t, map[string]any{"access_token": access, "refresh_token": "new-refresh"}), requests)
		repo := repositories.NewOpenAITokensRepository(server.URL, codexClientID, server.Client())

		// when
		_, err := repo.Refresh(codexSignInCreds())

		// then
		require.NoError(t, err)
		request := <-requests
		assert.Equal(t, map[string]string{
			"client_id":     codexClientID,
			"grant_type":    "refresh_token",
			"refresh_token": "old-refresh",
			"content-type":  "application/json",
		}, request)
	})

	t.Run("should keep the ID token and account through a refresh that does not restate them", func(t *testing.T) {
		t.Parallel()
		// given: Codex cannot load a sign-in without its ID token
		expires := time.Now().Add(accessLifetime).Truncate(time.Second)
		access := builders.NewOpenAITokenBuilder().WithExpiry(expires).Build()
		server := serveTokens(t, http.StatusOK,
			tokenResponse(t, map[string]any{"access_token": access, "refresh_token": "new-refresh"}),
			make(chan map[string]string, 1))
		repo := repositories.NewOpenAITokensRepository(server.URL, codexClientID, server.Client())

		// when
		refreshed, err := repo.Refresh(codexSignInCreds())

		// then
		require.NoError(t, err)
		assert.Equal(t, access, refreshed.AccessToken)
		assert.Equal(t, "new-refresh", refreshed.RefreshToken,
			"the endpoint rotated the refresh token, and the old one is refused from now on")
		assert.Equal(t, "old-id", refreshed.IDToken)
		assert.Equal(t, codexAccountID, refreshed.AccountID)
		assert.Equal(t, codexPlan, refreshed.SubscriptionType)
		assert.Equal(t, expires.UnixMilli(), refreshed.ExpiresAt, "the expiry is read out of the access token")
	})

	t.Run("should take the plan and account a newly minted ID token names", func(t *testing.T) {
		t.Parallel()
		// given
		idToken := builders.NewOpenAITokenBuilder().WithEmail(codexEmail).WithAccountID("other-account").
			WithPlan("pro").Build()
		access := builders.NewOpenAITokenBuilder().WithExpiry(time.Now().Add(accessLifetime)).Build()
		server := serveTokens(t, http.StatusOK, tokenResponse(t, map[string]any{
			"access_token": access, "refresh_token": "new-refresh", "id_token": idToken,
		}), make(chan map[string]string, 1))
		repo := repositories.NewOpenAITokensRepository(server.URL, codexClientID, server.Client())

		// when
		refreshed, err := repo.Refresh(codexSignInCreds())

		// then
		require.NoError(t, err)
		assert.Equal(t, idToken, refreshed.IDToken)
		assert.Equal(t, "pro", refreshed.SubscriptionType)
		assert.Equal(t, "other-account", refreshed.AccountID)
	})

	t.Run("should refuse an answer without an access token rather than blank the account", func(t *testing.T) {
		t.Parallel()
		// given
		server := serveTokens(t, http.StatusOK, tokenResponse(t, map[string]any{"refresh_token": "new-refresh"}),
			make(chan map[string]string, 1))
		repo := repositories.NewOpenAITokensRepository(server.URL, codexClientID, server.Client())

		// when
		refreshed, err := repo.Refresh(codexSignInCreds())

		// then
		require.Error(t, err)
		assert.Nil(t, refreshed)
	})

	t.Run("should report the endpoint's refusal of a reused refresh token", func(t *testing.T) {
		t.Parallel()
		// given
		server := serveTokens(t, http.StatusUnauthorized, `{"error":{"code":"refresh_token_reused"}}`,
			make(chan map[string]string, 1))
		repo := repositories.NewOpenAITokensRepository(server.URL, codexClientID, server.Client())

		// when
		_, err := repo.Refresh(codexSignInCreds())

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refresh_token_reused")
	})
}
