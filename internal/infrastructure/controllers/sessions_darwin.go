//go:build darwin

package controllers

import (
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

// newSessionsRepository returns the session detector for this platform. macOS has
// no /proc, so sessions are detected from the process table instead.
func newSessionsRepository() domain.SessionsRepository {
	return repositories.NewPSSessionsRepository()
}
