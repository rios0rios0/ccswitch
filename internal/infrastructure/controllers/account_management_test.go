package controllers_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/controllers"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

const (
	primaryEmail = "primary@example.com"
	backupEmail  = "backup@example.com"
	thirdEmail   = "third@example.com"
)

// executeCLI runs the public command tree against an isolated store. Explicit
// token enrollment keeps these tests independent of real logins and keychains.
func executeCLI(storePath string, args ...string) error {
	root := controllers.NewRootCommand("test")
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{
		"--store", storePath,
		"--credentials", filepath.Join(filepath.Dir(storePath), "credentials.json"),
		"--claude-json", filepath.Join(filepath.Dir(storePath), "claude.json"),
	}, args...))
	return root.Execute()
}

// enrollFixture creates a manual-only account through the public CLI.
func enrollFixture(t *testing.T, path, email string) {
	t.Helper()
	require.NoError(t, executeCLI(path, "enroll", "--email", email, "--token", "fixture-token-placeholder"))
}

// storedOrder reloads the persisted order rather than relying on in-memory state.
func storedOrder(t *testing.T, path string) []string {
	t.Helper()
	store, err := repositories.NewJSONAccountsRepository(path).Load()
	require.NoError(t, err)
	var emails []string
	for i, account := range store.Ordered() {
		assert.Equal(t, i, account.Order)
		emails = append(emails, account.Email)
	}
	return emails
}

func TestAccountManagementCLI(t *testing.T) {
	t.Parallel()

	t.Run("should persist enrollment and priority changes across invocations", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		enrollFixture(t, path, primaryEmail)
		enrollFixture(t, path, backupEmail)
		enrollFixture(t, path, thirdEmail)

		// when
		err := executeCLI(path, "update", thirdEmail, "--priority", "top")

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{thirdEmail, primaryEmail, backupEmail}, storedOrder(t, path))
		require.NoError(t, executeCLI(path, "reorder", backupEmail, primaryEmail))
		assert.Equal(t, []string{backupEmail, primaryEmail, thirdEmail}, storedOrder(t, path))
		require.NoError(t, executeCLI(path, "enroll", "--email", thirdEmail,
			"--token", "replacement-token-placeholder", "--priority", "2"))
		assert.Equal(t, []string{backupEmail, thirdEmail, primaryEmail}, storedOrder(t, path))
		store, err := repositories.NewJSONAccountsRepository(path).Load()
		require.NoError(t, err)
		assert.Equal(t, primaryEmail, store.Rotation.CurrentEmail, "reordering must not switch credentials")
		assert.Equal(t, "replacement-token-placeholder", store.FindAccount(thirdEmail).Credentials.AccessToken)
	})

	t.Run("should support reading and removing accounts through aliases", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		enrollFixture(t, path, primaryEmail)
		enrollFixture(t, path, backupEmail)

		// when
		err := executeCLI(path, "get", backupEmail)

		// then
		require.NoError(t, err)
		require.NoError(t, executeCLI(path, "show", primaryEmail))
		require.NoError(t, executeCLI(path, "list"))
		require.NoError(t, executeCLI(path, "rm", backupEmail))
		assert.Equal(t, []string{primaryEmail}, storedOrder(t, path))
		require.NoError(t, executeCLI(path, "delete", primaryEmail))
		store, err := repositories.NewJSONAccountsRepository(path).Load()
		require.NoError(t, err)
		assert.Empty(t, store.Accounts)
		assert.Empty(t, store.Rotation.CurrentEmail)
		require.ErrorIs(t, executeCLI(path, "show", primaryEmail), entities.ErrAccountNotEnrolled)
	})
}

func TestAccountManagementCLIRejectsInvalidChanges(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{"update", primaryEmail},
		{"update", primaryEmail, "--priority", "0"},
		{"update", primaryEmail, "--priority", "3"},
		{"update", "missing@example.com", "--priority", "top"},
		{"reorder"},
		{"reorder", backupEmail, backupEmail},
		{"reorder", backupEmail, "missing@example.com"},
		{"remove", backupEmail, "missing@example.com"},
		{"remove"},
		{"show"},
		{"show", primaryEmail, backupEmail},
		{"enroll", "--email", thirdEmail, "--token", "fixture-token-placeholder", "--priority", "4"},
	}
	for _, args := range cases {
		t.Run(args[0]+"/"+args[len(args)-1], func(t *testing.T) {
			t.Parallel()
			// given
			path := filepath.Join(t.TempDir(), "store.json")
			enrollFixture(t, path, primaryEmail)
			enrollFixture(t, path, backupEmail)
			before, err := os.ReadFile(path)
			require.NoError(t, err)

			// when
			err = executeCLI(path, args...)

			// then
			require.Error(t, err)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, before, after, "rejected commands must leave the persisted store untouched")
		})
	}
}
