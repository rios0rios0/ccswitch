//go:build !windows

package services

import (
	"os"
	"syscall"
)

// aliveSignal is the no-op signal used to probe whether a process exists.
const aliveSignal = syscall.Signal(0)

// detachAttrs returns the attributes that detach the monitor from the launching
// terminal. A new session (setsid) leaves the daemon without a controlling
// terminal, so closing the shell does not send it SIGHUP.
func detachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// processAlive reports whether a process with the given pid currently exists.
// Signal 0 performs the kernel's existence and permission checks without
// delivering anything.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(aliveSignal) == nil
}
