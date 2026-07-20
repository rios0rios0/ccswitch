// Package repositories contains the infrastructure adapters that implement the
// domain repository ports over the filesystem and HTTP.
package repositories

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	filePerm = 0o600
	dirPerm  = 0o700
)

// atomicWrite writes data to path via a temporary file in the same directory
// followed by a rename, so a reader never observes a partially written file. The
// parent directory is created when missing.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".ccswitch-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to set temp file mode: %w", err)
	}
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to replace %q: %w", path, err)
	}
	return nil
}
