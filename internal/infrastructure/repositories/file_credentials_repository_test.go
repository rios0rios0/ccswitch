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

	t.Run("should preserve the MCP tokens stored beside claudeAiOauth", func(t *testing.T) {
		t.Parallel()
		// given: the credentials file holds more than the subscription tokens --
		// mcpOAuth carries the OAuth tokens of every authenticated MCP server, so
		// marshalling only claudeAiOauth over it signs the user out of all of them
		// on every rotation
		dir := t.TempDir()
		credPath := filepath.Join(dir, ".credentials.json")
		jsonPath := filepath.Join(dir, ".claude.json")
		original := `{"claudeAiOauth":{"accessToken":"old"},` +
			`"mcpOAuth":{"server-a":{"accessToken":"mcp-token"}},` +
			`"designOauth":{"accessToken":"design-token"}}`
		require.NoError(t, os.WriteFile(credPath, []byte(original), 0o600))
		repo := repositories.NewFileCredentialsRepository(credPath, jsonPath)

		// when
		err := repo.Write(
			&entities.OAuthCredentials{AccessToken: "new", RefreshToken: "r"},
			&entities.AccountIdentity{EmailAddress: "a@example.com"},
		)

		// then
		require.NoError(t, err)
		data, readErr := os.ReadFile(credPath)
		require.NoError(t, readErr)
		assert.Contains(t, string(data), "mcp-token")
		assert.Contains(t, string(data), "design-token")
		assert.Contains(t, string(data), `"accessToken": "new"`)
	})

	t.Run("should refuse to overwrite an unparsable credentials file", func(t *testing.T) {
		t.Parallel()
		// given: a file that exists but cannot be parsed may still hold MCP tokens,
		// so replacing it wholesale is worse than failing
		dir := t.TempDir()
		credPath := filepath.Join(dir, ".credentials.json")
		jsonPath := filepath.Join(dir, ".claude.json")
		require.NoError(t, os.WriteFile(credPath, []byte("{not json"), 0o600))
		repo := repositories.NewFileCredentialsRepository(credPath, jsonPath)

		// when
		err := repo.Write(&entities.OAuthCredentials{AccessToken: "new"}, nil)

		// then
		require.Error(t, err)
		data, readErr := os.ReadFile(credPath)
		require.NoError(t, readErr)
		assert.Equal(t, "{not json", string(data))
	})
}
