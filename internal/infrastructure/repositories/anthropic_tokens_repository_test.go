package repositories_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

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
		creds, err := repo.Refresh("old-refresh")

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
		creds, err := repo.Refresh("kept-refresh")

		// then
		require.NoError(t, err)
		assert.Equal(t, "kept-refresh", creds.RefreshToken)
	})
}
