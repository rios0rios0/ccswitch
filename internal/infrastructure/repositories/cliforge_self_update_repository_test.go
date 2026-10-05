package repositories_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

// fakeReleaseUpdater stands in for cliforge: it runs install where cliforge would
// download and swap in a release, and records how it was driven.
type fakeReleaseUpdater struct {
	install      func() error
	dryRun       bool
	force        bool
	executeCalls int
	checkCalls   int
}

func (f *fakeReleaseUpdater) Execute(dryRun, force bool) error {
	f.executeCalls++
	f.dryRun = dryRun
	f.force = force
	if f.install == nil {
		return nil
	}
	return f.install()
}

func (f *fakeReleaseUpdater) CheckForUpdates() {
	f.checkCalls++
}

// runningBinary creates the file standing in for the binary this process runs
// from, and returns its path.
func runningBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ccswitch")
	require.NoError(t, os.WriteFile(path, []byte("current release"), 0o600))
	return path
}

// executableAt resolves the running binary to path.
func executableAt(path string) func() (string, error) {
	return func() (string, error) { return path, nil }
}

// swapBinary installs a release at path the way cliforge does: the new file is
// written apart from the old one and moved over it.
func swapBinary(path string) func() error {
	return func() error {
		next := path + ".next"
		if err := os.WriteFile(next, []byte("latest release"), 0o600); err != nil {
			return err
		}
		return os.Rename(next, path)
	}
}

func TestCliforgeSelfUpdateRepositoryUpdate(t *testing.T) {
	t.Parallel()

	t.Run("should report the release installed when it replaced the binary", func(t *testing.T) {
		t.Parallel()
		// given
		binary := runningBinary(t)
		updater := &fakeReleaseUpdater{install: swapBinary(binary)}
		repo := repositories.NewCliforgeSelfUpdateRepositoryWithUpdater(updater, executableAt(binary))

		// when
		installed, err := repo.Update(false, true)

		// then
		require.NoError(t, err)
		assert.True(t, installed)
		assert.Equal(t, 1, updater.executeCalls)
		assert.False(t, updater.dryRun)
		assert.True(t, updater.force)
	})

	t.Run("should report nothing installed when the binary was left in place", func(t *testing.T) {
		t.Parallel()
		// given -- a dry run, a binary already current and a declined prompt all leave
		// the file where it was
		binary := runningBinary(t)
		updater := &fakeReleaseUpdater{}
		repo := repositories.NewCliforgeSelfUpdateRepositoryWithUpdater(updater, executableAt(binary))

		// when
		installed, err := repo.Update(true, false)

		// then
		require.NoError(t, err)
		assert.False(t, installed)
		assert.True(t, updater.dryRun)
		assert.False(t, updater.force)
	})

	t.Run("should report the release installed when the binary can no longer be read back", func(t *testing.T) {
		t.Parallel()
		// given
		binary := runningBinary(t)
		updater := &fakeReleaseUpdater{install: func() error { return os.Remove(binary) }}
		repo := repositories.NewCliforgeSelfUpdateRepositoryWithUpdater(updater, executableAt(binary))

		// when
		installed, err := repo.Update(false, true)

		// then
		require.NoError(t, err)
		assert.True(t, installed)
	})

	t.Run("should return the error the update failed with", func(t *testing.T) {
		t.Parallel()
		// given
		failure := errors.New("fixture download failure")
		binary := runningBinary(t)
		updater := &fakeReleaseUpdater{install: func() error { return failure }}
		repo := repositories.NewCliforgeSelfUpdateRepositoryWithUpdater(updater, executableAt(binary))

		// when
		installed, err := repo.Update(false, true)

		// then
		require.ErrorIs(t, err, failure)
		assert.False(t, installed)
	})

	t.Run("should fail without updating when the running binary cannot be inspected", func(t *testing.T) {
		t.Parallel()
		// given
		updater := &fakeReleaseUpdater{}
		missing := filepath.Join(t.TempDir(), "ccswitch")
		repo := repositories.NewCliforgeSelfUpdateRepositoryWithUpdater(updater, executableAt(missing))

		// when
		installed, err := repo.Update(false, true)

		// then
		require.Error(t, err)
		assert.False(t, installed)
		assert.Zero(t, updater.executeCalls)
	})

	t.Run("should fail without updating when the running binary cannot be located", func(t *testing.T) {
		t.Parallel()
		// given
		failure := errors.New("fixture executable failure")
		updater := &fakeReleaseUpdater{}
		repo := repositories.NewCliforgeSelfUpdateRepositoryWithUpdater(
			updater, func() (string, error) { return "", failure },
		)

		// when
		installed, err := repo.Update(false, true)

		// then
		require.ErrorIs(t, err, failure)
		assert.False(t, installed)
		assert.Zero(t, updater.executeCalls)
	})
}

func TestCliforgeSelfUpdateRepositoryCheckForUpdates(t *testing.T) {
	t.Parallel()

	t.Run("should hand the check to the updater", func(t *testing.T) {
		t.Parallel()
		// given
		updater := &fakeReleaseUpdater{}
		repo := repositories.NewCliforgeSelfUpdateRepositoryWithUpdater(updater, executableAt(runningBinary(t)))

		// when
		repo.CheckForUpdates()

		// then
		assert.Equal(t, 1, updater.checkCalls)
		assert.Zero(t, updater.executeCalls)
	})
}
