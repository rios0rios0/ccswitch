//go:build windows

package repositories_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

// helperEnvVar marks the child process spawned by these tests. The child is this
// same test binary, so without the marker it would spawn children of its own.
const helperEnvVar = "CCSWITCH_TEST_SLEEP_HELPER"

// TestSleepHelper is not a test: it is the body of the child process the
// detection test spawns, and it does nothing unless it was started as one.
func TestSleepHelper(t *testing.T) {
	if os.Getenv(helperEnvVar) == "" {
		t.Skip("not running as the spawned helper process")
	}
	time.Sleep(30 * time.Second)
}

// startProcessNamed copies the test binary under the given executable name and
// runs it as an idle child, so the process list holds a process with a known
// name. It returns once the child has been started.
func startProcessNamed(t *testing.T, name string) {
	t.Helper()

	self, err := os.Executable()
	require.NoError(t, err)
	source, err := os.ReadFile(self)
	require.NoError(t, err)

	binary := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(binary, source, 0o700))

	child := exec.Command(binary, "-test.run=^TestSleepHelper$", "-test.timeout=60s")
	child.Env = append(os.Environ(), helperEnvVar+"=1")
	require.NoError(t, child.Start())

	t.Cleanup(func() {
		_ = child.Process.Kill()
		_, _ = child.Process.Wait()
	})
}

// eventuallyRunning polls the detector, since a spawned process does not appear
// in the snapshot instantly.
func eventuallyRunning(repo *repositories.ToolhelpSessionsRepository) bool {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if repo.ClaudeRunning() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func TestToolhelpSessionsRepositoryClaudeRunning(t *testing.T) {
	if os.Getenv(helperEnvVar) != "" {
		t.Skip("running as the spawned helper process")
	}

	t.Run("should detect a running claude process", func(t *testing.T) {
		// given
		startProcessNamed(t, "claude.exe")
		repo := repositories.NewToolhelpSessionsRepository()

		// when
		running := eventuallyRunning(repo)

		// then
		assert.True(t, running)
	})
}
