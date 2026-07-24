// Package entities contains the pure domain model for ccswitch, free of any
// infrastructure or framework concerns.
package entities

// OAuthCredentials holds the Claude Code OAuth tokens for a single account.
// It mirrors the "claudeAiOauth" object stored inside ~/.claude/.credentials.json.
type OAuthCredentials struct {
	AccessToken           string   `json:"accessToken"`
	RefreshToken          string   `json:"refreshToken"`
	ExpiresAt             int64    `json:"expiresAt"`
	RefreshTokenExpiresAt int64    `json:"refreshTokenExpiresAt,omitempty"`
	Scopes                []string `json:"scopes,omitempty"`
	SubscriptionType      string   `json:"subscriptionType,omitempty"`
	RateLimitTier         string   `json:"rateLimitTier,omitempty"`
}

// AccessTokenExpired reports whether the access token is expired at the given
// epoch milliseconds. A missing expiry is treated as expired.
func (c OAuthCredentials) AccessTokenExpired(nowMillis int64) bool {
	if c.ExpiresAt == 0 {
		return true
	}
	return nowMillis >= c.ExpiresAt
}

// SameAccountAs reports whether two credential sets carry the same refresh token
// and therefore belong to the same account.
//
// This is only a positive signal: the server rotates the refresh token on every
// refresh, so a false result does NOT mean the credentials belong to different
// accounts — it may simply mean one side has been refreshed since. Use
// Store.MatchAccount, which resolves the account by its stable identity, to
// decide which enrolled account a credential set belongs to.
func (c OAuthCredentials) SameAccountAs(other OAuthCredentials) bool {
	return c.RefreshToken != "" && c.RefreshToken == other.RefreshToken
}

// Valid reports whether the credentials carry the minimum tokens required to
// authenticate and to refresh.
func (c OAuthCredentials) Valid() bool {
	return c.AccessToken != "" && c.RefreshToken != ""
}
