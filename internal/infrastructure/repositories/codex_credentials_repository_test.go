package repositories_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
	"github.com/rios0rios0/ccswitch/test/builders"
)

const (
	codexEmail     = "codex@example.com"
	codexAccountID = "fixture-account-id"
	codexPlan      = "plus"
	// accessLifetime is how long an access token minted for Codex lasts.
	accessLifetime = 10 * 24 * time.Hour
)

// codexSignIn returns the tokens of a ChatGPT sign-in issued at the given moment: an ID
// token naming the account, and an access token naming its expiry.
func codexSignIn(issued time.Time) (string, string) {
	idToken := builders.NewOpenAITokenBuilder().
		WithEmail(codexEmail).WithName("Codex Person").WithAccountID(codexAccountID).WithPlan(codexPlan).
		WithIssuedAt(issued).WithExpiry(issued.Add(time.Hour)).Build()
	accessToken := builders.NewOpenAITokenBuilder().
		WithProfileEmail(codexEmail).WithAccountID(codexAccountID).WithPlan(codexPlan).
		WithIssuedAt(issued).WithExpiry(issued.Add(accessLifetime)).Build()
	return idToken, accessToken
}

// writeCodexAuth writes an auth.json holding the given document into a new Codex home
// and returns the home.
func writeCodexAuth(t *testing.T, document map[string]any) string {
	t.Helper()
	home := t.TempDir()
	data, err := json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(home, "auth.json"), data, 0o600))
	return home
}

// readCodexAuth returns the auth.json in the Codex home as a generic document.
func readCodexAuth(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "auth.json"))
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(data, &document))
	return document
}

func TestCodexCredentialsRepositoryRead(t *testing.T) {
	t.Parallel()

	t.Run("should read the ChatGPT sign-in and the identity its ID token names", func(t *testing.T) {
		t.Parallel()
		// given
		issued := time.Now().Truncate(time.Second)
		idToken, accessToken := codexSignIn(issued)
		home := writeCodexAuth(t, map[string]any{
			"auth_mode":      "chatgpt",
			"OPENAI_API_KEY": nil,
			"tokens": map[string]any{
				"id_token": idToken, "access_token": accessToken,
				"refresh_token": "fixture-refresh-placeholder", "account_id": codexAccountID,
			},
			"last_refresh": issued.UTC().Format(time.RFC3339Nano),
		})
		repo := repositories.NewCodexCredentialsRepository(home)

		// when
		creds, identity, err := repo.Read()

		// then
		require.NoError(t, err)
		assert.Equal(t, accessToken, creds.AccessToken)
		assert.Equal(t, "fixture-refresh-placeholder", creds.RefreshToken)
		assert.Equal(t, idToken, creds.IDToken)
		assert.Equal(t, codexAccountID, creds.AccountID)
		assert.Equal(t, issued.Add(accessLifetime).UnixMilli(), creds.ExpiresAt,
			"auth.json records no expiry; the access token names it")
		assert.Equal(t, codexPlan, creds.SubscriptionType)
		require.NotNil(t, identity)
		assert.Equal(t, codexEmail, identity.EmailAddress)
		assert.Equal(t, codexAccountID, identity.AccountUUID)
		assert.Equal(t, "Codex Person", identity.DisplayName)
	})

	t.Run("should tell apart two people's seats in one workspace, which act for the same account",
		func(t *testing.T) {
			t.Parallel()
			// given: matching on the workspace's account alone would attribute one seat's
			// tokens to the other's enrolled account
			seatOf := func(userID string) string {
				home := writeCodexAuth(t, map[string]any{
					"auth_mode": "chatgpt",
					"tokens": map[string]any{
						"id_token": builders.NewOpenAITokenBuilder().WithEmail(userID + "@example.com").
							WithUserID(userID).WithAccountID(codexAccountID).Build(),
						"access_token": "opaque-access-placeholder", "refresh_token": "fixture-refresh-placeholder",
						"account_id": codexAccountID,
					},
				})
				_, identity, err := repositories.NewCodexCredentialsRepository(home).Read()
				require.NoError(t, err)
				require.NotNil(t, identity)
				return identity.AccountUUID
			}

			// when
			first, second := seatOf("user-first"), seatOf("user-second")

			// then
			assert.Equal(t, "user-first__"+codexAccountID, first, "the seat as Codex itself names it")
			assert.NotEqual(t, first, second)
		})

	t.Run("should treat tokens without an expiry as fresh for as long as Codex does", func(t *testing.T) {
		t.Parallel()
		// given: an access token that names no expiry, as Codex falls back from
		lastRefresh := time.Now().Add(-time.Hour).Truncate(time.Second)
		home := writeCodexAuth(t, map[string]any{
			"auth_mode": "chatgpt",
			"tokens": map[string]any{
				"id_token":      builders.NewOpenAITokenBuilder().WithEmail(codexEmail).Build(),
				"access_token":  "opaque-access-placeholder",
				"refresh_token": "fixture-refresh-placeholder",
			},
			"last_refresh": lastRefresh.UTC().Format(time.RFC3339Nano),
		})

		// when
		creds, _, err := repositories.NewCodexCredentialsRepository(home).Read()

		// then
		require.NoError(t, err)
		assert.Equal(t, lastRefresh.Add(8*24*time.Hour).UnixMilli(), creds.ExpiresAt)
	})

	t.Run("should report a Codex signed in with an API key as not signed in with ChatGPT", func(t *testing.T) {
		t.Parallel()
		// given
		home := writeCodexAuth(t, map[string]any{"auth_mode": "apikey", "OPENAI_API_KEY": "fixture-key-placeholder"})

		// when
		_, _, err := repositories.NewCodexCredentialsRepository(home).Read()

		// then
		require.ErrorIs(t, err, repositories.ErrCodexNotChatGPT)
	})

	t.Run("should report a ChatGPT sign-in without tokens as blank credentials", func(t *testing.T) {
		t.Parallel()
		// given
		home := writeCodexAuth(t, map[string]any{"auth_mode": "chatgpt", "tokens": nil})

		// when
		creds, identity, err := repositories.NewCodexCredentialsRepository(home).Read()

		// then
		require.NoError(t, err)
		assert.True(t, creds.Blank(), "blank credentials are never captured over an account's tokens")
		assert.Nil(t, identity)
	})

	t.Run("should fail when Codex has no auth file", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewCodexCredentialsRepository(t.TempDir())

		// when
		_, _, err := repo.Read()

		// then
		require.Error(t, err)
		assert.False(t, repo.Exists())
	})
}

func TestCodexCredentialsRepositoryWrite(t *testing.T) {
	t.Parallel()

	t.Run("should round-trip a sign-in through the auth file", func(t *testing.T) {
		t.Parallel()
		// given
		issued := time.Now().Truncate(time.Second)
		idToken, accessToken := codexSignIn(issued)
		home := t.TempDir()
		repo := repositories.NewCodexCredentialsRepository(home)
		creds := &entities.OAuthCredentials{
			AccessToken: accessToken, RefreshToken: "fixture-refresh-placeholder",
			IDToken: idToken, AccountID: codexAccountID,
		}

		// when
		writeErr := repo.Write(creds, nil)
		read, identity, readErr := repo.Read()

		// then
		require.NoError(t, writeErr)
		require.NoError(t, readErr)
		assert.Equal(t, accessToken, read.AccessToken)
		assert.Equal(t, idToken, read.IDToken)
		require.NotNil(t, identity)
		assert.Equal(t, codexEmail, identity.EmailAddress)
		document := readCodexAuth(t, home)
		assert.Equal(t, "chatgpt", document["auth_mode"])
		assert.Contains(t, document, "OPENAI_API_KEY", "Codex writes the key as null for a ChatGPT sign-in")
		assert.Nil(t, document["OPENAI_API_KEY"])
		assert.Equal(t, issued.UTC().Format(time.RFC3339Nano), document["last_refresh"],
			"Codex's last refresh is when the access token was minted")
		assert.True(t, repo.Exists())
	})

	t.Run("should carry every other key of the auth file across untouched", func(t *testing.T) {
		t.Parallel()
		// given: auth.json can hold more than the tokens, and keys this version does not
		// know must survive the swap
		_, accessToken := codexSignIn(time.Now())
		home := writeCodexAuth(t, map[string]any{
			"auth_mode":      "chatgpt",
			"OPENAI_API_KEY": nil,
			"agent_identity": map[string]any{"agent_runtime_id": "fixture-runtime"},
			"tokens": map[string]any{
				"id_token": "old-id", "access_token": "old-access", "refresh_token": "old-refresh",
				"account_id": "old-account", "future_field": "kept",
			},
		})

		// when
		err := repositories.NewCodexCredentialsRepository(home).Write(&entities.OAuthCredentials{
			AccessToken: accessToken, RefreshToken: "new-refresh", IDToken: "new-id",
		}, nil)

		// then
		require.NoError(t, err)
		document := readCodexAuth(t, home)
		assert.Equal(t, map[string]any{"agent_runtime_id": "fixture-runtime"}, document["agent_identity"])
		tokens, ok := document["tokens"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "new-id", tokens["id_token"])
		assert.Equal(t, "new-refresh", tokens["refresh_token"])
		assert.Nil(t, tokens["account_id"], "an account the credentials do not name is not carried over")
		assert.Equal(t, "kept", tokens["future_field"])
	})

	t.Run("should refuse to replace a sign-in made with an API key", func(t *testing.T) {
		t.Parallel()
		// given: signing Codex in with an API key is a deliberate choice, which a rotation
		// must not undo
		original := map[string]any{"auth_mode": "apikey", "OPENAI_API_KEY": "fixture-key-placeholder"}
		home := writeCodexAuth(t, original)

		// when
		err := repositories.NewCodexCredentialsRepository(home).Write(
			&entities.OAuthCredentials{AccessToken: "access", RefreshToken: "refresh", IDToken: "id"}, nil)

		// then
		require.ErrorIs(t, err, repositories.ErrCodexNotChatGPT)
		assert.Equal(t, original, readCodexAuth(t, home))
	})

	t.Run("should refuse an API key sign-in written before Codex recorded its auth mode", func(t *testing.T) {
		t.Parallel()
		// given
		original := map[string]any{"OPENAI_API_KEY": "fixture-key-placeholder", "tokens": nil}
		home := writeCodexAuth(t, original)

		// when
		err := repositories.NewCodexCredentialsRepository(home).Write(
			&entities.OAuthCredentials{AccessToken: "access", RefreshToken: "refresh", IDToken: "id"}, nil)

		// then
		require.ErrorIs(t, err, repositories.ErrCodexNotChatGPT)
		assert.Equal(t, original, readCodexAuth(t, home))
	})

	t.Run("should refuse credentials without an ID token, which would sign Codex out", func(t *testing.T) {
		t.Parallel()
		// given
		home := t.TempDir()
		repo := repositories.NewCodexCredentialsRepository(home)

		// when
		err := repo.Write(&entities.OAuthCredentials{AccessToken: "access", RefreshToken: "refresh"}, nil)

		// then
		require.Error(t, err)
		assert.False(t, repo.Exists(), "nothing is written")
	})

	t.Run("should refuse to overwrite an unparsable auth file", func(t *testing.T) {
		t.Parallel()
		// given
		home := t.TempDir()
		path := filepath.Join(home, "auth.json")
		require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))

		// when
		err := repositories.NewCodexCredentialsRepository(home).Write(
			&entities.OAuthCredentials{AccessToken: "access", RefreshToken: "refresh", IDToken: "id"}, nil)

		// then
		require.Error(t, err)
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.Equal(t, "{not json", string(data))
	})
}
