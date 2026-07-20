package repositories

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

const (
	tokenHTTPTimeout = 10 * time.Second
	millisPerSecond  = 1000
	grantRefresh     = "refresh_token"
)

type refreshRequest struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// AnthropicTokensRepository refreshes OAuth access tokens against the Claude OAuth
// token endpoint.
type AnthropicTokensRepository struct {
	client   *http.Client
	tokenURL string
	clientID string
	now      func() time.Time
}

// NewAnthropicTokensRepository creates a repository targeting the given token URL
// and OAuth client id. When client is nil a default client is used.
func NewAnthropicTokensRepository(
	tokenURL, clientID string,
	client *http.Client,
) *AnthropicTokensRepository {
	if client == nil {
		client = &http.Client{Timeout: tokenHTTPTimeout} //nolint:exhaustruct // only Timeout is needed
	}
	return &AnthropicTokensRepository{
		client:   client,
		tokenURL: tokenURL,
		clientID: clientID,
		now:      time.Now,
	}
}

// Refresh mints new credentials from the given refresh token.
func (r *AnthropicTokensRepository) Refresh(refreshToken string) (*entities.OAuthCredentials, error) {
	request := refreshRequest{
		GrantType:    grantRefresh,
		RefreshToken: refreshToken,
		ClientID:     r.clientID,
	}
	payload, err := json.Marshal(request) //nolint:gosec // OAuth refresh grant needs the refresh token
	if err != nil {
		return nil, fmt.Errorf("failed to marshal refresh request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), tokenHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.tokenURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call token endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed refreshResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	return r.toCredentials(parsed, refreshToken), nil
}

// toCredentials converts a refresh response into domain credentials, carrying the
// prior refresh token forward when the server does not rotate it.
func (r *AnthropicTokensRepository) toCredentials(
	resp refreshResponse,
	priorRefresh string,
) *entities.OAuthCredentials {
	refresh := resp.RefreshToken
	if refresh == "" {
		refresh = priorRefresh
	}
	return &entities.OAuthCredentials{
		AccessToken:  resp.AccessToken,
		RefreshToken: refresh,
		ExpiresAt:    r.now().UnixMilli() + resp.ExpiresIn*millisPerSecond,
	}
}
