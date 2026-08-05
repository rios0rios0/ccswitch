package services_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/infrastructure/services"
)

// newDaemonService builds a service over a fresh state directory, returning it
// alongside its pidfile path.
func newDaemonService(t *testing.T) (*services.DaemonService, string) {
	t.Helper()
	stateDir := t.TempDir()
	pidPath := filepath.Join(stateDir, "monitor.pid")
	return services.NewDaemonService(pidPath, filepath.Join(stateDir, "monitor.log")), pidPath
}

// writePidFile records an arbitrary pid, standing in for a daemon that wrote the
// file and has since gone away.
func writePidFile(t *testing.T, pidPath, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(pidPath, []byte(contents), 0o600))
}

func TestDaemonServiceRunning(t *testing.T) {
	t.Parallel()

	t.Run("should report running when the pidfile holds a live process", func(t *testing.T) {
		t.Parallel()
		// given
		daemon, _ := newDaemonService(t)
		require.NoError(t, daemon.WriteSelf())

		// when
		running := daemon.Running()

		// then
		assert.True(t, running)
	})

	t.Run("should report not running when the pidfile is absent", func(t *testing.T) {
		t.Parallel()
		// given
		daemon, _ := newDaemonService(t)

		// when
		running := daemon.Running()

		// then
		assert.False(t, running)
	})

	t.Run("should report not running when the pidfile holds an invalid pid", func(t *testing.T) {
		t.Parallel()
		// given
		daemon, pidPath := newDaemonService(t)
		writePidFile(t, pidPath, "0")

		// when
		running := daemon.Running()

		// then
		assert.False(t, running)
	})

	t.Run("should report not running when the pidfile is not a number", func(t *testing.T) {
		t.Parallel()
		// given
		daemon, pidPath := newDaemonService(t)
		writePidFile(t, pidPath, "not-a-pid")

		// when
		running := daemon.Running()

		// then
		assert.False(t, running)
	})
}

func TestDaemonServiceEnsure(t *testing.T) {
	t.Parallel()

	t.Run("should not start a second process when one is already running", func(t *testing.T) {
		t.Parallel()
		// given
		daemon, _ := newDaemonService(t)
		require.NoError(t, daemon.WriteSelf())

		// when
		started, err := daemon.Ensure("/nonexistent/binary", nil)

		// then
		require.NoError(t, err)
		assert.False(t, started)
	})

	t.Run("should fail when the binary cannot be started", func(t *testing.T) {
		t.Parallel()
		// given
		daemon, _ := newDaemonService(t)

		// when
		started, err := daemon.Ensure(filepath.Join(t.TempDir(), "absent-binary"), nil)

		// then
		require.Error(t, err)
		assert.False(t, started)
	})
}

func TestDaemonServiceWriteSelf(t *testing.T) {
	t.Parallel()

	t.Run("should record this process id", func(t *testing.T) {
		t.Parallel()
		// given
		daemon, pidPath := newDaemonService(t)

		// when
		err := daemon.WriteSelf()

		// then
		require.NoError(t, err)
		data, readErr := os.ReadFile(pidPath)
		require.NoError(t, readErr)
		assert.Equal(t, strconv.Itoa(os.Getpid()), string(data))
	})
}

func TestDaemonServiceRemove(t *testing.T) {
	t.Parallel()

	t.Run("should delete the pidfile", func(t *testing.T) {
		t.Parallel()
		// given
		daemon, pidPath := newDaemonService(t)
		require.NoError(t, daemon.WriteSelf())

		// when
		daemon.Remove()

		// then
		assert.NoFileExists(t, pidPath)
		assert.False(t, daemon.Running())
	})

	t.Run("should ignore a missing pidfile", func(t *testing.T) {
		t.Parallel()
		// given
		daemon, pidPath := newDaemonService(t)

		// when
		daemon.Remove()

		// then
		assert.NoFileExists(t, pidPath)
	})
}

func TestDaemonServiceLogPath(t *testing.T) {
	t.Parallel()

	t.Run("should return the configured log path", func(t *testing.T) {
		t.Parallel()
		// given
		stateDir := t.TempDir()
		logPath := filepath.Join(stateDir, "monitor.log")
		daemon := services.NewDaemonService(filepath.Join(stateDir, "monitor.pid"), logPath)

		// when
		actual := daemon.LogPath()

		// then
		assert.Equal(t, logPath, actual)
	})
}
