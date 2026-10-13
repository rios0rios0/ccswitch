package repositories

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

const (
	// codexAuthFileName is the file under Codex's home that holds its sign-in, unless
	// Codex was told to keep it in the system keyring instead.
	codexAuthFileName = "auth.json"
	// The keys of auth.json that ccswitch reads or writes. Every other key is carried
	// across a write untouched.
	codexAuthModeKey    = "auth_mode"
	codexAPIKeyKey      = "OPENAI_API_KEY" //nolint:gosec // the name of a JSON key, not a secret
	codexTokensKey      = "tokens"
	codexLastRefreshKey = "last_refresh"
	// codexChatGPTMode is the auth mode of a ChatGPT sign-in, the only kind ccswitch
	// rotates.
	codexChatGPTMode = "chatgpt"
	// codexRefreshInterval is how long Codex treats tokens as fresh after it last
	// refreshed them when the access token names no expiry of its own.
	codexRefreshInterval = 8 * 24 * time.Hour
)

// ErrCodexNotChatGPT reports a Codex that is signed in some other way than with a
// ChatGPT account -- an API key, say -- which ccswitch neither reads nor replaces.
var ErrCodexNotChatGPT = errors.New("codex is not signed in with a ChatGPT account")

// errCodexNoIDToken reports credentials that cannot be installed for Codex.
var errCodexNoIDToken = errors.New(
	"refusing to install Codex credentials without an ID token, which Codex cannot load a sign-in without")

// codexAuthDocument mirrors the parts of Codex's auth.json that tell whose sign-in it
// holds.
type codexAuthDocument struct {
	AuthMode string `json:"auth_mode"`
	// APIKey is a pointer because Codex writes it as null for a ChatGPT sign-in.
	APIKey      *string      `json:"OPENAI_API_KEY"`
	Tokens      *codexTokens `json:"tokens"`
	LastRefresh string       `json:"last_refresh"`
}

// codexTokens mirrors the "tokens" object of a ChatGPT sign-in in auth.json.
type codexTokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}

// CodexCredentialsRepository reads and writes the ChatGPT sign-in that the Codex CLI
// keeps in auth.json under its home directory: ~/.codex, or wherever CODEX_HOME points.
// Codex keeps it there on every platform unless its cli_auth_credentials_store setting
// moves it to the system keyring, which ccswitch does not reach.
//
// The account's identity travels inside the ID token, so unlike Claude Code there is
// no second file to keep in step with the tokens.
type CodexCredentialsRepository struct {
	path string
	now  func() time.Time
}

// NewCodexCredentialsRepository creates a repository over the auth.json in the given
// Codex home directory.
func NewCodexCredentialsRepository(codexHome string) *CodexCredentialsRepository {
	return &CodexCredentialsRepository{path: filepath.Join(codexHome, codexAuthFileName), now: time.Now}
}

// Read returns the ChatGPT sign-in Codex holds, and the identity its ID token names.
// A Codex signed in some other way answers ErrCodexNotChatGPT.
func (r *CodexCredentialsRepository) Read() (*entities.OAuthCredentials, *entities.AccountIdentity, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read Codex auth file: %w", err)
	}

	var document codexAuthDocument
	if err = json.Unmarshal(data, &document); err != nil {
		return nil, nil, fmt.Errorf("failed to parse Codex auth file: %w", err)
	}
	if !isChatGPTSignIn(document) {
		return nil, nil, fmt.Errorf("%w (its auth mode is %q)", ErrCodexNotChatGPT, document.AuthMode)
	}
	if document.Tokens == nil {
		// A ChatGPT sign-in without tokens is signed out; blank credentials say so, and
		// every capture path refuses them.
		return &entities.OAuthCredentials{}, nil, nil
	}

	creds := codexCredentials(*document.Tokens, document.LastRefresh)
	return &creds, codexIdentity(*document.Tokens), nil
}

// Write installs the credentials as Codex's ChatGPT sign-in. The identity is not
// needed: Codex reads it out of the ID token.
//
// Like the writes of Claude Code's credentials, it is a read-modify-write: auth.json
// may hold more than the tokens -- an API key kept beside the sign-in, an agent
// identity -- so every other key is carried across untouched, as are any keys of the
// "tokens" object this version does not know. It refuses to replace a sign-in that is
// not a ChatGPT one, which the person chose deliberately, and to write credentials
// without an ID token, since Codex fails to load a sign-in that lacks one and would be
// left signed out.
func (r *CodexCredentialsRepository) Write(creds *entities.OAuthCredentials, _ *entities.AccountIdentity) error {
	if creds.IDToken == "" {
		return errCodexNoIDToken
	}
	root, err := r.readRoot()
	if err != nil {
		return err
	}
	if err = refuseOtherSignIn(root); err != nil {
		return err
	}

	tokens, err := mergeCodexTokens(root[codexTokensKey], creds)
	if err != nil {
		return err
	}
	root[codexTokensKey] = tokens
	root[codexAuthModeKey] = mustMarshalString(codexChatGPTMode)
	if _, ok := root[codexAPIKeyKey]; !ok {
		root[codexAPIKeyKey] = json.RawMessage("null")
	}
	root[codexLastRefreshKey] = mustMarshalString(r.refreshedAt(creds).UTC().Format(time.RFC3339Nano))

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Codex auth file: %w", err)
	}
	if err = atomicWrite(r.path, data); err != nil {
		return fmt.Errorf("failed to write Codex auth file: %w", err)
	}
	return nil
}

// Exists reports whether Codex's auth file is present.
func (r *CodexCredentialsRepository) Exists() bool {
	_, err := os.Stat(r.path)
	return err == nil
}

// readRoot returns auth.json as a key-preserving map, or an empty map when the file
// does not exist yet. A file that exists but cannot be read or parsed is an error
// rather than an empty map, so that a document which is merely unavailable is never
// silently replaced.
func (r *CodexCredentialsRepository) readRoot() (map[string]json.RawMessage, error) {
	root := map[string]json.RawMessage{}

	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return root, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read Codex auth file: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return root, nil
	}
	if err = json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("refusing to overwrite unparsable Codex auth file %q: %w", r.path, err)
	}
	return root, nil
}

// refreshedAt returns when the credentials were minted, which is what Codex records
// as its last refresh: the moment the access token was issued, or now when it does
// not say.
func (r *CodexCredentialsRepository) refreshedAt(creds *entities.OAuthCredentials) time.Time {
	if claims, err := parseOpenAIClaims(creds.AccessToken); err == nil && !claims.issuedAt().IsZero() {
		return claims.issuedAt()
	}
	return r.now()
}

// isChatGPTSignIn reports whether the document holds a ChatGPT sign-in. A document
// written before Codex recorded its auth mode is one unless it holds an API key and
// no tokens.
func isChatGPTSignIn(document codexAuthDocument) bool {
	if document.AuthMode != "" {
		return document.AuthMode == codexChatGPTMode
	}
	return document.Tokens != nil || document.APIKey == nil || *document.APIKey == ""
}

// refuseOtherSignIn returns ErrCodexNotChatGPT when the stored document holds a sign-in
// that is not a ChatGPT one.
func refuseOtherSignIn(root map[string]json.RawMessage) error {
	var document codexAuthDocument
	if raw, ok := root[codexAuthModeKey]; ok {
		if err := json.Unmarshal(raw, &document.AuthMode); err != nil {
			return fmt.Errorf("refusing to overwrite Codex auth file with an unreadable auth mode: %w", err)
		}
	}
	if raw, ok := root[codexAPIKeyKey]; ok {
		// A key that is not a string cannot be an API key Codex would sign in with.
		_ = json.Unmarshal(raw, &document.APIKey)
	}
	if raw, ok := root[codexTokensKey]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		document.Tokens = &codexTokens{}
	}
	if !isChatGPTSignIn(document) {
		return fmt.Errorf("refusing to replace Codex's sign-in: %w (its auth mode is %q)",
			ErrCodexNotChatGPT, document.AuthMode)
	}
	return nil
}

// mergeCodexTokens writes the credentials into the stored "tokens" object, keeping
// whatever else it holds.
func mergeCodexTokens(stored json.RawMessage, creds *entities.OAuthCredentials) (json.RawMessage, error) {
	tokens := map[string]json.RawMessage{}
	if len(stored) > 0 && !bytes.Equal(bytes.TrimSpace(stored), []byte("null")) {
		if err := json.Unmarshal(stored, &tokens); err != nil {
			return nil, fmt.Errorf("refusing to overwrite Codex auth file with unreadable tokens: %w", err)
		}
	}
	tokens["id_token"] = mustMarshalString(creds.IDToken)
	tokens["access_token"] = mustMarshalString(creds.AccessToken)
	tokens["refresh_token"] = mustMarshalString(creds.RefreshToken)
	tokens["account_id"] = json.RawMessage("null")
	if creds.AccountID != "" {
		tokens["account_id"] = mustMarshalString(creds.AccountID)
	}

	merged, err := json.Marshal(tokens)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Codex tokens: %w", err)
	}
	return merged, nil
}

// codexCredentials maps a ChatGPT sign-in onto credentials. The expiry and the plan are
// read out of the tokens, since auth.json records neither on its own; an access token
// that names no expiry is treated as fresh for as long as Codex would treat it.
func codexCredentials(tokens codexTokens, lastRefresh string) entities.OAuthCredentials {
	creds := entities.OAuthCredentials{ //nolint:exhaustruct // a ChatGPT sign-in has no scopes or tiers to record
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		IDToken:      tokens.IDToken,
		AccountID:    tokens.AccountID,
	}
	if access, err := parseOpenAIClaims(tokens.AccessToken); err == nil && access.expiresAtMillis() != 0 {
		creds.ExpiresAt = access.expiresAtMillis()
	} else if refreshed, parseErr := time.Parse(time.RFC3339Nano, lastRefresh); parseErr == nil {
		creds.ExpiresAt = refreshed.Add(codexRefreshInterval).UnixMilli()
	}
	if id, err := parseOpenAIClaims(tokens.IDToken); err == nil {
		creds.SubscriptionType = id.Auth.PlanType
		if creds.AccountID == "" {
			creds.AccountID = id.Auth.AccountID
		}
	}
	return creds
}

// codexIdentity returns whose sign-in the tokens are, from the ID token, or nil when
// they name nobody. The stable identity it matches enrolled accounts on is the seat --
// the person and the ChatGPT account they act for -- since every member of a workspace
// acts for the same account.
func codexIdentity(tokens codexTokens) *entities.AccountIdentity {
	id, _ := parseOpenAIClaims(tokens.IDToken)
	email := id.email()
	if email == "" {
		access, _ := parseOpenAIClaims(tokens.AccessToken)
		email = access.email()
	}
	accountID := tokens.AccountID
	if accountID == "" {
		accountID = id.Auth.AccountID
	}
	if email == "" && accountID == "" {
		return nil
	}
	return &entities.AccountIdentity{ //nolint:exhaustruct // ChatGPT names no organization for a sign-in
		EmailAddress: email,
		AccountUUID:  id.seat(accountID),
		DisplayName:  id.Name,
	}
}

// mustMarshalString encodes a string as a JSON value, which cannot fail.
func mustMarshalString(value string) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}
