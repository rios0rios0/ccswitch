package controllers

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/services"
)

// newSelfUpdateCommand builds the `self-update` subcommand, which replaces the
// running binary with the latest release.
func newSelfUpdateCommand(cfg *entities.Config, updates domain.SelfUpdateRepository) *cobra.Command {
	var dryRun, force bool
	cmd := &cobra.Command{
		Use:   selfUpdateCommandName,
		Short: "Update ccswitch to the latest release",
		Long: "Download the latest ccswitch release from GitHub and install it in place of the " +
			"running binary, after asking for confirmation.\n\n" +
			"A monitor daemon that is already running goes on running the binary it was started " +
			"from, so it has to be restarted to pick the update up; ccswitch says so when it finds one.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			installed, err := commands.NewSelfUpdateCommand(updates).Execute(dryRun, force)
			if err != nil {
				return err
			}
			if installed {
				warnStaleDaemon(cmd.ErrOrStderr(), newDaemonService(cfg))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be installed without installing it")
	cmd.Flags().BoolVar(&force, "force", false, "install without asking for confirmation")
	return cmd
}

// warnStaleDaemon tells the user to restart a monitor daemon that is still running
// after an update replaced its binary. Left alone it keeps running the old code
// indefinitely: the process holds on to the file it was started from, and
// `monitor --ensure-daemon` finds it alive and leaves it be.
func warnStaleDaemon(out io.Writer, daemon *services.DaemonService) {
	pid, running := daemon.RunningPid()
	if !running {
		return
	}
	fmt.Fprintf(out, "[ccswitch] the monitor daemon (pid %d) is still running the previous version; "+
		"stop it and run `ccswitch monitor --ensure-daemon` to start the updated one\n", pid)
}
