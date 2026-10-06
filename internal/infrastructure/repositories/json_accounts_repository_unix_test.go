//go:build !windows

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

// wantStorePerm is the mode the store is written with. Windows has no such bits
// to assert: Go reports every writable file there as 0666, and the store takes
// the access of the directory it lives in instead.
const wantStorePerm = os.FileMode(0o600)

func TestJSONAccountsRepositoryPermissions(t *testing.T) {
	t.Parallel()

	t.Run("should write the store with owner-only permissions", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")
		repo := repositories.NewJSONAccountsRepository(path)

		// when
		err := repo.Save(&entities.Store{})

		// then
		require.NoError(t, err)
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		assert.Equal(t, wantStorePerm, info.Mode().Perm())
	})
}
