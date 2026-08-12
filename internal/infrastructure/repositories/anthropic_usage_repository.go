package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

const (
	usagePath        = "/api/oauth/usage"
	oauthBetaHeader  = "oauth-2025-04-20"
	usageHTTPTimeout = 10 * time.Second
	userAgent        = "ccswitch (github.com/rios0rios0/ccswitch)"
)

type windowResponse struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

type limitResponse struct {
	Kind     string  `json:"kind"`
	Group    string  `json:"group"`
	Percent  float64 `json:"percent"`
	Severity string  `json:"severity"`
	IsActive bool    `json:"is_active"`
	ResetsAt string  `json:"resets_at"`
}

type extraUsageResponse struct {
	IsEnabled bool `json:"is_enabled"`
}

type usageResponse struct {
	FiveHour   windowResponse     `json:"five_hour"`
	SevenDay   windowResponse     `json:"seven_day"`
	Limits     []limitResponse    `json:"limits"`
	ExtraUsage extraUsageResponse `json:"extra_usage"`
}

// AnthropicUsageRepository fetches usage from the Claude usage endpoint over HTTP.
type AnthropicUsageRepository struct {
	client  *http.Client
	baseURL string
}

// NewAnthropicUsageRepository creates a repository targeting the given base URL.
// When client is nil a default client with a sane timeout is used.
func NewAnthropicUsageRepository(baseURL string, client *http.Client) *AnthropicUsageRepository {
	if client == nil {
		client = &http.Client{Timeout: usageHTTPTimeout} //nolint:exhaustruct // only Timeout is needed
	}
	return &AnthropicUsageRepository{client: client, baseURL: baseURL}
}

// Fetch retrieves the current usage for the account owning the given access token.
func (r *AnthropicUsageRepository) Fetch(accessToken string) (*entities.Usage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), usageHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+usagePath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Anthropic-Beta", oauthBetaHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call usage endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read usage response: %w", err)
	}
	// A rejected token is reported as its own error so callers can refresh and retry
	// instead of reporting the account as unreadable; see domain.ErrUnauthorized.
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("%w (status %d): %s", domain.ErrUnauthorized, resp.StatusCode, string(body))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed usageResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse usage response: %w", err)
	}
	return mapUsage(parsed), nil
}

// mapUsage converts the wire response into a domain Usage entity.
func mapUsage(resp usageResponse) *entities.Usage {
	limits := make([]entities.Limit, 0, len(resp.Limits))
	for _, item := range resp.Limits {
		limits = append(limits, entities.Limit{
			Kind:     item.Kind,
			Group:    item.Group,
			Percent:  item.Percent,
			Severity: item.Severity,
			IsActive: item.IsActive,
			ResetsAt: parseTime(item.ResetsAt),
		})
	}
	return &entities.Usage{
		FiveHour: entities.Window{
			Utilization: resp.FiveHour.Utilization,
			ResetsAt:    parseTime(resp.FiveHour.ResetsAt),
		},
		SevenDay: entities.Window{
			Utilization: resp.SevenDay.Utilization,
			ResetsAt:    parseTime(resp.SevenDay.ResetsAt),
		},
		Limits:     limits,
		ExtraUsage: resp.ExtraUsage.IsEnabled,
	}
}

// parseTime parses an RFC3339 timestamp, returning the zero time on failure.
func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
