package repositories_test

import (
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
