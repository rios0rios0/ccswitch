//go:build windows

package controllers

import (
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

// newSessionsRepository returns the session detector for this platform.
func newSessionsRepository() domain.SessionsRepository {
	return repositories.NewToolhelpSessionsRepository()
}
