//go:build !darwin

package controllers

import (
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

// newCredentialsRepository returns the credentials swapper for this platform,
// which reads and writes Claude Code's .credentials.json file.
func newCredentialsRepository(cfg *entities.Config) domain.CredentialsRepository {
	return repositories.NewFileCredentialsRepository(cfg.CredentialsPath, cfg.ClaudeJSONPath)
}
