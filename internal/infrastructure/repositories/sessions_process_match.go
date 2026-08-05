package repositories

import "strings"

// claudeProcessName is the executable name Claude Code runs under.
const claudeProcessName = "claude"

// windowsExecutableSuffix is the extension Windows reports as part of the
// executable name ("claude.exe").
const windowsExecutableSuffix = ".exe"

// matchesClaudeProcess reports whether an executable name identifies a Claude
// Code process. The comparison is shared by every platform's session detector so
// both agree on what counts as a session: /proc reports "claude", while the
// Windows process list reports "claude.exe" and treats file names
// case-insensitively.
func matchesClaudeProcess(name string) bool {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) > len(windowsExecutableSuffix) &&
		strings.EqualFold(trimmed[len(trimmed)-len(windowsExecutableSuffix):], windowsExecutableSuffix) {
		trimmed = trimmed[:len(trimmed)-len(windowsExecutableSuffix)]
	}
	return strings.EqualFold(trimmed, claudeProcessName)
}
