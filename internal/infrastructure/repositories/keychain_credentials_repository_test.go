//go:build darwin

package repositories_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

// fakeSecurityRunner emulates `security` over an in-memory keychain item.
type fakeSecurityRunner struct {
	stored  string // the item's plaintext payload; empty means "no such item"
	absent  bool
	truncAt int // when > 0, store only this many hex characters, as `security -i` does
	writes  int
}

// Run dispatches the subset of `security` verbs the repository relies on.
func (f *fakeSecurityRunner) Run(stdin []byte, args ...string) ([]byte, error) {
	switch args[0] {
	case "find-generic-password":
		if f.absent {
			return nil, errors.New("SecKeychainSearchCopyNext: The specified item could not be found")
		}
		return []byte(f.stored + "\n"), nil
	case "-i":
		return f.store(extractHexArgument(string(stdin)))
	case "add-generic-password":
		return f.store(args[len(args)-1])
	default:
		return nil, errors.New("unexpected verb " + args[0])
	}
}

// store decodes a hex payload into the item, optionally truncating it first.
func (f *fakeSecurityRunner) store(payload string) ([]byte, error) {
	f.writes++
	if f.truncAt > 0 && len(payload) > f.truncAt {
		payload = payload[:f.truncAt]
	}
	decoded, err := hex.DecodeString(payload)
	if err != nil {
		return nil, errors.New("bad hex payload")
	}
	f.stored = string(decoded)
	f.absent = false
	return nil, nil
}

// extractHexArgument pulls the -X value out of a `security -i` command line.
func extractHexArgument(line string) string {
	_, after, found := strings.Cut(line, `-X "`)
	if !found {
		return ""
	}
	value, _, _ := strings.Cut(after, `"`)
	return value
}

// keychainRepo builds a repository over the fake runner and a scratch state file.
func keychainRepo(t *testing.T, runner repositories.SecurityRunner) *repositories.KeychainCredentialsRepository {
	t.Helper()
	return repositories.NewKeychainCredentialsRepositoryWithRunner(
		filepath.Join(t.TempDir(), ".claude.json"), "Claude Code-credentials", "tester", runner)
}

func TestKeychainCredentialsRepositoryRead(t *testing.T) {
	t.Parallel()

	t.Run("should read the oauth credentials from the keychain item", func(t *testing.T) {
		t.Parallel()
		// given
		runner := &fakeSecurityRunner{
			stored: `{"claudeAiOauth":{"accessToken":"at","refreshToken":"rt","expiresAt":123}}`,
		}
		repo := keychainRepo(t, runner)

		// when
		creds, _, err := repo.Read()

		// then
		require.NoError(t, err)
		assert.Equal(t, "at", creds.AccessToken)
		assert.Equal(t, "rt", creds.RefreshToken)
		assert.Equal(t, int64(123), creds.ExpiresAt)
	})

	t.Run("should decode a hex-encoded payload", func(t *testing.T) {
		t.Parallel()
		// given `security -w` prints some items as a hex dump rather than as text
		document := `{"claudeAiOauth":{"accessToken":"at","refreshToken":"rt"}}`
		repo := keychainRepo(t, &fakeSecurityRunner{stored: hex.EncodeToString([]byte(document))})

		// when
		creds, _, err := repo.Read()

		// then
		require.NoError(t, err)
		assert.Equal(t, "at", creds.AccessToken)
	})

	t.Run("should fail when the keychain item is absent", func(t *testing.T) {
		t.Parallel()
		// given
		repo := keychainRepo(t, &fakeSecurityRunner{absent: true})

		// when
		_, _, err := repo.Read()

		// then
		require.Error(t, err)
	})
}

func TestKeychainCredentialsRepositoryWrite(t *testing.T) {
	t.Parallel()

	t.Run("should preserve mcpOAuth and every other key when rotating", func(t *testing.T) {
		t.Parallel()
		// given an item that also holds the MCP servers' access tokens, which a
		// whole-document overwrite would erase on every rotation
		runner := &fakeSecurityRunner{stored: `{` +
			`"claudeAiOauth":{"accessToken":"old","refreshToken":"old-rt"},` +
			`"mcpOAuth":{"linear|abc":{"accessToken":"mcp-token","serverName":"linear"}},` +
			`"someFutureKey":[1,2,3]}`}
		repo := keychainRepo(t, runner)

		// when
		err := repo.Write(&entities.OAuthCredentials{AccessToken: "new", RefreshToken: "new-rt"}, nil)

		// then
		require.NoError(t, err)

		var root map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(runner.stored), &root))
		assert.JSONEq(t,
			`{"linear|abc":{"accessToken":"mcp-token","serverName":"linear"}}`,
			string(root["mcpOAuth"]))
		assert.JSONEq(t, `[1,2,3]`, string(root["someFutureKey"]))
		assert.Contains(t, string(root["claudeAiOauth"]), `"accessToken":"new"`)
		assert.NotContains(t, string(root["claudeAiOauth"]), "old-rt")
	})

	t.Run("should create the document when no keychain item exists yet", func(t *testing.T) {
		t.Parallel()
		// given
		runner := &fakeSecurityRunner{absent: true}
		repo := keychainRepo(t, runner)

		// when
		err := repo.Write(&entities.OAuthCredentials{AccessToken: "at", RefreshToken: "rt"}, nil)

		// then
		require.NoError(t, err)
		assert.Contains(t, runner.stored, `"accessToken":"at"`)
	})

	t.Run("should refuse to overwrite an unparsable item", func(t *testing.T) {
		t.Parallel()
		// given a payload that cannot be merged into; replacing it wholesale would
		// sign the user out of every MCP server
		runner := &fakeSecurityRunner{stored: "not json at all"}
		repo := keychainRepo(t, runner)

		// when
		err := repo.Write(&entities.OAuthCredentials{AccessToken: "at", RefreshToken: "rt"}, nil)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refusing to overwrite")
		assert.Equal(t, "not json at all", runner.stored)
		assert.Zero(t, runner.writes)
	})

	t.Run("should fail loudly when security stores a truncated payload", func(t *testing.T) {
		t.Parallel()
		// given `security -i` silently truncates long command lines, which would
		// otherwise leave a corrupt item behind and report success
		runner := &fakeSecurityRunner{
			stored:  `{"claudeAiOauth":{"accessToken":"old","refreshToken":"old-rt"}}`,
			truncAt: 16,
		}
		repo := keychainRepo(t, runner)

		// when
		err := repo.Write(&entities.OAuthCredentials{AccessToken: "at", RefreshToken: "rt"}, nil)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "truncated")
	})

	t.Run("should record the account identity in the Claude state file", func(t *testing.T) {
		t.Parallel()
		// given
		runner := &fakeSecurityRunner{stored: `{"claudeAiOauth":{}}`}
		statePath := filepath.Join(t.TempDir(), ".claude.json")
		repo := repositories.NewKeychainCredentialsRepositoryWithRunner(
			statePath, "Claude Code-credentials", "tester", runner)

		// when
		err := repo.Write(
			&entities.OAuthCredentials{AccessToken: "at", RefreshToken: "rt"},
			&entities.AccountIdentity{EmailAddress: "backup@example.com", AccountUUID: "uuid-1"},
		)

		// then
		require.NoError(t, err)
		_, identity, readErr := repo.Read()
		require.NoError(t, readErr)
		require.NotNil(t, identity)
		assert.Equal(t, "backup@example.com", identity.EmailAddress)
		assert.Equal(t, "uuid-1", identity.AccountUUID)
	})
}

func TestKeychainCredentialsRepositoryExists(t *testing.T) {
	t.Parallel()

	t.Run("should report present when the item can be read", func(t *testing.T) {
		t.Parallel()
		// given
		repo := keychainRepo(t, &fakeSecurityRunner{stored: `{"claudeAiOauth":{}}`})

		// when / then
		assert.True(t, repo.Exists())
	})

	t.Run("should report absent when the item is missing", func(t *testing.T) {
		t.Parallel()
		// given
		repo := keychainRepo(t, &fakeSecurityRunner{absent: true})

		// when / then
		assert.False(t, repo.Exists())
	})
}
