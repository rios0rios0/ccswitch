package repositories_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

func TestFileCredentialsRepository(t *testing.T) {
	t.Parallel()

	t.Run("should round-trip credentials and identity", func(t *testing.T) {
		t.Parallel()
		// given
		dir := t.TempDir()
		credPath := filepath.Join(dir, ".credentials.json")
		jsonPath := filepath.Join(dir, ".claude.json")
		repo := repositories.NewFileCredentialsRepository(credPath, jsonPath)
		creds := &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "r", SubscriptionType: "max"}
		identity := &entities.AccountIdentity{EmailAddress: "a@example.com", AccountUUID: "u"}

		// when
		writeErr := repo.Write(creds, identity)
		gotCreds, gotIdentity, readErr := repo.Read()

		// then
		require.NoError(t, writeErr)
		require.NoError(t, readErr)
		assert.Equal(t, "a", gotCreds.AccessToken)
		require.NotNil(t, gotIdentity)
		assert.Equal(t, "a@example.com", gotIdentity.EmailAddress)
	})

	t.Run("should preserve unrelated keys in the Claude state file", func(t *testing.T) {
		t.Parallel()
		// given
		dir := t.TempDir()
		credPath := filepath.Join(dir, ".credentials.json")
		jsonPath := filepath.Join(dir, ".claude.json")
		original := `{"numStartups":42,"oauthAccount":{"emailAddress":"old@example.com"}}`
		require.NoError(t, os.WriteFile(jsonPath, []byte(original), 0o600))
		repo := repositories.NewFileCredentialsRepository(credPath, jsonPath)

		// when
		err := repo.Write(
			&entities.OAuthCredentials{AccessToken: "a", RefreshToken: "r"},
			&entities.AccountIdentity{EmailAddress: "new@example.com"},
		)

		// then
		require.NoError(t, err)
		data, readErr := os.ReadFile(jsonPath)
		require.NoError(t, readErr)
		assert.Contains(t, string(data), "numStartups")
		assert.Contains(t, string(data), "new@example.com")
	})
}
