package controllers

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// newEnrollCommand builds the `enroll` subcommand.
func newEnrollCommand(cfg *entities.Config) *cobra.Command {
	var token, email string
	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Capture the currently logged-in Claude account into the store",
		RunE: func(_ *cobra.Command, _ []string) error {
			deps := newDeps(cfg)
			return commands.NewEnrollAccountCommand(deps.accounts, deps.credentials).Execute(token, email)
		},
	}
	cmd.Flags().StringVar(&token, "token", "",
		"enroll a long-lived OAuth token directly (e.g. from claude setup-token) instead of "+
			"reading the Claude Code credentials file; requires --email")
	cmd.Flags().StringVar(&email, "email", "", "account email to label the token given with --token")
	return cmd
}

// newListCommand builds the `list` subcommand.
func newListCommand(cfg *entities.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List enrolled accounts with their live usage",
		RunE: func(_ *cobra.Command, _ []string) error {
			deps := newDeps(cfg)
			return commands.NewListAccountsCommand(deps.config, deps.accounts, deps.usage, deps.tokens).Execute()
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
			return commands.NewStatusCommand(deps.config, deps.accounts, deps.usage, deps.tokens).Execute()
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
		Use:   "ensure",
		Short: "Install the current account's credentials if not already active (no network)",
		RunE: func(_ *cobra.Command, _ []string) error {
			deps := newDeps(cfg)
			return commands.NewEnsureActiveCommand(deps.accounts, deps.credentials).Execute(quiet)
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress output on success")
	return cmd
}

// newVersionCommand builds the `version` subcommand.
func newVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the ccswitch version",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Fprintln(os.Stdout, version)
			return nil
		},
	}
}
