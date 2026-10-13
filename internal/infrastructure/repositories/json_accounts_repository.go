package repositories

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// JSONAccountsRepository persists one provider's accounts as a single JSON file with
// owner-only permissions.
//
// Claude's accounts live in the main store, which also holds the settings every
// provider shares. Every other provider keeps its accounts in a file of its own beside
// it (see ForProvider) rather than in a new key of the main store, because a ccswitch
// that predates the provider -- such as a monitor daemon still running the binary it
// was started from after an update -- rewrites the main store on every tick with only
// the keys it knows, and would erase every account it did not.
type JSONAccountsRepository struct {
	path     string
	provider entities.Provider
	// settingsPath is the store the shared settings are read from when it is another
	// file than this one: the main store, for every provider but Claude.
	settingsPath string
}

// NewJSONAccountsRepository creates the repository of the main store at the given
// path, which holds Claude's accounts and the settings every provider shares.
func NewJSONAccountsRepository(path string) *JSONAccountsRepository {
	return &JSONAccountsRepository{path: path, provider: entities.ProviderClaude}
}

// ForProvider returns the repository of another provider's accounts, kept in a file of
// their own at path. It reads the settings from this repository's store, and never
// writes them: they belong to the main store, which every provider reads them from.
func (r *JSONAccountsRepository) ForProvider(provider entities.Provider, path string) *JSONAccountsRepository {
	return &JSONAccountsRepository{path: path, provider: provider, settingsPath: r.path}
}

// Load reads the store from disk. A missing file yields an empty store so that a
// first enrollment can proceed.
func (r *JSONAccountsRepository) Load() (*entities.Store, error) {
	store, err := readStore(r.path)
	if err != nil {
		return nil, err
	}
	store.Provider = r.provider

	if r.settingsPath != "" {
		shared, sharedErr := readStore(r.settingsPath)
		if sharedErr != nil {
			return nil, sharedErr
		}
		store.Settings = shared.Settings
	}
	return store, nil
}

// Save atomically persists the store with owner-only permissions. A provider whose
// settings live in the main store writes its accounts alone.
func (r *JSONAccountsRepository) Save(store *entities.Store) error {
	persisted := *store
	if r.settingsPath != "" {
		persisted.Settings = entities.Settings{}
	}

	data, err := json.MarshalIndent(&persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal account store: %w", err)
	}
	if err = atomicWrite(r.path, data); err != nil {
		return fmt.Errorf("failed to write account store: %w", err)
	}
	return nil
}

// readStore reads a store file, returning an empty store when it does not exist.
func readStore(path string) (*entities.Store, error) {
	data, err := os.ReadFile(path)
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
