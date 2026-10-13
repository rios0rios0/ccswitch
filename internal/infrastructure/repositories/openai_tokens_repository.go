package repositories

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// errNoAccessToken reports a refresh the token endpoint answered without an access
// token, which must not replace the credentials it was asked to refresh.
var errNoAccessToken = errors.New("token endpoint returned no access token")

// openAIRefreshRequest is the refresh grant Codex sends, as JSON.
type openAIRefreshRequest struct {
	ClientID     string `json:"client_id"`
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
}

// openAIRefreshResponse is what the token endpoint answers a refresh with. Each token
// is restated only when the endpoint minted a new one.
type openAIRefreshResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// OpenAITokensRepository refreshes the tokens of a Codex sign-in against the OpenAI
// OAuth token endpoint, as Codex itself does.
type OpenAITokensRepository struct {
	client   *http.Client
	tokenURL string
	clientID string
	now      func() time.Time
}

// NewOpenAITokensRepository creates a repository targeting the given token URL and
// OAuth client id. When client is nil a default client is used.
func NewOpenAITokensRepository(tokenURL, clientID string, client *http.Client) *OpenAITokensRepository {
	if client == nil {
		client = &http.Client{Timeout: tokenHTTPTimeout} //nolint:exhaustruct // only Timeout is needed
	}
	return &OpenAITokensRepository{client: client, tokenURL: tokenURL, clientID: clientID, now: time.Now}
}

// Refresh mints new credentials from the given ones, preserving everything about the
// account that the token endpoint does not restate.
//
// The endpoint rotates the refresh token on every refresh and refuses the old one
// afterwards ("refresh_token_reused"), which is why the result has to reach Codex's
// auth.json as well as the store (see the commands' publishRefreshed).
func (r *OpenAITokensRepository) Refresh(creds entities.OAuthCredentials) (*entities.OAuthCredentials, error) {
	payload, err := json.Marshal(openAIRefreshRequest{ //nolint:gosec // the refresh grant has to carry the token
		ClientID:     r.clientID,
		GrantType:    grantRefresh,
		RefreshToken: creds.RefreshToken,
	})
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
	req.Header.Set("User-Agent", userAgent)

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

	var parsed openAIRefreshResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	if parsed.AccessToken == "" {
		return nil, errNoAccessToken
	}
	return r.toCredentials(parsed, creds), nil
}

// toCredentials converts a refresh response into domain credentials, merged onto the
// ones it replaces so that the refresh token and the ID token survive a response that
// does not restate them -- Codex cannot load a sign-in without its ID token. The
// expiry is read out of the new access token, and the plan out of a new ID token.
func (r *OpenAITokensRepository) toCredentials(
	resp openAIRefreshResponse,
	prior entities.OAuthCredentials,
) *entities.OAuthCredentials {
	minted := entities.OAuthCredentials{ //nolint:exhaustruct // the merge below fills the rest
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		IDToken:      resp.IDToken,
	}
	// Without an expiry in the token or the response, the token is treated as fresh for
	// as long as Codex would treat it; an expiry of zero would read as spent and refresh
	// it again on every poll.
	switch access, err := parseOpenAIClaims(resp.AccessToken); {
	case err == nil && access.expiresAtMillis() != 0:
		minted.ExpiresAt = access.expiresAtMillis()
	case resp.ExpiresIn > 0:
		minted.ExpiresAt = r.now().UnixMilli() + resp.ExpiresIn*millisPerSecond
	default:
		minted.ExpiresAt = r.now().Add(codexRefreshInterval).UnixMilli()
	}
	if id, err := parseOpenAIClaims(resp.IDToken); err == nil {
		minted.SubscriptionType = id.Auth.PlanType
		minted.AccountID = id.Auth.AccountID
	}

	merged := prior.WithRefreshed(minted)
	return &merged
}
