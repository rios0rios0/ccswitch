package controllers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/controllers"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
	"github.com/rios0rios0/ccswitch/test/builders"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

const (
	codexEmail       = "codex@example.com"
	secondCodexEmail = "codex2@example.com"
	sharedEmail      = "both@example.com"
	codexLifetime    = 10 * 24 * time.Hour
)

// codexHomeOf returns the Codex home the test CLI runs against: beside the store.
func codexHomeOf(storePath string) string {
	return filepath.Join(filepath.Dir(storePath), "codex")
}

// signCodexIn writes the auth.json `codex login` would leave behind for the email
// into the Codex home the test CLI reads.
func signCodexIn(t *testing.T, storePath, email string) {
	t.Helper()
	issued := time.Now()
	document := map[string]any{
		"auth_mode":      "chatgpt",
		"OPENAI_API_KEY": nil,
		"tokens": map[string]any{
			"id_token": builders.NewOpenAITokenBuilder().
				WithEmail(email).WithAccountID("account-of-" + email).WithPlan("plus").Build(),
			"access_token": builders.NewOpenAITokenBuilder().
				WithProfileEmail(email).WithAccountID("account-of-" + email).
				WithIssuedAt(issued).WithExpiry(issued.Add(codexLifetime)).Build(),
			"refresh_token": "refresh-of-" + email,
			"account_id":    "account-of-" + email,
		},
		"last_refresh": issued.UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(codexHomeOf(storePath), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(codexHomeOf(storePath), "auth.json"), data, 0o600))
}

// enrollCodex signs Codex in as the email and enrolls that sign-in.
func enrollCodex(t *testing.T, storePath, email string) {
	t.Helper()
	signCodexIn(t, storePath, email)
	require.NoError(t, executeCLI(storePath, "enroll", "--provider", "codex"))
}

// codexStoreOf loads the store Codex's accounts are kept in, beside the main store.
func codexStoreOf(t *testing.T, storePath string) *entities.Store {
	t.Helper()
	store, err := repositories.NewJSONAccountsRepository(storePath).
		ForProvider(entities.ProviderCodex, filepath.Join(filepath.Dir(storePath), "store.codex.json")).Load()
	require.NoError(t, err)
	return store
}

// codexSignedInAs returns the email Codex is signed in as, from its auth.json.
func codexSignedInAs(t *testing.T, storePath string) string {
	t.Helper()
	_, identity, err := repositories.NewCodexCredentialsRepository(codexHomeOf(storePath)).Read()
	require.NoError(t, err)
	require.NotNil(t, identity)
	return identity.EmailAddress
}

func TestCodexAccountsCLI(t *testing.T) {
	t.Parallel()

	t.Run("should enroll the ChatGPT account Codex is signed in to into a store of its own", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		signCodexIn(t, path, codexEmail)

		// when
		err := executeCLI(path, "enroll", "--provider", "codex")

		// then
		require.NoError(t, err)
		store := codexStoreOf(t, path)
		require.Len(t, store.Accounts, 1)
		assert.Equal(t, codexEmail, store.Rotation.CurrentEmail)
		assert.NotEmpty(t, store.Accounts[0].Credentials.IDToken)
		_, statErr := os.Stat(path)
		assert.ErrorIs(t, statErr, os.ErrNotExist, "Claude's store is not written when a Codex account is enrolled")
	})

	t.Run("should switch Codex to another enrolled account with use, found by its email alone", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		enrollCodex(t, path, codexEmail)
		enrollCodex(t, path, secondCodexEmail)
		signCodexIn(t, path, codexEmail)

		// when
		err := executeCLI(path, "use", secondCodexEmail)

		// then
		require.NoError(t, err)
		assert.Equal(t, secondCodexEmail, codexSignedInAs(t, path))
		assert.Equal(t, secondCodexEmail, codexStoreOf(t, path).Rotation.CurrentEmail)
		_, statErr := os.Stat(filepath.Join(filepath.Dir(path), "credentials.json"))
		assert.ErrorIs(t, statErr, os.ErrNotExist, "Claude Code's credentials are left alone")
	})

	t.Run("should install the current Codex account before a launch with ensure", func(t *testing.T) {
		t.Parallel()
		// given: Codex was signed in as someone else since the account was enrolled
		path := filepath.Join(t.TempDir(), "store.json")
		enrollCodex(t, path, codexEmail)
		enrollCodex(t, path, secondCodexEmail)
		require.NoError(t, executeCLI(path, "use", codexEmail))
		signCodexIn(t, path, secondCodexEmail)

		// when
		err := executeCLI(path, "ensure", "--provider", "codex", "--quiet")

		// then
		require.NoError(t, err)
		assert.Equal(t, codexEmail, codexSignedInAs(t, path))
	})

	t.Run("should move a Codex account in Codex's order without touching Claude's", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		enrollFixture(t, path, primaryEmail)
		enrollFixture(t, path, backupEmail)
		enrollCodex(t, path, codexEmail)
		enrollCodex(t, path, secondCodexEmail)

		// when
		err := executeCLI(path, "update", secondCodexEmail, "--priority", "top")

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{secondCodexEmail, codexEmail}, emailsOf(codexStoreOf(t, path)))
		assert.Equal(t, []string{primaryEmail, backupEmail}, storedOrder(t, path))
	})

	t.Run("should remove accounts of both providers at once", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		enrollFixture(t, path, primaryEmail)
		enrollFixture(t, path, backupEmail)
		enrollCodex(t, path, codexEmail)
		enrollCodex(t, path, secondCodexEmail)

		// when
		err := executeCLI(path, "remove", backupEmail, secondCodexEmail)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{primaryEmail}, storedOrder(t, path))
		assert.Equal(t, []string{codexEmail}, emailsOf(codexStoreOf(t, path)))
	})

	t.Run("should tell from the store alone that a Codex account has capacity", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		enrollCodex(t, path, codexEmail)
		var out bytes.Buffer

		// when
		err := executeCLIWith(path, &doubles.StubSelfUpdateRepository{}, &out, io.Discard,
			"capacity", "--provider", "codex")

		// then
		require.NoError(t, err)
		assert.Empty(t, out.String())
	})
}

func TestCodexAccountsCLIRejectsWhatItCannotResolve(t *testing.T) {
	t.Parallel()

	t.Run("should ask which provider an email enrolled with both refers to", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		enrollFixture(t, path, sharedEmail)
		enrollCodex(t, path, sharedEmail)
		enrollCodex(t, path, codexEmail)

		// when
		err := executeCLI(path, "update", sharedEmail, "--priority", "bottom")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--provider claude or --provider codex")
		assert.Equal(t, []string{sharedEmail, codexEmail}, emailsOf(codexStoreOf(t, path)))
		require.NoError(t, executeCLI(path, "update", sharedEmail, "--priority", "bottom", "--provider", "codex"))
		assert.Equal(t, []string{codexEmail, sharedEmail}, emailsOf(codexStoreOf(t, path)))
	})

	t.Run("should refuse to reorder accounts of different providers together", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		enrollFixture(t, path, primaryEmail)
		enrollFixture(t, path, backupEmail)
		enrollCodex(t, path, codexEmail)

		// when
		err := executeCLI(path, "reorder", backupEmail, codexEmail)

		// then
		require.Error(t, err)
		assert.Equal(t, []string{primaryEmail, backupEmail}, storedOrder(t, path))
	})

	t.Run("should remove nothing when one of the named accounts is enrolled with no provider", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		enrollFixture(t, path, primaryEmail)
		enrollCodex(t, path, codexEmail)

		// when
		err := executeCLI(path, "remove", codexEmail, "missing@example.com")

		// then
		require.ErrorIs(t, err, entities.ErrAccountNotEnrolled)
		assert.Equal(t, []string{codexEmail}, emailsOf(codexStoreOf(t, path)))
	})

	t.Run("should refuse a long-lived token for Codex, which mints none", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")

		// when
		err := executeCLI(path, "enroll", "--provider", "codex",
			"--email", codexEmail, "--token", "fixture-token-placeholder")

		// then
		require.Error(t, err)
		assert.Empty(t, codexStoreOf(t, path).Accounts)
	})

	t.Run("should reject a provider ccswitch does not support", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")

		// when
		err := executeCLI(path, "--provider", "gemini", "rotate")

		// then
		require.ErrorIs(t, err, entities.ErrUnknownProvider)
	})
}

func TestDaemonArgsCodexHome(t *testing.T) {
	t.Parallel()

	t.Run("should start the daemon against the same Codex home as the invocation", func(t *testing.T) {
		t.Parallel()
		// given
		cfg := &entities.Config{Interval: time.Minute, CodexHome: "/somewhere/codex"}

		// when
		args := controllers.DaemonArgs(cfg)

		// then
		assert.Contains(t, args, "--codex-home")
		assert.Equal(t, "/somewhere/codex", args[indexOf(args, "--codex-home")+1])
	})
}

// emailsOf returns the store's accounts in rotation order.
func emailsOf(store *entities.Store) []string {
	ordered := store.Ordered()
	emails := make([]string, 0, len(ordered))
	for _, account := range ordered {
		emails = append(emails, account.Email)
	}
	return emails
}

// indexOf returns the position of the value among the values, or -1.
func indexOf(values []string, value string) int {
	for i, candidate := range values {
		if candidate == value {
			return i
		}
	}
	return -1
}
