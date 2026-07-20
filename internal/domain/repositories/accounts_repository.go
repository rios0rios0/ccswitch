// Package repositories defines the domain-facing contracts (ports) that the
// infrastructure layer implements.
package repositories

import "github.com/rios0rios0/ccswitch/internal/domain/entities"

// AccountsRepository persists the ccswitch account store (enrolled accounts and
// rotation state).
type AccountsRepository interface {
	// Load returns the persisted store. A missing store yields an empty store and
	// no error, so that the first enrollment can proceed.
	Load() (*entities.Store, error)
	// Save atomically persists the store with restricted permissions.
	Save(store *entities.Store) error
}
