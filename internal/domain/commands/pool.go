package commands

import (
	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// Pool is one provider's accounts together with the repositories that reach them: the
// store they are enrolled in, the credential store the provider's CLI signs in from,
// the endpoints that report their usage and refresh their tokens, and the probe that
// tells whether the CLI is running. The store loaded through Accounts says which
// provider it is (see entities.Store.Provider).
//
// Accounts rotate only within their pool, because each CLI reads its own credential
// store: installing a Codex sign-in where Claude Code looks for its tokens would log
// it out. The commands that span every provider -- list, status, threshold and the
// monitor -- take a pool for each.
type Pool struct {
	Accounts    repositories.AccountsRepository
	Credentials repositories.CredentialsRepository
	Usage       repositories.UsageRepository
	Tokens      repositories.TokensRepository
	Sessions    repositories.SessionsRepository
}

// running reports whether a session of the pool's CLI is running. A pool without a
// session probe never has one.
func (p Pool) running() bool {
	return p.Sessions != nil && p.Sessions.Running()
}

// providerLog returns the logger for what happens to the given provider's accounts,
// so that a monitor rotating several providers says whose account each line is about.
func providerLog(provider entities.Provider) *logger.Entry {
	return logger.WithField("provider", string(provider.Resolved()))
}
