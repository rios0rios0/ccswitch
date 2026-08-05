//go:build windows

package repositories

import (
	"os"
	"syscall"
	"unsafe"
)

// ToolhelpSessionsRepository detects running Claude Code sessions by walking the
// Windows process list. Windows has no /proc, so the ToolHelp32 snapshot API
// takes its place; it is available on every supported Windows version and needs
// no elevated rights to enumerate process names.
//
// Only a natively installed Claude Code (claude.exe) is recognized. An npm
// installation runs the CLI inside node.exe, and the snapshot carries executable
// names but no command lines, so such a session is invisible here — matching
// node.exe instead would suppress rotation for every Node process on the machine.
type ToolhelpSessionsRepository struct {
	selfPID int
}

// NewToolhelpSessionsRepository creates a repository that enumerates processes
// through a ToolHelp32 snapshot.
func NewToolhelpSessionsRepository() *ToolhelpSessionsRepository {
	return &ToolhelpSessionsRepository{selfPID: os.Getpid()}
}

// ClaudeRunning reports whether any process other than this one is running the
// Claude Code executable.
func (r *ToolhelpSessionsRepository) ClaudeRunning() bool {
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
		if matchesClaudeProcess(syscall.UTF16ToString(entry.ExeFile[:])) {
			return true
		}
	}
	return false
}
