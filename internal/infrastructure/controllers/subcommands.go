package controllers

import (
	"errors"
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
			"immediately: every account of every provider is repolled and the highest-priority " +
			"one of each whose utilization is below the new threshold becomes active.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			command := newSetThresholdCommand(newDeps(cfg))
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

// newSetThresholdCommand builds the command that sets the threshold over every
// provider's accounts. Claude's pool comes first, since its store holds the settings
// every provider shares.
func newSetThresholdCommand(deps *deps) *commands.SetThresholdCommand {
	pools := deps.orderedPools(entities.Providers())
	command := commands.NewSetThresholdCommand(
		deps.config, pools[0].Accounts, pools[0].Credentials, pools[0].Usage, pools[0].Tokens, pools[0].Sessions,
	)
	for _, pool := range pools[1:] {
		command.WithPool(pool)
	}
	return command
}

// newEnrollCommand builds the `enroll` subcommand.
func newEnrollCommand(cfg *entities.Config, provider *providerValue) *cobra.Command {
	var token, email, priority string
	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Capture the account Claude Code (or, with --provider codex, Codex) is signed in to",
		Example: "  ccswitch enroll                     # the account Claude Code is logged in to\n" +
			"  ccswitch enroll --provider codex    # the ChatGPT account Codex is signed in to",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var placement *entities.Priority
			if cmd.Flags().Changed(priorityFlag) {
				parsed, err := entities.ParsePriority(priority)
				if err != nil {
					return err
				}
				placement = &parsed
			}
			pool := newDeps(cfg).pool(provider.orDefault())
			return commands.NewEnrollAccountCommand(pool.Accounts, pool.Credentials).
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
func newShowCommand(cfg *entities.Config, provider *providerValue) *cobra.Command {
	return &cobra.Command{
		Use:     "show <email>",
		Aliases: []string{"get"},
		Short:   "Show one enrolled account in detail, including when each of its limits resets",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			deps := newDeps(cfg)
			owner, err := providerOf(deps, provider, args[0])
			if err != nil {
				return err
			}
			pool := deps.pool(owner)
			return commands.NewGetAccountCommand(
				deps.config, pool.Accounts, pool.Credentials, pool.Usage, pool.Tokens,
			).Execute(args[0])
		},
	}
}

// newUpdateCommand builds the `update` subcommand, which changes an enrolled
// account's place in the rotation order.
func newUpdateCommand(cfg *entities.Config, provider *providerValue) *cobra.Command {
	var priority string
	cmd := &cobra.Command{
		Use:   "update <email>",
		Short: "Change an enrolled account's priority in its provider's rotation order",
		Long: "Move an enrolled account to another place in the rotation order of its provider.\n\n" +
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
			owner, err := providerOf(deps, provider, args[0])
			if err != nil {
				return err
			}
			return commands.NewUpdateAccountCommand(deps.pool(owner).Accounts).Execute(args[0], parsed)
		},
	}
	cmd.Flags().StringVar(&priority, priorityFlag, "", "where to move the account: "+priorityUsage)
	// MarkFlagRequired fails only for a flag that was never defined, and the line
	// above defines it.
	_ = cmd.MarkFlagRequired(priorityFlag)
	return cmd
}

// newRemoveCommand builds the `remove` subcommand.
func newRemoveCommand(cfg *entities.Config, provider *providerValue) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <email>...",
		Aliases: []string{"rm", "delete"},
		Short:   "Remove enrolled accounts from the store",
		Long: "Remove enrolled accounts, and their stored tokens, from the store. Nothing is removed " +
			"unless every named account is enrolled.\n\n" +
			"Removing the active account of a provider hands over to the highest-priority remaining " +
			"account of that provider that has capacity. Its credentials are installed at once, or on " +
			"the next launch while a session of that provider's CLI is running.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			deps := newDeps(cfg)
			groups, err := groupByProvider(deps, provider, args)
			if err != nil {
				return err
			}
			// Claude's accounts go first: an email enrolled with no provider lands among
			// them, and that command checks every email it is given before removing any,
			// so nothing is removed unless every named account is enrolled.
			for _, owner := range entities.Providers() {
				if len(groups[owner]) == 0 {
					continue
				}
				pool := deps.pool(owner)
				if err = commands.NewDeleteAccountsCommand(pool.Accounts, pool.Credentials, pool.Sessions).
					Execute(groups[owner]); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// newReorderCommand builds the `reorder` subcommand.
func newReorderCommand(cfg *entities.Config, provider *providerValue) *cobra.Command {
	return &cobra.Command{
		Use:   "reorder <email>...",
		Short: "Set the rotation order: the named accounts first, in the order given",
		Long: "Put the named accounts at the front of their provider's rotation order, in the order " +
			"given. Accounts left out keep their relative order behind them, so naming the ones that " +
			"should come first is enough. The first account is the primary. Each provider has an " +
			"order of its own, so the named accounts must all belong to one.",
		Example: "  ccswitch reorder work@example.com personal@example.com",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			deps := newDeps(cfg)
			owner, err := providerOfAll(deps, provider, args)
			if err != nil {
				return err
			}
			return commands.NewReorderAccountsCommand(deps.pool(owner).Accounts).Execute(args)
		},
	}
}

// newListCommand builds the `list` subcommand.
func newListCommand(cfg *entities.Config, provider *providerValue) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List enrolled accounts with their live usage, under the provider of each",
		RunE: func(_ *cobra.Command, _ []string) error {
			deps := newDeps(cfg)
			pools := deps.orderedPools(provider.listed())
			command := commands.NewListAccountsCommand(
				deps.config, pools[0].Accounts, pools[0].Credentials, pools[0].Usage, pools[0].Tokens,
			)
			for _, pool := range pools[1:] {
				command.WithPool(pool)
			}
			return command.Execute()
		},
	}
}

// newStatusCommand builds the `status` subcommand.
func newStatusCommand(cfg *entities.Config, provider *providerValue) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active account of each provider and its usage",
		RunE: func(_ *cobra.Command, _ []string) error {
			deps := newDeps(cfg)
			pools := deps.orderedPools(provider.listed())
			command := commands.NewStatusCommand(
				deps.config, pools[0].Accounts, pools[0].Credentials, pools[0].Usage, pools[0].Tokens,
			)
			for _, pool := range pools[1:] {
				command.WithPool(pool)
			}
			return command.Execute()
		},
	}
}

// newUseCommand builds the `use` subcommand.
func newUseCommand(cfg *entities.Config, provider *providerValue) *cobra.Command {
	return &cobra.Command{
		Use:   "use <email>",
		Short: "Switch the active account of the email's provider to that enrolled account",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			deps := newDeps(cfg)
			owner, err := providerOf(deps, provider, args[0])
			if err != nil {
				return err
			}
			pool := deps.pool(owner)
			return commands.NewUseAccountCommand(pool.Accounts, pool.Credentials).Execute(args[0])
		},
	}
}

// newRotateCommand builds the `rotate` subcommand.
func newRotateCommand(cfg *entities.Config, provider *providerValue) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate to the next healthy backup account (Claude's, or --provider codex for Codex's)",
		RunE: func(_ *cobra.Command, _ []string) error {
			pool := newDeps(cfg).pool(provider.orDefault())
			return commands.NewRotateAccountCommand(pool.Accounts, pool.Credentials, pool.Sessions).Execute(force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "switch even if a session of the provider's CLI is running")
	return cmd
}

// newEnsureCommand builds the `ensure` subcommand.
func newEnsureCommand(cfg *entities.Config, provider *providerValue) *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use: ensureCommandName,
		Short: "Install the current account's credentials if not already active " +
			"(Claude's, or --provider codex for Codex's; no network)",
		RunE: func(_ *cobra.Command, _ []string) error {
			pool := newDeps(cfg).pool(provider.orDefault())
			return commands.NewEnsureActiveCommand(pool.Accounts, pool.Credentials).Execute(quiet)
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress output on success")
	return cmd
}

// newCapacityCommand builds the `capacity` subcommand, which tells from the store
// alone whether an enrolled account has capacity left.
func newCapacityCommand(cfg *entities.Config, provider *providerValue) *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use: capacityCommandName,
		Short: "Tell whether an enrolled account has capacity left " +
			"(Claude's, or --provider codex for Codex's; no network)",
		Long: "Exit 0 while an enrolled account has capacity left, and exit " +
			strconv.Itoa(noCapacityExitCode) + " once every account the rotation can select is " +
			"exhausted, printing which one recovers first and when. It answers for Claude's accounts, " +
			"or for Codex's with --provider codex.\n\n" +
			"It reads the exhaustion the monitor last recorded in the store and makes no network " +
			"call, so a shell can run it before every launch. When ccswitch cannot tell -- " +
			"nothing enrolled, nothing pollable, or a long-lived account picked with `use` -- it " +
			"exits 0.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pool := newDeps(cfg).pool(provider.orDefault())
			err := commands.NewCheckCapacityCommand(pool.Accounts).
				WithOutput(cmd.OutOrStdout()).
				Execute(quiet)
			if errors.Is(err, commands.ErrNoCapacity) {
				// The answer was printed already; it is not a failure for cobra to
				// report as "Error: ...". ExitCode maps it to its own status.
				cmd.SilenceErrors = true
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "print nothing, and answer with the exit status alone")
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
