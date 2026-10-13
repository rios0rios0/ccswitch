package repositories

import "strings"

// windowsExecutableSuffix is the extension Windows reports as part of the
// executable name ("claude.exe", "codex.exe").
const windowsExecutableSuffix = ".exe"

// matchesProcess reports whether an executable name identifies a process of the
// CLI that runs as command, such as "claude" or "codex". The comparison is shared
// by every platform's session detector so they all agree on what counts as a
// session: /proc reports "claude", while the Windows process list reports
// "claude.exe" and treats file names case-insensitively.
func matchesProcess(name, command string) bool {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) > len(windowsExecutableSuffix) &&
		strings.EqualFold(trimmed[len(trimmed)-len(windowsExecutableSuffix):], windowsExecutableSuffix) {
		trimmed = trimmed[:len(trimmed)-len(windowsExecutableSuffix)]
	}
	return strings.EqualFold(trimmed, command)
}
