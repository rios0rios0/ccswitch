package repositories

import (
	"encoding/json"
	"os"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// claudeStateFile reads and patches the oauthAccount block of the Claude Code
// state file (~/.claude.json). It is shared by every CredentialsRepository
// implementation because the active account's identity lives in that file on all
// platforms, independently of where the tokens themselves are stored.
type claudeStateFile struct {
	path string
}

// readIdentity extracts the oauthAccount identity from the Claude state file,
// returning nil when it is absent or unreadable.
func (s claudeStateFile) readIdentity() *entities.AccountIdentity {
	data, err := os.ReadFile(s.path)
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
// update is cosmetic: the credentials are what authenticate the CLI.
func (s claudeStateFile) patchIdentity(identity *entities.AccountIdentity) {
	if identity == nil || identity.EmailAddress == "" {
		return
	}

	// Start from the existing state file when present, so unrelated keys are
	// preserved; start empty when it is absent so a fresh install still gets an
	// oauthAccount. Bail only when the file exists but cannot be parsed, to avoid
	// clobbering it.
	root := map[string]json.RawMessage{}
	if data, err := os.ReadFile(s.path); err == nil {
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
		_ = atomicWrite(s.path, out, filePerm)
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
