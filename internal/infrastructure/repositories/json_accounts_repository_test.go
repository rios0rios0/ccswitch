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

func TestJSONAccountsRepository(t *testing.T) {
	t.Parallel()

	t.Run("should round-trip the store", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		repo := repositories.NewJSONAccountsRepository(path)
		store := &entities.Store{
			Accounts: []entities.Account{{
				Email:       "a@example.com",
				Credentials: entities.OAuthCredentials{RefreshToken: "r"},
			}},
			Rotation: entities.RotationState{CurrentEmail: "a@example.com"},
		}

		// when
		saveErr := repo.Save(store)
		loaded, loadErr := repo.Load()

		// then
		require.NoError(t, saveErr)
		require.NoError(t, loadErr)
		require.Len(t, loaded.Accounts, 1)
		assert.Equal(t, "a@example.com", loaded.Rotation.CurrentEmail)
	})

	t.Run("should return an empty store when the file is missing", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewJSONAccountsRepository(filepath.Join(t.TempDir(), "absent.json"))

		// when
		loaded, err := repo.Load()

		// then
		require.NoError(t, err)
		assert.Empty(t, loaded.Accounts)
	})
}

func TestJSONAccountsRepositoryForProvider(t *testing.T) {
	t.Parallel()

	// sharedThreshold is a threshold persisted by `ccswitch threshold` in the main store.
	const sharedThreshold = 95.0

	t.Run("should say whose accounts each store holds", func(t *testing.T) {
		t.Parallel()
		// given
		dir := t.TempDir()
		main := repositories.NewJSONAccountsRepository(filepath.Join(dir, "store.json"))
		codex := main.ForProvider(entities.ProviderCodex, filepath.Join(dir, "store.codex.json"))

		// when
		claudeStore, claudeErr := main.Load()
		codexStore, codexErr := codex.Load()

		// then
		require.NoError(t, claudeErr)
		require.NoError(t, codexErr)
		assert.Equal(t, entities.ProviderClaude, claudeStore.Provider)
		assert.Equal(t, entities.ProviderCodex, codexStore.Provider)
	})

	t.Run("should keep a provider's accounts in a file of their own and leave the main store untouched",
		func(t *testing.T) {
			t.Parallel()
			// given: a ccswitch that predates the provider rewrites the main store with only
			// the keys it knows, so accounts kept there would be erased by it
			dir := t.TempDir()
			mainPath := filepath.Join(dir, "store.json")
			main := repositories.NewJSONAccountsRepository(mainPath)
			require.NoError(t, main.Save(&entities.Store{
				Accounts: []entities.Account{{Email: "claude@example.com"}},
				Rotation: entities.RotationState{CurrentEmail: "claude@example.com"},
			}))
			before, err := os.ReadFile(mainPath)
			require.NoError(t, err)
			codex := main.ForProvider(entities.ProviderCodex, filepath.Join(dir, "store.codex.json"))

			// when
			err = codex.Save(&entities.Store{
				Accounts: []entities.Account{{Email: "codex@example.com"}},
				Rotation: entities.RotationState{CurrentEmail: "codex@example.com"},
			})

			// then
			require.NoError(t, err)
			after, readErr := os.ReadFile(mainPath)
			require.NoError(t, readErr)
			assert.Equal(t, before, after)
			loaded, loadErr := codex.Load()
			require.NoError(t, loadErr)
			require.Len(t, loaded.Accounts, 1)
			assert.Equal(t, "codex@example.com", loaded.Rotation.CurrentEmail)
		})

	t.Run("should read the settings every provider shares from the main store", func(t *testing.T) {
		t.Parallel()
		// given
		dir := t.TempDir()
		main := repositories.NewJSONAccountsRepository(filepath.Join(dir, "store.json"))
		settings := entities.Settings{}
		settings.SetThreshold(sharedThreshold)
		require.NoError(t, main.Save(&entities.Store{Settings: settings}))
		codex := main.ForProvider(entities.ProviderCodex, filepath.Join(dir, "store.codex.json"))

		// when
		loaded, err := codex.Load()

		// then
		require.NoError(t, err)
		require.NotNil(t, loaded.Settings.Threshold, "a threshold set once applies to every provider")
		assert.InDelta(t, sharedThreshold, *loaded.Settings.Threshold, 0)
	})

	t.Run("should never write the shared settings into a provider's own file", func(t *testing.T) {
		t.Parallel()
		// given
		dir := t.TempDir()
		codexPath := filepath.Join(dir, "store.codex.json")
		codex := repositories.NewJSONAccountsRepository(filepath.Join(dir, "store.json")).
			ForProvider(entities.ProviderCodex, codexPath)
		settings := entities.Settings{}
		settings.SetThreshold(sharedThreshold)

		// when
		err := codex.Save(&entities.Store{Settings: settings})

		// then
		require.NoError(t, err)
		data, readErr := os.ReadFile(codexPath)
		require.NoError(t, readErr)
		assert.NotContains(t, string(data), "threshold", "the main store owns the settings")
	})
}
