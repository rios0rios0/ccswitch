package repositories

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	Scope        string `json:"scope"`
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	// Scope is the space-delimited scope list the minted access token carries.
	Scope string `json:"scope"`
	// RefreshTokenExpiresIn is the lifetime of the refresh token in seconds, sent
	// only when the endpoint rotated one.
	RefreshTokenExpiresIn *int64 `json:"refresh_token_expires_in"`
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

// Refresh mints new credentials from the given ones, preserving everything about
// the account that the token endpoint does not restate.
func (r *AnthropicTokensRepository) Refresh(
	creds entities.OAuthCredentials,
) (*entities.OAuthCredentials, error) {
	requested := requestScopes(creds)
	request := refreshRequest{
		GrantType:    grantRefresh,
		RefreshToken: creds.RefreshToken,
		ClientID:     r.clientID,
		Scope:        strings.Join(requested, " "),
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
	return r.toCredentials(parsed, creds, requested), nil
}

// requestScopes returns the scopes to ask the new access token to carry: the
// account's own when they satisfy the invariant, and Claude Code's subscription
// set when they do not. Sending none lets the endpoint decide, and a token that
// comes back without `user:inference` is one Claude Code discards as
// not-claude.ai.
//
// Falling back for a degraded set — not only an empty one — is what lets the
// repair widen it. Echoing a narrowed set back would ask the endpoint to mint
// exactly the token that needs repairing, leaving the account stuck there.
func requestScopes(creds entities.OAuthCredentials) []string {
	if !creds.Degraded() {
		return creds.Scopes
	}
	return claudeCodeScopes()
}

// claudeCodeScopes returns the scope set a Claude Code subscription login
// carries, in the order Claude Code itself requests them.
func claudeCodeScopes() []string {
	return []string{
		"user:profile",
		"user:inference",
		"user:sessions:claude_code",
		"user:mcp_servers",
		"user:file_upload",
	}
}

// toCredentials converts a refresh response into domain credentials, merged onto
// the ones it replaces so that the refresh token, the scopes, the tiers, and the
// refresh-token expiry survive a response that does not restate them.
func (r *AnthropicTokensRepository) toCredentials(
	resp refreshResponse,
	prior entities.OAuthCredentials,
	requested []string,
) *entities.OAuthCredentials {
	nowMillis := r.now().UnixMilli()
	granted := strings.Fields(resp.Scope)
	minted := entities.OAuthCredentials{ //nolint:exhaustruct // the merge below fills the rest
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    nowMillis + resp.ExpiresIn*millisPerSecond,
		Scopes:       granted,
	}
	if resp.RefreshTokenExpiresIn != nil {
		minted.RefreshTokenExpiresAt = nowMillis + *resp.RefreshTokenExpiresIn*millisPerSecond
	}

	merged := prior.WithRefreshed(minted)
	if len(granted) == 0 && merged.Degraded() {
		// The endpoint did not say what it minted, and the set carried over from the
		// prior credentials fails the scope invariant. What was asked for is the
		// better record than a stale narrowed set: keeping the latter would leave
		// the account degraded and refresh it on every poll without ever widening.
		//
		// This only applies when the response is silent. A response that does name
		// its scopes is the truth even when that truth is a narrowed set — recording
		// the request there would claim a scope the token does not carry.
		merged.Scopes = requested
	}
	return &merged
}
