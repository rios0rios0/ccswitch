package repositories

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rios0rios0/cliforge/pkg/selfupdate"
)

// ReleaseUpdater checks for and installs the releases of a CLI binary. cliforge's
// selfupdate.Command is the production implementation; it is an interface so tests
// can stand in for GitHub and for the binary swap.
type ReleaseUpdater interface {
	// Execute installs the latest release over the running binary when it is newer.
	Execute(dryRun, force bool) error
	// CheckForUpdates warns, in the background, when a newer release is out. It
	// counts a day as checked once a lookup has answered, and starts no more than a
	// few lookups a day.
	CheckForUpdates()
}

// CliforgeSelfUpdateRepository updates ccswitch from its GitHub releases through
// cliforge, the shared self-update library. The release assets it looks for are
// the ones the release pipeline's GoReleaser publishes:
// {binary}-{version}-{os}-{arch}.tar.gz, and .zip on Windows.
type CliforgeSelfUpdateRepository struct {
	updater    ReleaseUpdater
	executable func() (string, error)
}

// NewCliforgeSelfUpdateRepository creates a repository over the GitHub releases of
// owner/repo, whose assets carry the given binary name, for a binary built as
// version.
func NewCliforgeSelfUpdateRepository(owner, repo, binaryName, version string) *CliforgeSelfUpdateRepository {
	return NewCliforgeSelfUpdateRepositoryWithUpdater(
		selfupdate.NewCommand(owner, repo, binaryName, version),
		runningExecutable,
	)
}

// NewCliforgeSelfUpdateRepositoryWithUpdater creates a repository that drives the
// given updater over the binary that executable resolves, so tests can exercise it
// without reaching GitHub or replacing the test binary.
func NewCliforgeSelfUpdateRepositoryWithUpdater(
	updater ReleaseUpdater,
	executable func() (string, error),
) *CliforgeSelfUpdateRepository {
	return &CliforgeSelfUpdateRepository{updater: updater, executable: executable}
}

// Update installs the latest release over the running binary and reports whether
// it did.
//
// cliforge answers the same way whether it installed a release, found the binary
// already current, ran dry, or was turned down at its prompt, so whether a release
// went in is read off the binary itself: installing one leaves a different file at
// the executable's path.
func (r *CliforgeSelfUpdateRepository) Update(dryRun, force bool) (bool, error) {
	binary, err := r.executable()
	if err != nil {
		return false, err
	}
	before, err := identityOf(binary)
	if err != nil {
		return false, fmt.Errorf("failed to inspect the running binary: %w", err)
	}

	if err = r.updater.Execute(dryRun, force); err != nil {
		return false, err
	}
	return !stillAt(binary, before), nil
}

// CheckForUpdates warns when a newer release is out. cliforge runs the lookup in
// the background, so the command it accompanies is never held up, and skips
// development builds and binaries modified today. It counts a day as checked only
// once a lookup has answered, starts at most five lookups a day, and skips the
// lookup when it cannot keep that count.
func (r *CliforgeSelfUpdateRepository) CheckForUpdates() {
	r.updater.CheckForUpdates()
}

// identityOf captures the identity of the file at path through an open handle.
// On Windows [os.Stat] only records the path and reads the file's identity when
// [os.SameFile] compares it, by which time an update has put another file there,
// so a snapshot taken that way would always match the new binary.
func identityOf(path string) (os.FileInfo, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return file.Stat()
}

// stillAt reports whether the file at path is still the one before describes. A
// path that can no longer be read back does not hold it either.
func stillAt(path string, before os.FileInfo) bool {
	after, err := os.Stat(path)
	return err == nil && os.SameFile(before, after)
}

// runningExecutable resolves the binary this process runs from the way cliforge
// does before it replaces it: through any symlink, to the file itself.
func runningExecutable() (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to locate the running binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(binary)
	if err != nil {
		return "", fmt.Errorf("failed to resolve the running binary: %w", err)
	}
	return resolved, nil
}
