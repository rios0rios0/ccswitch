package controllers

import (
	"github.com/spf13/cobra"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// NewRootCommandWithUpdates builds the root command over the given release
// channel, so that no test invocation reaches GitHub.
func NewRootCommandWithUpdates(version string, updates domain.SelfUpdateRepository) *cobra.Command {
	return newRootCommand(version, updates)
}

// DaemonArgs returns the flags a detached monitor daemon is started with.
func DaemonArgs(cfg *entities.Config) []string {
	return daemonArgs(cfg)
}

// ResolveOutputStyle decides how richly to dress up output for the given --color
// mode, environment, and whether the output is a terminal.
func ResolveOutputStyle(mode string, env map[string]string, terminal bool) entities.OutputStyle {
	return resolveOutputStyle(colorMode(mode), func(name string) string { return env[name] }, terminal)
}
