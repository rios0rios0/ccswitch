package entities_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

func TestParseProvider(t *testing.T) {
	t.Parallel()

	t.Run("should read a provider whatever its case and surrounding space", func(t *testing.T) {
		t.Parallel()
		// given
		cases := []struct {
			name string
			want entities.Provider
		}{
			{name: "claude", want: entities.ProviderClaude},
			{name: "  codex\n", want: entities.ProviderCodex},
			{name: "CODEX", want: entities.ProviderCodex},
			{name: "\tClaude", want: entities.ProviderClaude},
		}

		for _, tc := range cases {
			// when
			provider, err := entities.ParseProvider(tc.name)

			// then
			require.NoError(t, err)
			assert.Equal(t, tc.want, provider)
		}
	})

	t.Run("should reject a provider ccswitch does not support and name the ones it does", func(t *testing.T) {
		t.Parallel()
		// given
		name := "gemini"

		// when
		_, err := entities.ParseProvider(name)

		// then
		require.ErrorIs(t, err, entities.ErrUnknownProvider)
		assert.Contains(t, err.Error(), "claude, codex")
	})
}

func TestProviderProfile(t *testing.T) {
	t.Parallel()

	t.Run(
		"should treat the zero value as Claude, the provider of every store written before Codex",
		func(t *testing.T) {
			t.Parallel()
			// given
			var provider entities.Provider

			// when
			resolved := provider.Resolved()

			// then
			assert.Equal(t, entities.ProviderClaude, resolved)
			assert.Equal(t, "Claude", provider.Name())
			assert.Equal(t, "claude", provider.Command())
		},
	)

	t.Run("should phrase Claude's messages exactly as ccswitch always has", func(t *testing.T) {
		t.Parallel()
		// given
		provider := entities.ProviderClaude

		// when
		steps, login, hint, note := provider.LoginSteps(), provider.LoginCommand(), provider.IdentityHint(),
			provider.OverrideNote()

		// then
		assert.Equal(t, "`claude` and `/login`", steps)
		assert.Equal(t, "`claude`", login)
		assert.Equal(t, "ensure ~/.claude.json contains an oauthAccount block", hint)
		assert.Equal(t, "Claude Code will ignore rotated OAuth credentials", note)
		assert.Equal(t, []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}, provider.OverrideVariables())
		assert.True(t, provider.LongLivedTokens())
	})

	t.Run("should name Codex's CLI, its login and the variables that override its sign-in", func(t *testing.T) {
		t.Parallel()
		// given
		provider := entities.ProviderCodex

		// when
		name, command, login := provider.Name(), provider.Command(), provider.LoginCommand()

		// then
		assert.Equal(t, "Codex", name)
		assert.Equal(t, "codex", command)
		assert.Equal(t, "`codex login`", login)
		assert.Equal(t, []string{"CODEX_API_KEY", "CODEX_ACCESS_TOKEN"}, provider.OverrideVariables())
		assert.False(t, provider.LongLivedTokens(), "every Codex account is a ChatGPT sign-in")
	})

	t.Run("should list Claude before Codex", func(t *testing.T) {
		t.Parallel()
		// given / when
		providers := entities.Providers()

		// then
		assert.Equal(t, []entities.Provider{entities.ProviderClaude, entities.ProviderCodex}, providers)
	})
}

func TestProviderDegraded(t *testing.T) {
	t.Parallel()

	// scopeless is a refreshable credential set naming no scope at all: degraded for
	// Claude Code, and the normal shape of a Codex sign-in.
	scopeless := entities.OAuthCredentials{AccessToken: "access", RefreshToken: "refresh"}

	t.Run("should hold Claude's credentials to the user:inference scope", func(t *testing.T) {
		t.Parallel()
		// given
		provider := entities.ProviderClaude

		// when
		degraded := provider.Degraded(scopeless)

		// then
		assert.True(t, degraded)
	})

	t.Run("should never report Codex's credentials degraded, whatever their scopes", func(t *testing.T) {
		t.Parallel()
		// given: a Codex sign-in names no Claude scope, and the repair refresh that a
		// degraded set gets would spend a refresh token on every poll
		provider := entities.ProviderCodex

		// when
		degraded := provider.Degraded(scopeless)

		// then
		assert.False(t, degraded)
	})
}
