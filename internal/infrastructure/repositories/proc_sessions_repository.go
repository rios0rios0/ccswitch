package repositories

import (
	"os"
	"path/filepath"
	"strconv"
)

// ProcSessionsRepository detects running Claude Code sessions by scanning a /proc
// filesystem. It reports false on systems without /proc.
type ProcSessionsRepository struct {
	procRoot string
	selfPID  int
}

// NewProcSessionsRepository creates a repository that scans the given /proc root.
func NewProcSessionsRepository(procRoot string) *ProcSessionsRepository {
	return &ProcSessionsRepository{procRoot: procRoot, selfPID: os.Getpid()}
}

// ClaudeRunning reports whether any process other than this one has the comm name
// "claude".
func (r *ProcSessionsRepository) ClaudeRunning() bool {
	entries, err := os.ReadDir(r.procRoot)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, convErr := strconv.Atoi(entry.Name())
		if convErr != nil || pid == r.selfPID {
			continue
		}
		if r.commMatches(pid) {
			return true
		}
	}
	return false
}

// commMatches reports whether the process comm name identifies Claude Code.
func (r *ProcSessionsRepository) commMatches(pid int) bool {
	data, err := os.ReadFile(filepath.Join(r.procRoot, strconv.Itoa(pid), "comm"))
	if err != nil {
		return false
	}
	return matchesClaudeProcess(string(data))
}
