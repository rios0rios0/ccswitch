package repositories

// SessionsRepository reports on the running sessions of one provider's CLI -- `claude`
// or `codex` -- so that ccswitch can avoid swapping credentials underneath a live
// process.
type SessionsRepository interface {
	// Running reports whether a session of the provider's CLI is currently running.
	Running() bool
}
