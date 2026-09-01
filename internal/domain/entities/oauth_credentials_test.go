package entities_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

func TestOAuthCredentialsWithRefreshed(t *testing.T) {
	t.Parallel()

	prior := func() entities.OAuthCredentials {
		return entities.OAuthCredentials{
			AccessToken:           "old-access",
			RefreshToken:          "old-refresh",
			ExpiresAt:             1000,
			RefreshTokenExpiresAt: 9000,
			Scopes:                []string{"user:profile", "user:inference"},
			SubscriptionType:      "max",
			RateLimitTier:         "default_claude_max_20x",
		}
	}

	t.Run("should keep every field the refresh response omitted", func(t *testing.T) {
		t.Parallel()
		// given: what the token endpoint actually answers -- a token pair and an
		// expiry, with no scopes, tier, or refresh-token expiry
		minted := entities.OAuthCredentials{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresAt:    2000,
		}

		// when
		merged := prior().WithRefreshed(minted)

		// then: dropping the scopes here is what logs Claude Code out later
		assert.Equal(t, "new-access", merged.AccessToken)
		assert.Equal(t, "new-refresh", merged.RefreshToken)
		assert.Equal(t, int64(2000), merged.ExpiresAt)
		assert.Equal(t, []string{"user:profile", "user:inference"}, merged.Scopes)
		assert.Equal(t, "max", merged.SubscriptionType)
		assert.Equal(t, "default_claude_max_20x", merged.RateLimitTier)
		assert.Equal(t, int64(9000), merged.RefreshTokenExpiresAt)
	})

	t.Run("should take every field the refresh response did restate", func(t *testing.T) {
		t.Parallel()
		// given
		minted := entities.OAuthCredentials{
			AccessToken:           "new-access",
			RefreshToken:          "new-refresh",
			ExpiresAt:             2000,
			RefreshTokenExpiresAt: 12000,
			Scopes:                []string{"user:inference"},
			SubscriptionType:      "pro",
			RateLimitTier:         "default",
		}

		// when
		merged := prior().WithRefreshed(minted)

		// then
		assert.Equal(t, []string{"user:inference"}, merged.Scopes)
		assert.Equal(t, "pro", merged.SubscriptionType)
		assert.Equal(t, "default", merged.RateLimitTier)
		assert.Equal(t, int64(12000), merged.RefreshTokenExpiresAt)
	})

	t.Run("should carry the prior refresh token when the response omits it", func(t *testing.T) {
		t.Parallel()
		// given
		minted := entities.OAuthCredentials{AccessToken: "new-access", ExpiresAt: 2000}

		// when
		merged := prior().WithRefreshed(minted)

		// then
		assert.Equal(t, "old-refresh", merged.RefreshToken)
	})
}

func TestOAuthCredentialsBlank(t *testing.T) {
	t.Parallel()

	t.Run("should report blank for the marker Claude Code writes on invalid_grant", func(t *testing.T) {
		t.Parallel()
		// given: Claude Code blanks the tokens in place rather than removing them
		creds := entities.OAuthCredentials{AccessToken: "", RefreshToken: "", ExpiresAt: 0}

		// when
		blank := creds.Blank()

		// then
		assert.True(t, blank)
	})

	t.Run("should not report blank for a long-lived token", func(t *testing.T) {
		t.Parallel()
		// given: a `claude setup-token` credential, which has no refresh token
		creds := entities.OAuthCredentials{AccessToken: "long-lived"}

		// when
		blank := creds.Blank()

		// then
		assert.False(t, blank)
	})
}

func TestOAuthCredentialsDegraded(t *testing.T) {
	t.Parallel()

	t.Run("should report degraded for a set carrying no scopes", func(t *testing.T) {
		t.Parallel()
		// given: what an earlier ccswitch left behind, rebuilding from a bare response
		creds := entities.OAuthCredentials{AccessToken: "a", RefreshToken: "r"}

		// when
		degraded := creds.Degraded()

		// then
		assert.True(t, degraded)
	})

	t.Run("should report degraded for scopes narrowed away from user:inference", func(t *testing.T) {
		t.Parallel()
		// given: a non-empty set that still fails the invariant. Claude Code discards
		// its own refresh of this exactly as it does an empty one, so testing only
		// for emptiness left the state sticky and invisible to the repair path.
		creds := entities.OAuthCredentials{
			AccessToken:  "a",
			RefreshToken: "r",
			Scopes:       []string{"user:profile", "user:file_upload"},
		}

		// when
		degraded := creds.Degraded()

		// then
		assert.True(t, degraded)
	})

	t.Run("should not report degraded once the scopes name user:inference", func(t *testing.T) {
		t.Parallel()
		// given
		creds := entities.OAuthCredentials{
			AccessToken:  "a",
			RefreshToken: "r",
			Scopes:       []string{"user:profile", entities.ScopeInference},
		}

		// when
		degraded := creds.Degraded()

		// then
		assert.False(t, degraded)
	})

	t.Run("should not report degraded for a long-lived token", func(t *testing.T) {
		t.Parallel()
		// given: no refresh token, so there is nothing to repair it with
		creds := entities.OAuthCredentials{AccessToken: "long-lived"}

		// when
		degraded := creds.Degraded()

		// then
		assert.False(t, degraded)
	})
}
