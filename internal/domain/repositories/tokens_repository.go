package repositories

import "github.com/rios0rios0/ccswitch/internal/domain/entities"

// TokensRepository exchanges a refresh token for a fresh set of OAuth
// credentials, used to poll usage for accounts that are not currently active.
type TokensRepository interface {
	// Refresh returns new credentials minted from the given refresh token.
	Refresh(refreshToken string) (*entities.OAuthCredentials, error)
}
