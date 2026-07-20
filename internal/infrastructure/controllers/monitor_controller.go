package controllers

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

const (
	floatPrecision = -1
	float64BitSize = 64
)

// newMonitorCommand builds the `monitor` subcommand, which either runs the daemon
// loop in the foreground or, with --ensure-daemon, starts it detached.
func newMonitorCommand(cfg *entities.Config) *cobra.Command {
	var ensureDaemon bool
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Run the usage-monitoring daemon that rotates accounts on exhaustion",
		RunE: func(_ *cobra.Command, _ []string) error {
			deps := newDeps(cfg)
			if ensureDaemon {
				return runEnsureDaemon(deps)
			}
			return runMonitorForeground(deps)
		},
	}
	cmd.Flags().BoolVar(&ensureDaemon, "ensure-daemon", false,
		"start the daemon in the background only if not already running, then exit")
	return cmd
}

// runEnsureDaemon starts a detached monitor process when one is not running.
func runEnsureDaemon(deps *deps) error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve own executable path: %w", err)
	}
	started, err := deps.daemon.Ensure(binary, daemonArgs(deps.config))
	if err != nil {
		return err
	}
	if started {
		logger.Infof("[ccswitch] monitor daemon started (logs: %s)", deps.daemon.LogPath())
	}
	return nil
}

// runMonitorForeground runs the monitor loop until the process is interrupted.
func runMonitorForeground(deps *deps) error {
	if err := deps.daemon.WriteSelf(); err != nil {
		logger.Warnf("[ccswitch] could not write pidfile: %v", err)
	}
	defer deps.daemon.Remove()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	monitor := commands.NewMonitorCommand(
		deps.config, deps.accounts, deps.credentials, deps.usage, deps.tokens, deps.sessions,
	)
	return monitor.Run(ctx)
}

// daemonArgs reconstructs the flags the detached daemon must run with so it uses
// the same configuration as the foreground invocation.
func daemonArgs(cfg *entities.Config) []string {
	return []string{
		"monitor",
		"--interval", cfg.Interval.String(),
		"--threshold", strconv.FormatFloat(cfg.Threshold, 'f', floatPrecision, float64BitSize),
		"--store", cfg.StorePath,
		"--credentials", cfg.CredentialsPath,
		"--claude-json", cfg.ClaudeJSONPath,
	}
}
