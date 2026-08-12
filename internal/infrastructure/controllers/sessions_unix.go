//go:build !windows && !darwin

package controllers

import (
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

// procRoot is the kernel's process filesystem, absent on systems that do not
// provide one (in which case no session is ever detected).
const procRoot = "/proc"

// newSessionsRepository returns the session detector for this platform.
func newSessionsRepository() domain.SessionsRepository {
	return repositories.NewProcSessionsRepository(procRoot)
}
