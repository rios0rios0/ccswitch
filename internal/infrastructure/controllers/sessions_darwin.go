//go:build darwin

package controllers

import (
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

// newSessionsRepository returns the detector of the sessions of the CLI that runs
// as command, for this platform. macOS has no /proc, so sessions are detected from
// the process table instead.
func newSessionsRepository(command string) domain.SessionsRepository {
	return repositories.NewPSSessionsRepository(command)
}
