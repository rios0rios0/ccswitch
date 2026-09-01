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

// WithRefreshed returns these credentials updated with the result of an OAuth
// refresh, carrying forward every descriptive field the token endpoint did not
// return.
//
// A refresh answers with the new token pair and little else: it omits the
// subscription tier and the rate-limit tier, and it reports a refresh-token
// expiry only when it rotated one. Rebuilding the credentials from the response
// alone therefore installs a strictly poorer document than the one it replaces,
// and the missing scopes are the part that logs the user out: Claude Code
// refuses to persist its own later refresh of a credential set whose scopes do
// not name `user:inference`, classifying it as not-claude.ai and dropping it.
// The pair on disk then goes stale, the refresh after that answers
// invalid_grant, and Claude Code blanks the credentials. Claude Code merges the
// same way for the same reason.
func (c OAuthCredentials) WithRefreshed(refreshed OAuthCredentials) OAuthCredentials {
	merged := refreshed
	if merged.RefreshToken == "" {
		merged.RefreshToken = c.RefreshToken
	}
	if merged.RefreshTokenExpiresAt == 0 {
		merged.RefreshTokenExpiresAt = c.RefreshTokenExpiresAt
	}
	if len(merged.Scopes) == 0 {
		merged.Scopes = c.Scopes
	}
	if merged.SubscriptionType == "" {
		merged.SubscriptionType = c.SubscriptionType
	}
	if merged.RateLimitTier == "" {
		merged.RateLimitTier = c.RateLimitTier
	}
	return merged
}

// Blank reports whether the credentials carry no access token.
//
// That is how Claude Code marks a login as dead: when the server answers a
// refresh with invalid_grant it rewrites claudeAiOauth with empty tokens and a
// zero expiry instead of deleting it. Capturing such a set into the store would
// replace the account's last good tokens with the very state that says they are
// gone, so every capture path has to reject it. A long-lived token still has an
// access token and is therefore not blank.
func (c OAuthCredentials) Blank() bool {
	return c.AccessToken == ""
}

// Degraded reports whether the credentials are missing the scopes a Claude Code
// subscription login carries, while still being refreshable.
//
// Only a credential set that an earlier ccswitch rebuilt from a bare refresh
// response looks like this, and it is not merely cosmetic: Claude Code discards
// its own refresh of a set whose scopes omit `user:inference`, so installing one
// strands the pair on disk until it 401s and the account is signed out. A single
// refresh restores them, which is why polling repairs such an account instead of
// waiting for its access token to age out.
func (c OAuthCredentials) Degraded() bool {
	return c.RefreshToken != "" && len(c.Scopes) == 0
}
