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

	r.state.patchIdentity(identity)
	return nil
}

// Exists reports whether the credentials file is present.
func (r *FileCredentialsRepository) Exists() bool {
	_, err := os.Stat(r.credentialsPath)
	return err == nil
}
