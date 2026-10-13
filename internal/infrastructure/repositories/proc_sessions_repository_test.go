package repositories_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

func writeProcEntry(t *testing.T, procRoot, pid, comm string) {
	t.Helper()
	pidDir := filepath.Join(procRoot, pid)
	require.NoError(t, os.MkdirAll(pidDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(pidDir, "comm"), []byte(comm+"\n"), 0o600))
}

func TestProcSessionsRepositoryRunning(t *testing.T) {
	t.Parallel()

	t.Run("should detect a running claude process", func(t *testing.T) {
		t.Parallel()
		// given
		procRoot := t.TempDir()
		writeProcEntry(t, procRoot, "4242", "claude")
		repo := repositories.NewProcSessionsRepository(procRoot, "claude")

		// when
		running := repo.Running()

		// then
		assert.True(t, running)
	})

	t.Run("should detect a claude process spelled as a Windows executable", func(t *testing.T) {
		t.Parallel()
		// given
		procRoot := t.TempDir()
		writeProcEntry(t, procRoot, "4243", "Claude.exe")
		repo := repositories.NewProcSessionsRepository(procRoot, "claude")

		// when
		running := repo.Running()

		// then
		assert.True(t, running)
	})

	t.Run("should report not running for a process merely ending in claude", func(t *testing.T) {
		t.Parallel()
		// given
		procRoot := t.TempDir()
		writeProcEntry(t, procRoot, "4244", "notclaude")
		repo := repositories.NewProcSessionsRepository(procRoot, "claude")

		// when
		running := repo.Running()

		// then
		assert.False(t, running)
	})

	t.Run("should report not running when no process is named claude", func(t *testing.T) {
		t.Parallel()
		// given
		procRoot := t.TempDir()
		writeProcEntry(t, procRoot, "4245", "node")
		repo := repositories.NewProcSessionsRepository(procRoot, "claude")

		// when
		running := repo.Running()

		// then
		assert.False(t, running)
	})

	t.Run("should detect a running codex process when looking for Codex", func(t *testing.T) {
		t.Parallel()
		// given
		procRoot := t.TempDir()
		writeProcEntry(t, procRoot, "4246", "codex")
		repo := repositories.NewProcSessionsRepository(procRoot, "codex")

		// when
		running := repo.Running()

		// then
		assert.True(t, running)
	})

	t.Run("should not count a Codex helper process as a Codex session", func(t *testing.T) {
		t.Parallel()
		// given: Codex starts helpers of its own, which do not hold a sign-in
		procRoot := t.TempDir()
		writeProcEntry(t, procRoot, "4247", "codex-code-mode")
		repo := repositories.NewProcSessionsRepository(procRoot, "codex")

		// when
		running := repo.Running()

		// then
		assert.False(t, running)
	})

	t.Run("should not count a claude process as a Codex session", func(t *testing.T) {
		t.Parallel()
		// given: each provider's credentials only have to wait for its own CLI
		procRoot := t.TempDir()
		writeProcEntry(t, procRoot, "4248", "claude")
		repo := repositories.NewProcSessionsRepository(procRoot, "codex")

		// when
		running := repo.Running()

		// then
		assert.False(t, running)
	})

	t.Run("should report not running when the proc root is absent", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewProcSessionsRepository(filepath.Join(t.TempDir(), "absent"), "claude")

		// when
		running := repo.Running()

		// then
		assert.False(t, running)
	})
}
