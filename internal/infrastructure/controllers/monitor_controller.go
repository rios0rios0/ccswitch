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
		Use:   monitorCommandName,
		Short: "Run the usage-monitoring daemon that rotates each provider's accounts on exhaustion",
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

	return newMonitor(deps).Run(ctx)
}

// newMonitor builds the monitor over every provider's accounts, each rotated on its
// own. Claude's pool comes first, since its store holds the settings every provider
// shares.
func newMonitor(deps *deps) *commands.MonitorCommand {
	pools := deps.orderedPools(entities.Providers())
	monitor := commands.NewMonitorCommand(
		deps.config, pools[0].Accounts, pools[0].Credentials, pools[0].Usage, pools[0].Tokens, pools[0].Sessions,
	)
	for _, pool := range pools[1:] {
		monitor.WithPool(pool)
	}
	return monitor
}

// daemonArgs reconstructs the flags the detached daemon must run with so it uses
// the same configuration as the foreground invocation.
//
// --threshold is passed on only when the caller named it. A daemon started with
// the flag baked in would treat it as an explicit override and ignore the value
// `ccswitch threshold` persists, which is precisely the in-flight retuning that
// command exists to provide.
func daemonArgs(cfg *entities.Config) []string {
	args := []string{
		monitorCommandName,
		"--interval", cfg.Interval.String(),
		"--store", cfg.StorePath,
		"--credentials", cfg.CredentialsPath,
		"--claude-json", cfg.ClaudeJSONPath,
		"--" + codexHomeFlag, cfg.CodexHome,
		fmt.Sprintf("--prefer-primary=%t", cfg.PreferPrimary),
	}
	if cfg.ThresholdExplicit {
		args = append(args,
			"--threshold", strconv.FormatFloat(cfg.Threshold, 'f', floatPrecision, float64BitSize))
	}
	if cfg.Verbose {
		args = append(args, "--"+verboseFlag)
	}
	return args
}
