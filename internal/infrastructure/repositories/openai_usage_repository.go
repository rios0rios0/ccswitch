package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

const (
	// codexUsagePath is where the ChatGPT backend reports an account's Codex limits,
	// under its /backend-api base -- the same endpoint Codex's own /status reads.
	codexUsagePath = "/wham/usage"
	// chatGPTAccountHeader names the ChatGPT account a request acts for, which a
	// person who belongs to a workspace has more than one of. It is the canonical form
	// of the "ChatGPT-Account-Id" header Codex sends.
	chatGPTAccountHeader = "Chatgpt-Account-Id"
	// codeReviewScope names the limit that applies to Codex's code reviews alone.
	codeReviewScope = "review"
	// unknownWindowLabel names a window the endpoint gave no length for.
	unknownWindowLabel = "window"

	secondsPerMinute = 60
	secondsPerHour   = 60 * secondsPerMinute
	secondsPerDay    = 24 * secondsPerHour
)

// openAIUsageResponse mirrors the Codex usage status the ChatGPT backend reports.
type openAIUsageResponse struct {
	PlanType string `json:"plan_type"`
	// RateLimit is the limit every Codex request counts against.
	RateLimit *openAIRateLimit `json:"rate_limit"`
	// CodeReviewRateLimit applies to Codex's code reviews alone.
	CodeReviewRateLimit *openAIRateLimit `json:"code_review_rate_limit"`
	// AdditionalRateLimits each apply to one model, such as a faster variant.
	AdditionalRateLimits []openAIAdditionalRateLimit `json:"additional_rate_limits"`
}

// openAIRateLimit is one limit: whether the account may use it, and its windows.
type openAIRateLimit struct {
	// Allowed is a pointer so that a response which leaves it out is not read as a
	// refusal.
	Allowed         *bool         `json:"allowed"`
	LimitReached    bool          `json:"limit_reached"`
	PrimaryWindow   *openAIWindow `json:"primary_window"`
	SecondaryWindow *openAIWindow `json:"secondary_window"`
}

// openAIWindow is one rolling window of a limit.
type openAIWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
	ResetAfterSeconds  int64   `json:"reset_after_seconds"`
	ResetAt            int64   `json:"reset_at"`
}

// openAIAdditionalRateLimit is a limit on one model, named the way Codex names it.
type openAIAdditionalRateLimit struct {
	LimitName      string           `json:"limit_name"`
	MeteredFeature string           `json:"metered_feature"`
	RateLimit      *openAIRateLimit `json:"rate_limit"`
}

// OpenAIUsageRepository fetches an account's Codex limits from the ChatGPT backend
// over HTTP.
type OpenAIUsageRepository struct {
	client  *http.Client
	baseURL string
	now     func() time.Time
}

// NewOpenAIUsageRepository creates a repository targeting the given base URL of the
// ChatGPT backend, such as https://chatgpt.com/backend-api. When client is nil a
// default client with a sane timeout is used.
func NewOpenAIUsageRepository(baseURL string, client *http.Client) *OpenAIUsageRepository {
	if client == nil {
		client = &http.Client{Timeout: usageHTTPTimeout} //nolint:exhaustruct // only Timeout is needed
	}
	return &OpenAIUsageRepository{client: client, baseURL: baseURL, now: time.Now}
}

// Fetch retrieves the current Codex limits of the account owning the given access
// token. The ChatGPT account it acts for is read out of the token itself.
func (r *OpenAIUsageRepository) Fetch(accessToken string) (*entities.Usage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), usageHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+codexUsagePath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if claims, claimsErr := parseOpenAIClaims(accessToken); claimsErr == nil && claims.Auth.AccountID != "" {
		req.Header.Set(chatGPTAccountHeader, claims.Auth.AccountID)
	}

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

	var parsed openAIUsageResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse usage response: %w", err)
	}
	return r.mapUsage(parsed), nil
}

// mapUsage converts the wire response into a domain Usage reported as limits only:
// the windows of a Codex account depend on its plan -- a free plan has a single
// 30-day one, the paid plans a 5-hour and a 7-day one -- so none of them is assumed.
//
// Only the windows of the main limit are active. A limit on code reviews or on one
// model leaves every other request served, so exhausting one must not rotate the
// account away; it is listed, since when it resets is what a reader wants from it. A
// refusal of the main limit blocks the account however full its windows are.
func (r *OpenAIUsageRepository) mapUsage(resp openAIUsageResponse) *entities.Usage {
	usage := &entities.Usage{LimitsOnly: true} //nolint:exhaustruct // a Codex usage has no fixed windows
	if resp.RateLimit != nil {
		usage.Limits = append(usage.Limits, r.windowLimits(*resp.RateLimit, "", true)...)
		usage.Blocked = resp.RateLimit.Allowed != nil && !*resp.RateLimit.Allowed
	}
	if resp.CodeReviewRateLimit != nil {
		usage.Limits = append(usage.Limits, r.windowLimits(*resp.CodeReviewRateLimit, codeReviewScope, false)...)
	}
	for _, additional := range resp.AdditionalRateLimits {
		if additional.RateLimit == nil {
			continue
		}
		scope := additional.LimitName
		if scope == "" {
			scope = additional.MeteredFeature
		}
		usage.Limits = append(usage.Limits, r.windowLimits(*additional.RateLimit, scope, false)...)
	}
	return usage
}

// windowLimits maps the windows of one limit onto domain limits, each named for the
// length of its window and the scope it applies to.
func (r *OpenAIUsageRepository) windowLimits(limit openAIRateLimit, scope string, active bool) []entities.Limit {
	windows := []*openAIWindow{limit.PrimaryWindow, limit.SecondaryWindow}
	limits := make([]entities.Limit, 0, len(windows))
	for _, window := range windows {
		if window == nil {
			continue
		}
		limits = append(limits, entities.Limit{ //nolint:exhaustruct // Codex reports no severity band
			Kind:     windowKind(window.LimitWindowSeconds, scope),
			Group:    scope,
			Percent:  window.UsedPercent,
			IsActive: active,
			ResetsAt: r.resetTime(*window),
		})
	}
	return limits
}

// resetTime returns when a window resets: the moment the endpoint names, or as far
// from now as it says, or the zero time when it says neither.
func (r *OpenAIUsageRepository) resetTime(window openAIWindow) time.Time {
	if window.ResetAt > 0 {
		return time.Unix(window.ResetAt, 0)
	}
	if window.ResetAfterSeconds > 0 {
		return r.now().Add(time.Duration(window.ResetAfterSeconds) * time.Second)
	}
	return time.Time{}
}

// windowKind names a window the way ccswitch labels limits: by the length of the
// window -- "5-hour", "7-day", "30-day" -- followed by the scope it applies to when it
// is not the main limit, as in "7-day review".
func windowKind(lengthSeconds int64, scope string) string {
	length := windowLength(lengthSeconds)
	if scope == "" {
		return length
	}
	return length + " " + scope
}

// windowLength phrases the length of a window in its largest whole unit, rounding a
// length of odd seconds up to the minute as Codex does.
func windowLength(seconds int64) string {
	switch {
	case seconds <= 0:
		return unknownWindowLabel
	case seconds%secondsPerDay == 0:
		return strconv.FormatInt(seconds/secondsPerDay, 10) + "-day"
	case seconds%secondsPerHour == 0:
		return strconv.FormatInt(seconds/secondsPerHour, 10) + "-hour"
	default:
		return strconv.FormatInt((seconds+secondsPerMinute-1)/secondsPerMinute, 10) + "-minute"
	}
}
