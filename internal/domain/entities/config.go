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
	// account is considered exhausted and rotation is triggered.
	Threshold float64
	// Interval is how often the monitor daemon polls usage.
	Interval time.Duration
	// UsageBaseURL is the base URL of the Anthropic usage API.
	UsageBaseURL string
	// TokenURL is the OAuth token endpoint used to refresh access tokens.
	TokenURL string
	// ClientID is the Claude Code public OAuth client identifier used for refresh.
	ClientID string
}
