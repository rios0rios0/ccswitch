package repositories

import (
	"errors"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// ErrUnauthorized marks a usage fetch the server rejected as unauthenticated.
//
// An access token can be rejected well before the expiry recorded alongside it:
// the server invalidates tokens on its own schedule, and logging a session in
// again supersedes the pair ccswitch captured earlier. Callers match on this to
// tell "the token is dead, mint a new one" apart from a transport or server-side
// failure, where refreshing would burn the refresh token for nothing.
var ErrUnauthorized = errors.New("usage endpoint rejected the access token")

// UsageRepository fetches live usage and limit data for an account from the
// Claude usage endpoint.
type UsageRepository interface {
	// Fetch returns the current usage for the account identified by the given OAuth
	// access token. It returns an error wrapping ErrUnauthorized when the server
	// rejects the token.
	Fetch(accessToken string) (*entities.Usage, error)
}
