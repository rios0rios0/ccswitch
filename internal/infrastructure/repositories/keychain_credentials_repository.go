//go:build darwin

package repositories

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

const (
	// KeychainServiceName is the generic-password service under which Claude Code
	// stores its credentials in the macOS login keychain.
	KeychainServiceName = "Claude Code-credentials"

	// securityStdinLimit is the longest single command line that `security -i`
	// accepts on stdin. Longer lines are SILENTLY TRUNCATED: the item is written
	// with a partial payload and the exit status cannot be relied on to report it.
	// Anything above this must be passed through argv instead.
	securityStdinLimit = 4032

	// securityNotFoundExitCode is the status `security` exits with for
	// errSecItemNotFound, i.e. no such item in the keychain. It is the only failure
	// that means the item is genuinely absent rather than merely unreadable.
	securityNotFoundExitCode = 44

	securityTimeout = 10 * time.Second
)

// ErrKeychainItemNotFound reports that the keychain holds no such item, as
// opposed to holding one that could not be read. The difference decides whether a
// write may start from an empty document, so it must never be inferred from "the
// read failed".
var ErrKeychainItemNotFound = errors.New("keychain item not found")

// SecurityRunner executes the macOS `security` tool. It is an interface so tests
// can exercise the repository without touching the real login keychain.
type SecurityRunner interface {
	// Run executes `security` with the given arguments, optionally feeding stdin,
	// and returns its standard output.
	Run(stdin []byte, args ...string) ([]byte, error)
}

// ExecSecurityRunner runs the real `security` binary.
type ExecSecurityRunner struct{}

// Run executes `security` with the given arguments, optionally feeding stdin.
func (ExecSecurityRunner) Run(stdin []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), securityTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "security", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == securityNotFoundExitCode {
			return nil, fmt.Errorf("%w: security %s: %s", ErrKeychainItemNotFound, args[0], detail)
		}
		return nil, fmt.Errorf("security %s failed: %w: %s", args[0], err, detail)
	}
	return out, nil
}

// KeychainCredentialsRepository reads and writes the Claude Code credentials from
// the macOS login keychain, which is where Claude Code stores them on this
// platform. Its ~/.claude/.credentials.json file is only a fallback, consulted
// when the keychain read returns nothing, so writing that file has no effect
// while a keychain item exists.
//
// The oauthAccount identity still lives in the Claude state file, so it is kept
// in sync exactly as the file-based repository does.
type KeychainCredentialsRepository struct {
	service string
	account string
	runner  SecurityRunner
	state   claudeStateFile
}

// NewKeychainCredentialsRepository creates a repository targeting the Claude Code
// keychain item of the current user via the real `security` tool.
func NewKeychainCredentialsRepository(claudeJSONPath string) *KeychainCredentialsRepository {
	return NewKeychainCredentialsRepositoryWithRunner(
		claudeJSONPath, KeychainServiceName, currentUsername(), ExecSecurityRunner{})
}

// NewKeychainCredentialsRepositoryWithRunner creates a repository targeting an
// explicit keychain item through the given runner. It exists so tests can drive
// the read-modify-write logic without touching the real login keychain.
func NewKeychainCredentialsRepositoryWithRunner(
	claudeJSONPath, service, account string,
	runner SecurityRunner,
) *KeychainCredentialsRepository {
	return &KeychainCredentialsRepository{
		service: service,
		account: account,
		runner:  runner,
		state:   claudeStateFile{path: claudeJSONPath},
	}
}

// currentUsername returns the account name Claude Code files its keychain item
// under, which is the current user's login name.
func currentUsername() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// Read returns the currently installed OAuth credentials and, when available, the
// active account identity taken from the Claude state file.
func (r *KeychainCredentialsRepository) Read() (*entities.OAuthCredentials, *entities.AccountIdentity, error) {
	blob, err := r.readBlob()
	if err != nil {
		return nil, nil, err
	}

	var envelope struct {
		ClaudeAiOauth entities.OAuthCredentials `json:"claudeAiOauth"`
	}
	if err = json.Unmarshal(blob, &envelope); err != nil {
		return nil, nil, fmt.Errorf("failed to parse keychain item %q: %w", r.service, err)
	}

	creds := envelope.ClaudeAiOauth
	return &creds, r.state.readIdentity(), nil
}

// Write installs the given credentials into the keychain item and, best effort,
// updates the oauthAccount block of the Claude state file.
//
// The write is a read-modify-write because the keychain item is a single JSON
// document that carries more than the OAuth tokens: it also holds mcpOAuth, the
// access tokens of every authenticated MCP server. Marshalling only
// claudeAiOauth would erase all of them on every rotation, so whatever is
// already stored is preserved verbatim and only claudeAiOauth is replaced.
func (r *KeychainCredentialsRepository) Write(
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

	data, err := json.Marshal(root)
	if err != nil {
		return fmt.Errorf("failed to marshal keychain payload: %w", err)
	}
	if err = r.writeBlob(data); err != nil {
		return err
	}

	r.state.patchIdentity(identity)
	return nil
}

// Exists reports whether the Claude Code keychain item is present and readable.
func (r *KeychainCredentialsRepository) Exists() bool {
	_, err := r.readBlob()
	return err == nil
}

// readRoot returns the stored keychain document as a key-preserving map, or an
// empty map when no item exists yet. It refuses to return an empty map for an
// item that exists but cannot be read or parsed, so that a payload that is merely
// unavailable is never silently replaced — losing mcpOAuth that way would sign
// the user out of every MCP server.
func (r *KeychainCredentialsRepository) readRoot() (map[string]json.RawMessage, error) {
	root := map[string]json.RawMessage{}

	blob, err := r.readBlob()
	if err != nil {
		// Only a genuine "no such item" means a fresh install that may start from an
		// empty document. Every other failure — a locked keychain, a denied access
		// prompt, a timeout — leaves an item that probably does exist, and merging
		// into an empty document would erase whatever mcpOAuth tokens it holds.
		if errors.Is(err, ErrKeychainItemNotFound) {
			return root, nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(blob)) == 0 {
		return root, nil
	}
	if err = json.Unmarshal(blob, &root); err != nil {
		return nil, fmt.Errorf(
			"refusing to overwrite unparsable keychain item %q (it may hold MCP tokens): %w",
			r.service, err)
	}
	return root, nil
}

// readBlob returns the raw JSON document stored in the keychain item.
func (r *KeychainCredentialsRepository) readBlob() ([]byte, error) {
	out, err := r.runner.Run(nil, "find-generic-password", "-a", r.account, "-s", r.service, "-w")
	if err != nil {
		return nil, fmt.Errorf("failed to read keychain item %q: %w", r.service, err)
	}
	return decodeSecurityPassword(out), nil
}

// writeBlob stores the document in the keychain item, creating or updating it.
func (r *KeychainCredentialsRepository) writeBlob(data []byte) error {
	payload := hex.EncodeToString(data)

	// Prefer stdin so the payload never reaches this process's argv, which is
	// readable from the process list. `security -i` truncates long lines without
	// a reliable error, so only use it while the whole line fits.
	line := fmt.Sprintf("add-generic-password -U -a %q -s %q -X %q\n", r.account, r.service, payload)
	if len(line) <= securityStdinLimit {
		if _, err := r.runner.Run([]byte(line), "-i"); err != nil {
			return fmt.Errorf("failed to write keychain item %q: %w", r.service, err)
		}
		return r.verifyBlob(data)
	}

	// Above the stdin limit the payload has to go through argv, where it is
	// briefly visible in this user's own process list. Claude Code makes the same
	// trade-off for large payloads, and the Claude Code document is always large
	// enough to land here once any MCP server has been authenticated.
	if _, err := r.runner.Run(nil,
		"add-generic-password", "-U", "-a", r.account, "-s", r.service, "-X", payload); err != nil {
		return fmt.Errorf("failed to write keychain item %q: %w", r.service, err)
	}
	return r.verifyBlob(data)
}

// verifyBlob reads the item back and confirms it matches what was written. This
// guards against `security` reporting success for a truncated payload, which
// would leave the user both logged out and stripped of every MCP token.
func (r *KeychainCredentialsRepository) verifyBlob(want []byte) error {
	got, err := r.readBlob()
	if err != nil {
		return fmt.Errorf("failed to verify keychain item %q after write: %w", r.service, err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		return fmt.Errorf(
			"keychain item %q was stored truncated or altered (wrote %d bytes, read back %d)",
			r.service, len(want), len(bytes.TrimSpace(got)))
	}
	return nil
}

// decodeSecurityPassword normalizes the output of `security -w`, which prints the
// stored value either as text or, for some items, as a hex dump. A JSON document
// can never be mistaken for hex because it starts with "{".
func decodeSecurityPassword(out []byte) []byte {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 || len(trimmed)%2 != 0 {
		return trimmed
	}
	decoded, err := hex.DecodeString(string(trimmed))
	if err != nil {
		return trimmed
	}
	return decoded
}
