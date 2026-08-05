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

func TestProcSessionsRepositoryClaudeRunning(t *testing.T) {
	t.Parallel()

	t.Run("should detect a running claude process", func(t *testing.T) {
		t.Parallel()
		// given
		procRoot := t.TempDir()
		writeProcEntry(t, procRoot, "4242", "claude")
		repo := repositories.NewProcSessionsRepository(procRoot)

		// when
		running := repo.ClaudeRunning()

		// then
		assert.True(t, running)
	})

	t.Run("should detect a claude process spelled as a Windows executable", func(t *testing.T) {
		t.Parallel()
		// given
		procRoot := t.TempDir()
		writeProcEntry(t, procRoot, "4243", "Claude.exe")
		repo := repositories.NewProcSessionsRepository(procRoot)

		// when
		running := repo.ClaudeRunning()

		// then
		assert.True(t, running)
	})

	t.Run("should report not running for a process merely ending in claude", func(t *testing.T) {
		t.Parallel()
		// given
		procRoot := t.TempDir()
		writeProcEntry(t, procRoot, "4244", "notclaude")
		repo := repositories.NewProcSessionsRepository(procRoot)

		// when
		running := repo.ClaudeRunning()

		// then
		assert.False(t, running)
	})

	t.Run("should report not running when no process is named claude", func(t *testing.T) {
		t.Parallel()
		// given
		procRoot := t.TempDir()
		writeProcEntry(t, procRoot, "4245", "node")
		repo := repositories.NewProcSessionsRepository(procRoot)

		// when
		running := repo.ClaudeRunning()

		// then
		assert.False(t, running)
	})

	t.Run("should report not running when the proc root is absent", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewProcSessionsRepository(filepath.Join(t.TempDir(), "absent"))

		// when
		running := repo.ClaudeRunning()

		// then
		assert.False(t, running)
	})
}
