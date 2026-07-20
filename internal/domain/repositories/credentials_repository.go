package repositories

import "github.com/rios0rios0/ccswitch/internal/domain/entities"

// CredentialsRepository reads and writes the Claude Code active credentials, i.e.
// the account that the `claude` CLI will authenticate as on its next launch.
type CredentialsRepository interface {
	// Read returns the OAuth credentials currently installed for the `claude` CLI
	// together with the account identity, when available.
	Read() (*entities.OAuthCredentials, *entities.AccountIdentity, error)
	// Write atomically installs the given credentials as the active ones and, best
	// effort, records the matching account identity for display.
	Write(creds *entities.OAuthCredentials, identity *entities.AccountIdentity) error
	// Exists reports whether the active credentials file is present.
	Exists() bool
}
