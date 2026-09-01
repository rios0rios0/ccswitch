package entities

import "time"

// Config holds the resolved runtime configuration for ccswitch: filesystem
// locations, the rotation threshold, the daemon poll interval, and the OAuth
// endpoints. It is assembled from defaults, environment, and flags by the
// controllers layer and injected into commands.
type Config struct {
	// CredentialsPath is the Claude Code active-credentials file (~/.claude/.credentials.json).
	CredentialsPath string
	// ClaudeJSONPath is the Claude Code state file that carries oauthAccount (~/.claude.json).
	ClaudeJSONPath string
	// StorePath is the ccswitch account store (~/.local/state/ccswitch/store.json).
	StorePath string
	// Threshold is the utilization percentage (0-100) at or above which an active
	// account is considered exhausted and rotation is triggered. It is the
	// fallback: the value persisted by `ccswitch threshold` wins unless the flag
	// was passed explicitly, which ThresholdExplicit records.
	Threshold float64
	// ThresholdExplicit reports whether --threshold was named on the command line.
	// Only then does the flag outrank the persisted setting; otherwise the store
	// is authoritative, which is what lets `ccswitch threshold` retune a running
	// daemon in flight.
	ThresholdExplicit bool
	// Interval is how often the monitor daemon polls usage.
	Interval time.Duration
	// PreferPrimary makes the monitor always run on the highest-priority account
	// that has capacity, switching back to it as soon as its limits reset. When
	// false the monitor cycles forward through the accounts instead, staying on
	// each one until it is exhausted.
	PreferPrimary bool
	// UsageBaseURL is the base URL of the Anthropic usage API.
	UsageBaseURL string
	// TokenURL is the OAuth token endpoint used to refresh access tokens.
	TokenURL string
	// ClientID is the Claude Code public OAuth client identifier used for refresh.
	ClientID string
}

// ResolveThreshold returns the rotation threshold to apply, preferring an
// explicitly passed --threshold over the value persisted in the store, and the
// built-in default over both when neither is set.
func (c *Config) ResolveThreshold(settings Settings) float64 {
	if c.ThresholdExplicit {
		return c.Threshold
	}
	return settings.ThresholdOr(c.Threshold)
}
