package entities

import (
	"errors"
	"fmt"
	"strings"
)

// Provider is the coding assistant an enrolled account signs in to. Each one keeps its
// own credential store, usage endpoint and process, so an account only ever takes over
// from an account of the same provider: Claude accounts rotate among Claude accounts,
// and Codex accounts among Codex accounts.
type Provider string

const (
	// ProviderClaude is Claude Code, which signs in with a Claude subscription. It is the
	// provider of every store written before ccswitch knew of another, which is why the
	// zero value means it too.
	ProviderClaude Provider = "claude"
	// ProviderCodex is the OpenAI Codex CLI, which signs in with a ChatGPT account.
	ProviderCodex Provider = "codex"
)

// ErrUnknownProvider reports a provider name ccswitch does not support.
var ErrUnknownProvider = errors.New("unknown provider")

// providerProfile is what ccswitch has to know about a provider to talk about it and to
// keep its credentials healthy.
type providerProfile struct {
	// name is how the provider is named to a person, above its accounts.
	name string
	// command is the executable the CLI runs as, which is also what launches it.
	command string
	// loginSteps is how to sign the CLI in, phrased to follow "run".
	loginSteps string
	// loginCommand is the command that signs the CLI in, phrased to follow "log in with".
	loginCommand string
	// identityHint is where the email of the signed-in account comes from, for when it
	// cannot be found there.
	identityHint string
	// overrides are the environment variables that make the CLI sign in with something
	// other than the credentials ccswitch installs.
	overrides []string
	// overrideNote says what such a variable does, completing "<VARIABLE> is set; ".
	overrideNote string
	// inferenceScope reports whether the credentials must name ScopeInference, without
	// which the CLI drops its own refresh of them (see OAuthCredentials.Degraded).
	inferenceScope bool
	// longLivedTokens reports whether the CLI mints long-lived tokens an account can be
	// enrolled from directly, as `claude setup-token` does.
	longLivedTokens bool
}

// providerProfiles maps every provider to its profile. Supporting another provider is a
// new entry here rather than another branch wherever a provider is told apart.
func providerProfiles() map[Provider]providerProfile {
	return map[Provider]providerProfile{
		ProviderClaude: {
			name:         "Claude",
			command:      "claude",
			loginSteps:   "`claude` and `/login`",
			loginCommand: "`claude`",
			identityHint: "ensure ~/.claude.json contains an oauthAccount block",
			overrides:    []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"},
			overrideNote: "Claude Code will ignore rotated OAuth credentials",
			// Claude Code classifies a credential set whose scopes omit user:inference as
			// not-claude.ai and discards its own refresh of it, which is the logout.
			inferenceScope:  true,
			longLivedTokens: true,
		},
		ProviderCodex: {
			name:         "Codex",
			command:      "codex",
			loginSteps:   "`codex login`",
			loginCommand: "`codex login`",
			identityHint: "ensure Codex's auth.json holds the ID token of a ChatGPT sign-in",
			overrides:    []string{"CODEX_API_KEY", "CODEX_ACCESS_TOKEN"},
			overrideNote: "Codex signs in with it instead of the rotated OAuth credentials",
			// Codex keeps a ChatGPT sign-in whatever scopes its tokens carry.
			inferenceScope:  false,
			longLivedTokens: false,
		},
	}
}

// Providers returns every supported provider, in the order their accounts are listed.
func Providers() []Provider {
	return []Provider{ProviderClaude, ProviderCodex}
}

// ParseProvider reads a provider as the command line names it, ignoring case.
func ParseProvider(name string) (Provider, error) {
	provider := Provider(strings.ToLower(strings.TrimSpace(name)))
	if _, ok := providerProfiles()[provider]; ok {
		return provider, nil
	}

	names := make([]string, 0, len(Providers()))
	for _, known := range Providers() {
		names = append(names, string(known))
	}
	return "", fmt.Errorf("%w %q: use one of %s", ErrUnknownProvider, name, strings.Join(names, ", "))
}

// Resolved returns the provider the value stands for, which for the zero value is
// Claude: every store written before ccswitch knew of another provider is Claude's.
func (p Provider) Resolved() Provider {
	if p == "" {
		return ProviderClaude
	}
	return p
}

// Name returns how the provider is named to a person: "Claude" or "Codex".
func (p Provider) Name() string {
	return p.profile().name
}

// Command returns the executable the provider's CLI runs as, which is also what a
// person types to launch it: "claude" or "codex".
func (p Provider) Command() string {
	return p.profile().command
}

// LoginSteps returns how to sign the CLI in, phrased to follow "run".
func (p Provider) LoginSteps() string {
	return p.profile().loginSteps
}

// LoginCommand returns the command that signs the CLI in, phrased to follow "log in
// with".
func (p Provider) LoginCommand() string {
	return p.profile().loginCommand
}

// IdentityHint says where the email of the signed-in account is read from, for when it
// cannot be found there.
func (p Provider) IdentityHint() string {
	return p.profile().identityHint
}

// OverrideVariables returns the environment variables that make the CLI sign in with
// something other than the credentials ccswitch installs.
func (p Provider) OverrideVariables() []string {
	return p.profile().overrides
}

// OverrideNote says what setting one of OverrideVariables does, completing a sentence
// that starts "<VARIABLE> is set; ".
func (p Provider) OverrideNote() string {
	return p.profile().overrideNote
}

// LongLivedTokens reports whether the provider's CLI mints long-lived tokens an
// account can be enrolled from directly, as `claude setup-token` does. Codex has no
// such token: every Codex account is a ChatGPT sign-in.
func (p Provider) LongLivedTokens() bool {
	return p.profile().longLivedTokens
}

// Degraded reports whether the credentials fail the provider's scope invariant while
// still being refreshable (see OAuthCredentials.Degraded). Only Claude has one: Codex
// keeps a ChatGPT sign-in whatever scopes its tokens carry, so its credentials never
// need the repair refresh, and treating them as degraded would spend a refresh token on
// every poll.
func (p Provider) Degraded(creds OAuthCredentials) bool {
	return p.profile().inferenceScope && creds.Degraded()
}

// profile returns the provider's profile. The zero value has Claude's (see Resolved).
func (p Provider) profile() providerProfile {
	return providerProfiles()[p.Resolved()]
}
