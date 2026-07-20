package repositories

import "github.com/rios0rios0/ccswitch/internal/domain/entities"

// UsageRepository fetches live usage and limit data for an account from the
// Claude usage endpoint.
type UsageRepository interface {
	// Fetch returns the current usage for the account identified by the given OAuth
	// access token.
	Fetch(accessToken string) (*entities.Usage, error)
}
