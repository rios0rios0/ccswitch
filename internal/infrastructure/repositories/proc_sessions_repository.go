package repositories

import (
	"os"
	"path/filepath"
	"strconv"
)

// ProcSessionsRepository detects the running sessions of one CLI by scanning a
// /proc filesystem. It reports false on systems without /proc.
type ProcSessionsRepository struct {
	procRoot string
	command  string
	selfPID  int
}

// NewProcSessionsRepository creates a repository that scans the given /proc root
// for processes of the CLI that runs as command, such as "claude" or "codex".
func NewProcSessionsRepository(procRoot, command string) *ProcSessionsRepository {
	return &ProcSessionsRepository{procRoot: procRoot, command: command, selfPID: os.Getpid()}
}

// Running reports whether any process other than this one has the comm name of
// the CLI.
func (r *ProcSessionsRepository) Running() bool {
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

// commMatches reports whether the process comm name identifies the CLI.
func (r *ProcSessionsRepository) commMatches(pid int) bool {
	data, err := os.ReadFile(filepath.Join(r.procRoot, strconv.Itoa(pid), "comm"))
	if err != nil {
		return false
	}
	return matchesProcess(string(data), r.command)
}
