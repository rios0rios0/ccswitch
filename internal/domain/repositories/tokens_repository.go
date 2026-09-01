package repositories

import "github.com/rios0rios0/ccswitch/internal/domain/entities"

// TokensRepository exchanges an account's credentials for a fresh set, used to
// poll usage for accounts that are not currently active and to keep every
// enrolled account's refresh token alive.
type TokensRepository interface {
	// Refresh returns new credentials minted from the given ones.
	//
	// It takes the whole credential set rather than the refresh token alone
	// because a refresh is not a pure token swap: the request has to name the
	// scopes the new access token should carry, and the response omits the
	// descriptive fields (subscription tier, rate-limit tier, refresh-token
	// expiry) that the returned credentials must nevertheless keep. Passing only
	// the token would silently downgrade the account on every refresh.
	Refresh(creds entities.OAuthCredentials) (*entities.OAuthCredentials, error)
}
