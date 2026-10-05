package repositories

// SelfUpdateRepository is the channel ccswitch's own releases are published on:
// it tells when a newer release is out, and installs one in place of the running
// binary.
type SelfUpdateRepository interface {
	// Update installs the latest release in place of the running binary when it is
	// newer, and reports whether it did. dryRun only reports what would be
	// installed, and force skips the confirmation prompt.
	Update(dryRun, force bool) (bool, error)
	// CheckForUpdates warns when a newer release is out. It looks at most once a
	// day, never holds up the command it runs alongside, and never fails one: a
	// lookup that goes wrong is only logged at debug level.
	CheckForUpdates()
}
