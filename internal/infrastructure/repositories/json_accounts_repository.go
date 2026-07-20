package repositories

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// JSONAccountsRepository persists the ccswitch store as a single JSON file with
// owner-only permissions.
type JSONAccountsRepository struct {
	path string
}

// NewJSONAccountsRepository creates a JSONAccountsRepository backed by the file at
// the given path.
func NewJSONAccountsRepository(path string) *JSONAccountsRepository {
	return &JSONAccountsRepository{path: path}
}

// Load reads the store from disk. A missing file yields an empty store so that a
// first enrollment can proceed.
func (r *JSONAccountsRepository) Load() (*entities.Store, error) {
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return &entities.Store{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read account store: %w", err)
	}

	var store entities.Store
	if err = json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("failed to parse account store: %w", err)
	}
	return &store, nil
}

// Save atomically persists the store with owner-only permissions.
func (r *JSONAccountsRepository) Save(store *entities.Store) error {
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal account store: %w", err)
	}
	if err = atomicWrite(r.path, data, filePerm); err != nil {
		return fmt.Errorf("failed to write account store: %w", err)
	}
	return nil
}
