package controllers_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/test/doubles"
)

// recordRunningDaemon writes the pidfile kept beside the store with this test's
// own process id, which is alive, standing in for a monitor daemon that runs.
func recordRunningDaemon(t *testing.T, storePath string) int {
	t.Helper()
	pid := os.Getpid()
	pidPath := filepath.Join(filepath.Dir(storePath), "monitor.pid")
	require.NoError(t, os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0o600))
	return pid
}

func TestSelfUpdateCLI(t *testing.T) {
	t.Parallel()

	t.Run("should pass --dry-run and --force on to the release channel", func(t *testing.T) {
		t.Parallel()
		// given
		updates := &doubles.StubSelfUpdateRepository{}
		path := filepath.Join(t.TempDir(), "store.json")

		// when
		err := executeCLIWith(path, updates, io.Discard, io.Discard, "self-update", "--dry-run", "--force")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, updates.UpdateCalls)
		assert.True(t, updates.DryRun)
		assert.True(t, updates.Force)
	})

	t.Run("should fail when the release cannot be installed", func(t *testing.T) {
		t.Parallel()
		// given
		failure := errors.New("fixture release failure")
		updates := &doubles.StubSelfUpdateRepository{UpdateErr: failure}
		path := filepath.Join(t.TempDir(), "store.json")

		// when
		err := executeCLIWith(path, updates, io.Discard, io.Discard, "self-update", "--force")

		// then
		require.ErrorIs(t, err, failure)
	})

	t.Run("should reject arguments without updating", func(t *testing.T) {
		t.Parallel()
		// given
		updates := &doubles.StubSelfUpdateRepository{}
		path := filepath.Join(t.TempDir(), "store.json")

		// when
		err := executeCLIWith(path, updates, io.Discard, io.Discard, "self-update", "latest")

		// then
		require.Error(t, err)
		assert.Zero(t, updates.UpdateCalls)
	})

	t.Run("should tell the user to restart a daemon left running the replaced binary", func(t *testing.T) {
		t.Parallel()
		// given
		var errOut bytes.Buffer
		updates := &doubles.StubSelfUpdateRepository{Installed: true}
		path := filepath.Join(t.TempDir(), "store.json")
		pid := recordRunningDaemon(t, path)

		// when
		err := executeCLIWith(path, updates, io.Discard, &errOut, "self-update", "--force")

		// then
		require.NoError(t, err)
		assert.Contains(t, errOut.String(),
			"the monitor daemon (pid "+strconv.Itoa(pid)+") is still running the previous version")
		assert.Contains(t, errOut.String(), "ccswitch monitor --ensure-daemon")
	})

	t.Run("should not mention the daemon when none is running", func(t *testing.T) {
		t.Parallel()
		// given
		var errOut bytes.Buffer
		updates := &doubles.StubSelfUpdateRepository{Installed: true}
		path := filepath.Join(t.TempDir(), "store.json")

		// when
		err := executeCLIWith(path, updates, io.Discard, &errOut, "self-update", "--force")

		// then
		require.NoError(t, err)
		assert.Empty(t, errOut.String())
	})

	t.Run("should not mention the daemon when nothing was installed", func(t *testing.T) {
		t.Parallel()
		// given
		var errOut bytes.Buffer
		updates := &doubles.StubSelfUpdateRepository{Installed: false}
		path := filepath.Join(t.TempDir(), "store.json")
		recordRunningDaemon(t, path)

		// when
		err := executeCLIWith(path, updates, io.Discard, &errOut, "self-update", "--force")

		// then
		require.NoError(t, err)
		assert.Empty(t, errOut.String())
	})
}
