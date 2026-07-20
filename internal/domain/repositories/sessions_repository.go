package repositories

// SessionsRepository reports on running Claude Code sessions so that ccswitch can
// avoid swapping credentials underneath a live process.
type SessionsRepository interface {
	// ClaudeRunning reports whether a `claude` process is currently running.
	ClaudeRunning() bool
}
