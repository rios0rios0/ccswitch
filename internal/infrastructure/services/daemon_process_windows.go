//go:build windows

package services

import (
	"math"
	"syscall"
)

// detachedProcess is Win32's DETACHED_PROCESS creation flag. The standard library
// does not export it, unlike CREATE_NEW_PROCESS_GROUP.
const detachedProcess = 0x00000008

// detachAttrs returns the attributes that detach the monitor from the launching
// console. DETACHED_PROCESS withholds the parent's console (so no window is
// attached and the daemon survives its closing), and CREATE_NEW_PROCESS_GROUP
// keeps Ctrl+C and Ctrl+Break in that console from reaching the daemon. Together
// they are the Windows counterpart of setsid.
func detachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess}
}

// processAlive reports whether a process with the given pid currently exists.
//
// Windows has no signals, so liveness is read from the process object itself:
// waiting on its handle with a zero timeout returns WAIT_TIMEOUT while the
// process runs and WAIT_OBJECT_0 once it has exited. GetExitCodeProcess would be
// the obvious alternative, but it reports STILL_ACTIVE (259) for a running
// process and cannot distinguish that from a process that genuinely exited with
// code 259 — the wait has no such ambiguity.
//
// The handle is opened with SYNCHRONIZE, the one right a wait requires — asking
// for PROCESS_QUERY_INFORMATION instead makes WaitForSingleObject fail with
// ERROR_ACCESS_DENIED, which would report a live daemon as dead and start a
// second one on every launch. A failure to open is treated as "not running":
// the pidfile's owner always holds this right over its own daemon, so the
// realistic failure is the pid being gone.
func processAlive(pid int) bool {
	if pid <= 0 || pid > math.MaxUint32 {
		return false
	}
	handle, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = syscall.CloseHandle(handle) }()

	event, err := syscall.WaitForSingleObject(handle, 0)
	if err != nil {
		return false
	}
	return event == syscall.WAIT_TIMEOUT
}
