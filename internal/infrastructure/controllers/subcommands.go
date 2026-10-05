package controllers

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

const (
	// priorityFlag is shared by `enroll` and `update`, which both place an account
	// in the rotation order.
	priorityFlag  = "priority"
	priorityUsage = "a position counted from 1 for the primary, or one of top, bottom, up, down"
)

// newThresholdCommand builds the `threshold` subcommand, which reads or sets the
// rotation threshold that the store carries and the daemon rereads every tick.
func newThresholdCommand(cfg *entities.Config) *cobra.Command {
	var reset bool
	cmd := &cobra.Command{
		Use:   "threshold [percent]",
		Short: "Show or set the utilization percent that triggers rotation",
		Long: "Show the rotation threshold, or set it to the given percent.\n\n" +
			"A threshold set here is persisted in the store, so a running monitor daemon " +
			"picks it up on its next tick without being restarted, and it is applied " +
			"immediately: every account is repolled and the highest-priority one whose " +
			"utilization is below the new threshold becomes active.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			deps := newDeps(cfg)
			command := commands.NewSetThresholdCommand(
				deps.config, deps.accounts, deps.credentials,
				deps.usage, deps.tokens, deps.sessions,
			)
			switch {
			case reset:
				return command.Reset()
			case len(args) == 0:
				return command.Show()
			}
			threshold, err := strconv.ParseFloat(args[0], float64BitSize)
			if err != nil {
				return fmt.Errorf("invalid threshold %q: %w", args[0], err)
			}
			return command.Execute(threshold)
		},
	}
	cmd.Flags().BoolVar(&reset, "reset", false,
		"drop the stored threshold and go back to the built-in default")
	return cmd
}

// newEnrollCommand builds the `enroll` subcommand.
func newEnrollCommand(cfg *entities.Config) *cobra.Command {
	var token, email, priority string
	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Capture the currently logged-in Claude account into the store",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var placement *entities.Priority
			if cmd.Flags().Changed(priorityFlag) {
				parsed, err := entities.ParsePriority(priority)
				if err != nil {
					return err
				}
				placement = &parsed
			}
			deps := newDeps(cfg)
			return commands.NewEnrollAccountCommand(deps.accounts, deps.credentials).
				Execute(token, email, placement)
		},
	}
	cmd.Flags().StringVar(&token, "token", "",
		"enroll a long-lived OAuth token directly (e.g. from claude setup-token) instead of "+
			"reading the Claude Code credentials file; requires --email")
	cmd.Flags().StringVar(&email, "email", "", "account email to label the token given with --token")
	cmd.Flags().StringVar(&priority, priorityFlag, "",
		"place the account in the rotation order instead of last: "+priorityUsage)
	return cmd
}

// newShowCommand builds the `show` subcommand.
func newShowCommand(cfg *entities.Config) *cobra.Command {
	return &cobra.Command{
		Use:     "show <email>",
		Aliases: []string{"get"},
		Short:   "Show one enrolled account in detail, including when each of its limits resets",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			deps := newDeps(cfg)
			return commands.NewGetAccountCommand(
				deps.config, deps.accounts, deps.credentials, deps.usage, deps.tokens,
			).Execute(args[0])
		},
	}
}

// newUpdateCommand builds the `update` subcommand, which changes an enrolled
// account's place in the rotation order.
func newUpdateCommand(cfg *entities.Config) *cobra.Command {
	var priority string
	cmd := &cobra.Command{
		Use:   "update <email>",
		Short: "Change an enrolled account's priority in the rotation order",
		Long: "Move an enrolled account to another place in the rotation order.\n\n" +
			"With --prefer-primary (the default) the monitor always runs on the highest-priority " +
			"account that has capacity, so a running daemon switches to an account moved above the " +
			"active one on its next poll, and away from the active one when it is moved below an " +
			"account with capacity.",
		Example: "  ccswitch update backup@example.com --priority top\n" +
			"  ccswitch update backup@example.com --priority 2\n" +
			"  ccswitch update backup@example.com --priority down",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			parsed, err := entities.ParsePriority(priority)
			if err != nil {
				return err
			}
			deps := newDeps(cfg)
			return commands.NewUpdateAccountCommand(deps.accounts).Execute(args[0], parsed)
		},
	}
	cmd.Flags().StringVar(&priority, priorityFlag, "", "where to move the account: "+priorityUsage)
	// MarkFlagRequired fails only for a flag that was never defined, and the line
	// above defines it.
	_ = cmd.MarkFlagRequired(priorityFlag)
	return cmd
}

// newRemoveCommand builds the `remove` subcommand.
func newRemoveCommand(cfg *entities.Config) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <email>...",
		Aliases: []string{"rm", "delete"},
		Short:   "Remove enrolled accounts from the store",
		Long: "Remove enrolled accounts, and their stored tokens, from the store. Nothing is removed " +
			"unless every named account is enrolled.\n\n" +
			"Removing the active account hands over to the highest-priority remaining account that " +
			"has capacity. Its credentials are installed at once, or on the next launch while a " +
			"claude session is running.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			deps := newDeps(cfg)
			return commands.NewDeleteAccountsCommand(deps.accounts, deps.credentials, deps.sessions).
				Execute(args)
		},
	}
}

// newReorderCommand builds the `reorder` subcommand.
func newReorderCommand(cfg *entities.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "reorder <email>...",
		Short: "Set the rotation order: the named accounts first, in the order given",
		Long: "Put the named accounts at the front of the rotation order, in the order given. " +
			"Accounts left out keep their relative order behind them, so naming the ones that " +
			"should come first is enough. The first account is the primary.",
		Example: "  ccswitch reorder work@example.com personal@example.com",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			deps := newDeps(cfg)
			return commands.NewReorderAccountsCommand(deps.accounts).Execute(args)
		},
	}
}

// newListCommand builds the `list` subcommand.
func newListCommand(cfg *entities.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List enrolled accounts with their live usage",
		RunE: func(_ *cobra.Command, _ []string) error {
			deps := newDeps(cfg)
			return commands.NewListAccountsCommand(
				deps.config, deps.accounts, deps.credentials, deps.usage, deps.tokens,
			).Execute()
		},
	}
}

// newStatusCommand builds the `status` subcommand.
func newStatusCommand(cfg *entities.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active account and its usage",
		RunE: func(_ *cobra.Command, _ []string) error {
			deps := newDeps(cfg)
			return commands.NewStatusCommand(
				deps.config, deps.accounts, deps.credentials, deps.usage, deps.tokens,
			).Execute()
		},
	}
}

// newUseCommand builds the `use` subcommand.
func newUseCommand(cfg *entities.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "use <email>",
		Short: "Switch the active Claude account to a specific enrolled one",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			deps := newDeps(cfg)
			return commands.NewUseAccountCommand(deps.accounts, deps.credentials).Execute(args[0])
		},
	}
}

// newRotateCommand builds the `rotate` subcommand.
func newRotateCommand(cfg *entities.Config) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate to the next healthy backup account",
		RunE: func(_ *cobra.Command, _ []string) error {
			deps := newDeps(cfg)
			return commands.NewRotateAccountCommand(deps.accounts, deps.credentials, deps.sessions).Execute(force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "switch even if a claude session is running")
	return cmd
}

// newEnsureCommand builds the `ensure` subcommand.
func newEnsureCommand(cfg *entities.Config) *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   ensureCommandName,
		Short: "Install the current account's credentials if not already active (no network)",
		RunE: func(_ *cobra.Command, _ []string) error {
			deps := newDeps(cfg)
			return commands.NewEnsureActiveCommand(deps.accounts, deps.credentials).Execute(quiet)
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress output on success")
	return cmd
}

// newVersionCommand builds the `version` subcommand, which prints the bare version
// so scripts can read it; `ccswitch --version` prints the labelled form.
func newVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   versionCommandName,
		Short: "Print the ccswitch version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version)
			return nil
		},
	}
}
