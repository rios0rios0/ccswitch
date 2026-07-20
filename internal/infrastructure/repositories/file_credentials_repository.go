package repositories

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// claudeCredentialsFile mirrors the on-disk shape of ~/.claude/.credentials.json.
type claudeCredentialsFile struct {
	ClaudeAiOauth entities.OAuthCredentials `json:"claudeAiOauth"`
}

// FileCredentialsRepository reads and writes the Claude Code credentials file and
// keeps the oauthAccount block of the Claude state file in sync for display.
type FileCredentialsRepository struct {
	credentialsPath string
	claudeJSONPath  string
}

// NewFileCredentialsRepository creates a FileCredentialsRepository for the given
// Claude Code file locations.
func NewFileCredentialsRepository(credentialsPath, claudeJSONPath string) *FileCredentialsRepository {
	return &FileCredentialsRepository{
		credentialsPath: credentialsPath,
		claudeJSONPath:  claudeJSONPath,
	}
}

// Read returns the currently installed OAuth credentials and, when available, the
// active account identity taken from the Claude state file.
func (r *FileCredentialsRepository) Read() (*entities.OAuthCredentials, *entities.AccountIdentity, error) {
	data, err := os.ReadFile(r.credentialsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var file claudeCredentialsFile
	if err = json.Unmarshal(data, &file); err != nil {
		return nil, nil, fmt.Errorf("failed to parse credentials file: %w", err)
	}

	creds := file.ClaudeAiOauth
	return &creds, r.readIdentity(), nil
}

// Write atomically installs the given credentials and best-effort updates the
// oauthAccount block of the Claude state file. A failure to update the state file
// is not fatal because the credentials are what authenticate the CLI.
func (r *FileCredentialsRepository) Write(
	creds *entities.OAuthCredentials,
	identity *entities.AccountIdentity,
) error {
	file := claudeCredentialsFile{ClaudeAiOauth: *creds}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}
	if err = atomicWrite(r.credentialsPath, data, filePerm); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	r.patchIdentity(identity)
	return nil
}

// Exists reports whether the credentials file is present.
func (r *FileCredentialsRepository) Exists() bool {
	_, err := os.Stat(r.credentialsPath)
	return err == nil
}

// readIdentity extracts the oauthAccount identity from the Claude state file,
// returning nil when it is absent or unreadable.
func (r *FileCredentialsRepository) readIdentity() *entities.AccountIdentity {
	data, err := os.ReadFile(r.claudeJSONPath)
	if err != nil {
		return nil
	}
	var envelope struct {
		OAuthAccount *entities.AccountIdentity `json:"oauthAccount"`
	}
	if err = json.Unmarshal(data, &envelope); err != nil {
		return nil
	}
	return envelope.OAuthAccount
}

// patchIdentity merges the given identity into the oauthAccount block of the
// Claude state file, preserving all other keys. Errors are swallowed because the
// update is cosmetic.
func (r *FileCredentialsRepository) patchIdentity(identity *entities.AccountIdentity) {
	if identity == nil || identity.EmailAddress == "" {
		return
	}

	// Start from the existing state file when present, so unrelated keys are
	// preserved; start empty when it is absent so a fresh install still gets an
	// oauthAccount. Bail only when the file exists but cannot be parsed, to avoid
	// clobbering it.
	root := map[string]json.RawMessage{}
	if data, err := os.ReadFile(r.claudeJSONPath); err == nil {
		if err = json.Unmarshal(data, &root); err != nil {
			return
		}
	}

	account := map[string]json.RawMessage{}
	if raw, ok := root["oauthAccount"]; ok {
		_ = json.Unmarshal(raw, &account)
	}
	setStringField(account, "emailAddress", identity.EmailAddress)
	setStringField(account, "accountUuid", identity.AccountUUID)
	setStringField(account, "displayName", identity.DisplayName)
	setStringField(account, "organizationName", identity.OrganizationName)

	if merged, mErr := json.Marshal(account); mErr == nil {
		root["oauthAccount"] = merged
	}
	if out, mErr := json.MarshalIndent(root, "", "  "); mErr == nil {
		_ = atomicWrite(r.claudeJSONPath, out, filePerm)
	}
}

// setStringField sets a JSON string value on the map when the value is non-empty.
func setStringField(target map[string]json.RawMessage, key, value string) {
	if value == "" {
		return
	}
	if encoded, err := json.Marshal(value); err == nil {
		target[key] = encoded
	}
}
