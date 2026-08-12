//go:build darwin

package controllers

import (
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

// newCredentialsRepository returns the credentials swapper for this platform.
//
// On macOS Claude Code keeps its tokens in the login keychain and treats
// ~/.credentials.json only as a fallback, consulted when the keychain read
// returns nothing. Writing that file therefore has no effect while the keychain
// item exists, so the keychain is the only correct target here.
func newCredentialsRepository(cfg *entities.Config) domain.CredentialsRepository {
	return repositories.NewKeychainCredentialsRepository(cfg.ClaudeJSONPath)
}
