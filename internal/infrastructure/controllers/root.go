// Package controllers wires the ccswitch domain commands into the cobra CLI and
// builds the infrastructure adapters from the resolved configuration.
package controllers

import (
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/services"
)

const (
	defaultThreshold = 90.0
	defaultInterval  = 5 * time.Minute
	// defaultClientID is Claude Code's public OAuth client identifier, used to
	// refresh access tokens for backup accounts.
	defaultClientID  = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	defaultUsageBase = "https://api.anthropic.com"
	defaultTokenURL  = "https://platform.claude.com/v1/oauth/token" //nolint:gosec // public endpoint, not a secret
	pidFileName      = "monitor.pid"
	logFileName      = "monitor.log"
)

// deps bundles the infrastructure adapters and config shared by the subcommands.
type deps struct {
	config      *entities.Config
	accounts    *repositories.JSONAccountsRepository
	credentials domain.CredentialsRepository
	usage       *repositories.AnthropicUsageRepository
	tokens      *repositories.AnthropicTokensRepository
	sessions    domain.SessionsRepository
	daemon      *services.DaemonService
}

// NewRootCommand builds the ccswitch root command with every subcommand wired.
func NewRootCommand(version string) *cobra.Command {
	cfg := defaultConfig()

	root := &cobra.Command{
		Use:           "ccswitch",
		Short:         "Monitor Claude Code usage and rotate between backup accounts",
		Long:          "ccswitch watches Claude Code usage limits and transparently rotates between enrolled backup accounts when the active account runs out.",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	bindPersistentFlags(root, cfg)

	root.AddCommand(
		newEnrollCommand(cfg),
		newListCommand(cfg),
		newStatusCommand(cfg),
		newUseCommand(cfg),
		newRotateCommand(cfg),
		newEnsureCommand(cfg),
		newMonitorCommand(cfg),
		newVersionCommand(version),
	)
	return root
}

// bindPersistentFlags attaches the flags shared by all subcommands, writing into
// the shared config which is read after cobra parses them.
func bindPersistentFlags(root *cobra.Command, cfg *entities.Config) {
	flags := root.PersistentFlags()
	flags.Float64Var(&cfg.Threshold, "threshold", cfg.Threshold,
		"utilization percent (0-100) that triggers rotation")
	flags.DurationVar(&cfg.Interval, "interval", cfg.Interval, "monitor poll interval")
	flags.BoolVar(&cfg.PreferPrimary, "prefer-primary", cfg.PreferPrimary,
		"always run on the highest-priority account that has capacity, returning to it "+
			"as soon as its limits reset (use --prefer-primary=false for round-robin)")
	flags.StringVar(&cfg.StorePath, "store", cfg.StorePath, "path to the ccswitch account store")
	flags.StringVar(&cfg.CredentialsPath, "credentials", cfg.CredentialsPath,
		"path to Claude Code .credentials.json")
	flags.StringVar(&cfg.ClaudeJSONPath, "claude-json", cfg.ClaudeJSONPath,
		"path to Claude Code ~/.claude.json (for the oauthAccount identity)")
}

// newDeps constructs the infrastructure adapters from the resolved config. It is
// called inside each subcommand's RunE so it observes final flag values.
func newDeps(cfg *entities.Config) *deps {
	stateDir := filepath.Dir(cfg.StorePath)
	return &deps{
		config:      cfg,
		accounts:    repositories.NewJSONAccountsRepository(cfg.StorePath),
		credentials: newCredentialsRepository(cfg),
		usage:       repositories.NewAnthropicUsageRepository(cfg.UsageBaseURL, nil),
		tokens:      repositories.NewAnthropicTokensRepository(cfg.TokenURL, cfg.ClientID, nil),
		sessions:    newSessionsRepository(),
		daemon: services.NewDaemonService(
			filepath.Join(stateDir, pidFileName),
			filepath.Join(stateDir, logFileName),
		),
	}
}

// defaultConfig returns the configuration seeded from the user's home directory
// and the built-in defaults.
func defaultConfig() *entities.Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return &entities.Config{
		CredentialsPath: filepath.Join(home, ".claude", ".credentials.json"),
		ClaudeJSONPath:  filepath.Join(home, ".claude.json"),
		StorePath:       defaultStorePath(home),
		Threshold:       defaultThreshold,
		Interval:        defaultInterval,
		PreferPrimary:   true,
		UsageBaseURL:    defaultUsageBase,
		TokenURL:        defaultTokenURL,
		ClientID:        defaultClientID,
	}
}

// defaultStorePath returns the ccswitch store location, honoring XDG_STATE_HOME.
func defaultStorePath(home string) string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "ccswitch", "store.json")
}
