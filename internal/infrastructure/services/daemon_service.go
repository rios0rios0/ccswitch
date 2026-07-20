// Package services contains infrastructure services that support the CLI, such as
// supervision of the background monitor daemon.
package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	statePerm    = 0o600
	stateDirPerm = 0o700
)

// aliveSignal is the no-op signal used to probe whether a process exists.
const aliveSignal = syscall.Signal(0)

// DaemonService supervises the background monitor process using a pidfile.
type DaemonService struct {
	pidPath string
	logPath string
}

// NewDaemonService creates a DaemonService using the given pidfile and log paths.
func NewDaemonService(pidPath, logPath string) *DaemonService {
	return &DaemonService{pidPath: pidPath, logPath: logPath}
}

// Running reports whether a live monitor process is recorded in the pidfile.
func (d *DaemonService) Running() bool {
	pid, err := d.readPid()
	if err != nil {
		return false
	}
	return processAlive(pid)
}

// Ensure starts a detached monitor process when one is not already running and
// reports whether it started a new process.
func (d *DaemonService) Ensure(binary string, args []string) (bool, error) {
	if d.Running() {
		return false, nil
	}
	if err := d.spawn(binary, args); err != nil {
		return false, err
	}
	return true, nil
}

// WriteSelf records the current process id as the running daemon.
func (d *DaemonService) WriteSelf() error {
	return d.writePid(os.Getpid())
}

// Remove deletes the pidfile, ignoring a missing file.
func (d *DaemonService) Remove() {
	_ = os.Remove(d.pidPath)
}

// LogPath returns the daemon log path.
func (d *DaemonService) LogPath() string {
	return d.logPath
}

// spawn launches the binary detached from the controlling terminal, with output
// redirected to the log file, and records the child's pid.
func (d *DaemonService) spawn(binary string, args []string) error {
	logFile, err := d.openLog()
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()

	argv := append([]string{binary}, args...)
	attr := &os.ProcAttr{
		Files: []*os.File{nil, logFile, logFile},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	}
	proc, err := os.StartProcess(binary, argv, attr)
	if err != nil {
		return fmt.Errorf("failed to start monitor daemon: %w", err)
	}
	if err = d.writePid(proc.Pid); err != nil {
		return err
	}
	return proc.Release()
}

// openLog opens the daemon log file for appending, creating it and its directory.
func (d *DaemonService) openLog() (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(d.logPath), stateDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}
	file, err := os.OpenFile(d.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, statePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to open daemon log: %w", err)
	}
	return file, nil
}

// writePid records the given pid in the pidfile.
func (d *DaemonService) writePid(pid int) error {
	if err := os.MkdirAll(filepath.Dir(d.pidPath), stateDirPerm); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}
	if err := os.WriteFile(d.pidPath, []byte(strconv.Itoa(pid)), statePerm); err != nil {
		return fmt.Errorf("failed to write pidfile: %w", err)
	}
	return nil
}

// readPid reads the pid recorded in the pidfile.
func (d *DaemonService) readPid() (int, error) {
	data, err := os.ReadFile(d.pidPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read pidfile: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid pidfile contents: %w", err)
	}
	return pid, nil
}

// processAlive reports whether a process with the given pid currently exists.
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
