//go:build windows

package controllers

import (
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

// newSessionsRepository returns the detector of the sessions of the CLI that runs
// as command, for this platform.
func newSessionsRepository(command string) domain.SessionsRepository {
	return repositories.NewToolhelpSessionsRepository(command)
}
