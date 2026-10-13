//go:build darwin

package repositories_test

import (
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

func psLister(table string) func() ([]byte, error) {
	return func() ([]byte, error) { return []byte(table), nil }
}

func TestPSSessionsRepositoryRunning(t *testing.T) {
	t.Parallel()

	t.Run("should detect a running claude process", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewPSSessionsRepositoryWithLister("claude", psLister(
			"  501 /bin/zsh\n 4242 claude\n",
		))

		// when
		running := repo.Running()

		// then
		assert.True(t, running)
	})

	t.Run("should detect a claude process launched by absolute path", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewPSSessionsRepositoryWithLister("claude", psLister(
			" 4242 /Users/someone/.local/bin/claude\n",
		))

		// when
		running := repo.Running()

		// then
		assert.True(t, running)
	})

	t.Run("should ignore the Claude desktop app bundle", func(t *testing.T) {
		t.Parallel()
		// given the desktop app's executable base name matches "claude"
		// case-insensitively; counting it would block every rotation forever
		repo := repositories.NewPSSessionsRepositoryWithLister("claude", psLister(
			" 4242 /Applications/Claude.app/Contents/MacOS/Claude\n"+
				" 4243 /Applications/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper\n",
		))

		// when
		running := repo.Running()

		// then
		assert.False(t, running)
	})

	t.Run("should ignore this very process", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewPSSessionsRepositoryWithLister("claude", psLister(
			" "+strconv.Itoa(os.Getpid())+" claude\n",
		))

		// when
		running := repo.Running()

		// then
		assert.False(t, running)
	})

	t.Run("should report not running for a process merely ending in claude", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewPSSessionsRepositoryWithLister("claude", psLister(" 4242 notclaude\n"))

		// when
		running := repo.Running()

		// then
		assert.False(t, running)
	})

	t.Run("should tolerate executables containing spaces", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewPSSessionsRepositoryWithLister("claude", psLister(
			" 4242 /Library/Some Vendor/Helper Daemon\n 4243 claude\n",
		))

		// when
		running := repo.Running()

		// then
		assert.True(t, running)
	})

	t.Run("should detect a running codex process when looking for Codex", func(t *testing.T) {
		t.Parallel()
		// given: the npm package starts its native binary, which is what ps lists
		repo := repositories.NewPSSessionsRepositoryWithLister("codex", psLister(
			" 4242 /usr/local/lib/node_modules/@openai/codex/vendor/aarch64-apple-darwin/bin/codex\n",
		))

		// when
		running := repo.Running()

		// then
		assert.True(t, running)
	})

	t.Run("should not count a claude process as a Codex session", func(t *testing.T) {
		t.Parallel()
		// given: each provider's credentials only have to wait for its own CLI
		repo := repositories.NewPSSessionsRepositoryWithLister("codex", psLister(" 4242 claude\n"))

		// when
		running := repo.Running()

		// then
		assert.False(t, running)
	})

	t.Run("should report not running when the process table cannot be read", func(t *testing.T) {
		t.Parallel()
		// given
		repo := repositories.NewPSSessionsRepositoryWithLister(
			"claude", func() ([]byte, error) { return nil, errors.New("ps failed") },
		)

		// when
		running := repo.Running()

		// then
		assert.False(t, running)
	})
}
