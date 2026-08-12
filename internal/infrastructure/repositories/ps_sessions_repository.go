//go:build darwin

package repositories

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// appBundleMarker identifies an executable living inside a macOS application
// bundle. The Claude desktop app runs as ".../Claude.app/Contents/MacOS/Claude",
// whose base name matches Claude Code's case-insensitively. Counting it as a
// session would keep the desktop app pinned as "Claude is running" and block
// every rotation forever, so bundled executables are never treated as sessions.
const appBundleMarker = ".app/"

const psTimeout = 5 * time.Second

// PSSessionsRepository detects running Claude Code sessions from the process
// table. macOS provides no /proc, so the process list is obtained from `ps`.
type PSSessionsRepository struct {
	lister  func() ([]byte, error)
	selfPID int
}

// NewPSSessionsRepository creates a repository that reads the process table with
// `ps`.
func NewPSSessionsRepository() *PSSessionsRepository {
	return NewPSSessionsRepositoryWithLister(listProcesses)
}

// NewPSSessionsRepositoryWithLister creates a repository over an explicit process
// lister, which must emit one "<pid> <executable>" row per process. It exists so
// tests can drive the matching rules against a fixed process table.
func NewPSSessionsRepositoryWithLister(lister func() ([]byte, error)) *PSSessionsRepository {
	return &PSSessionsRepository{lister: lister, selfPID: os.Getpid()}
}

// listProcesses returns "<pid> <executable>" for every process on the system.
func listProcesses() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), psTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "ps", "-Ao", "pid=,comm=").Output()
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller only distinguishes success from failure
	}
	return out, nil
}

// ClaudeRunning reports whether any process other than this one is a Claude Code
// CLI process.
func (r *PSSessionsRepository) ClaudeRunning() bool {
	out, err := r.lister()
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(bytes.TrimSpace(out)), "\n") {
		pid, executable, ok := splitProcessLine(line)
		if !ok || pid == r.selfPID {
			continue
		}
		if strings.Contains(executable, appBundleMarker) {
			continue
		}
		if matchesClaudeProcess(filepath.Base(executable)) {
			return true
		}
	}
	return false
}

// splitProcessLine parses one `ps -Ao pid=,comm=` row into its pid and executable
// path. The executable is everything after the pid because it may contain spaces.
func splitProcessLine(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(line)
	separator := strings.IndexAny(trimmed, " \t")
	if separator < 0 {
		return 0, "", false
	}

	pid, err := strconv.Atoi(trimmed[:separator])
	if err != nil {
		return 0, "", false
	}

	executable := strings.TrimSpace(trimmed[separator+1:])
	if executable == "" {
		return 0, "", false
	}
	return pid, executable, true
}
