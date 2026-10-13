//go:build windows

package repositories

import (
	"os"
	"syscall"
	"unsafe"
)

// ToolhelpSessionsRepository detects the running sessions of one CLI by walking
// the Windows process list. Windows has no /proc, so the ToolHelp32 snapshot API
// takes its place; it is available on every supported Windows version and needs
// no elevated rights to enumerate process names.
//
// Only a CLI running as its own executable (claude.exe, codex.exe) is recognized.
// An npm installation of Claude Code runs the CLI inside node.exe, and the
// snapshot carries executable names but no command lines, so such a session is
// invisible here — matching node.exe instead would suppress rotation for every
// Node process on the machine. Codex's npm package starts its native codex.exe,
// which is seen.
type ToolhelpSessionsRepository struct {
	command string
	selfPID int
}

// NewToolhelpSessionsRepository creates a repository that enumerates processes
// through a ToolHelp32 snapshot, looking for the CLI that runs as command, such
// as "claude" or "codex".
func NewToolhelpSessionsRepository(command string) *ToolhelpSessionsRepository {
	return &ToolhelpSessionsRepository{command: command, selfPID: os.Getpid()}
}

// Running reports whether any process other than this one is running the
// executable of the CLI.
func (r *ToolhelpSessionsRepository) Running() bool {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer func() { _ = syscall.CloseHandle(snapshot) }()

	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	for err = syscall.Process32First(snapshot, &entry); err == nil; err = syscall.Process32Next(snapshot, &entry) {
		if int(entry.ProcessID) == r.selfPID {
			continue
		}
		if matchesProcess(syscall.UTF16ToString(entry.ExeFile[:]), r.command) {
			return true
		}
	}
	return false
}
