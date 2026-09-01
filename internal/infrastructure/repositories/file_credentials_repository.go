package repositories

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// credentialsJSONKey is the key holding the Claude Code OAuth tokens inside the
// credential store, whether that store is the file or the macOS keychain item.
const credentialsJSONKey = "claudeAiOauth"

// claudeCredentialsFile mirrors the on-disk shape of ~/.claude/.credentials.json.
type claudeCredentialsFile struct {
	ClaudeAiOauth entities.OAuthCredentials `json:"claudeAiOauth"`
}

// FileCredentialsRepository reads and writes the Claude Code credentials file and
// keeps the oauthAccount block of the Claude state file in sync for display.
type FileCredentialsRepository struct {
	credentialsPath string
	state           claudeStateFile
}

// NewFileCredentialsRepository creates a FileCredentialsRepository for the given
// Claude Code file locations.
func NewFileCredentialsRepository(credentialsPath, claudeJSONPath string) *FileCredentialsRepository {
	return &FileCredentialsRepository{
		credentialsPath: credentialsPath,
		state:           claudeStateFile{path: claudeJSONPath},
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
	return &creds, r.state.readIdentity(), nil
}

// Write atomically installs the given credentials and best-effort updates the
// oauthAccount block of the Claude state file. A failure to update the state file
// is not fatal because the credentials are what authenticate the CLI.
//
// Like its keychain counterpart the write is a read-modify-write, because
// claudeAiOauth is not the only thing the file holds: alongside it sit mcpOAuth,
// the tokens of every authenticated MCP server, and designOauth. Marshalling only
// claudeAiOauth over the file would sign the user out of all of them on every
// rotation, so the stored document is preserved verbatim and only claudeAiOauth
// is replaced.
func (r *FileCredentialsRepository) Write(
	creds *entities.OAuthCredentials,
	identity *entities.AccountIdentity,
) error {
	root, err := r.readRoot()
	if err != nil {
		return err
	}

	// gosec flags marshalling a struct whose JSON keys look like secrets. They are
	// secrets, and serializing them is the point: this is the credential store
	// Claude Code reads them back from.
	encoded, err := json.Marshal(creds) //nolint:gosec // G117: writing tokens to the credential store is the purpose
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}
	root[credentialsJSONKey] = encoded

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}
	if err = atomicWrite(r.credentialsPath, data, filePerm); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	r.state.patchIdentity(identity)
	return nil
}

// Exists reports whether the credentials file is present.
func (r *FileCredentialsRepository) Exists() bool {
	_, err := os.Stat(r.credentialsPath)
	return err == nil
}

// readRoot returns the stored credentials document as a key-preserving map, or an
// empty map when the file does not exist yet. A file that exists but cannot be
// read or parsed is an error rather than an empty map, so that a document which
// is merely unavailable is never silently replaced.
func (r *FileCredentialsRepository) readRoot() (map[string]json.RawMessage, error) {
	root := map[string]json.RawMessage{}

	data, err := os.ReadFile(r.credentialsPath)
	if errors.Is(err, os.ErrNotExist) {
		return root, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return root, nil
	}
	if err = json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf(
			"refusing to overwrite unparsable credentials file %q (it may hold MCP tokens): %w",
			r.credentialsPath, err)
	}
	return root, nil
}
