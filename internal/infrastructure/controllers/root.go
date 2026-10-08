// Package controllers wires the ccswitch domain commands into the cobra CLI and
// builds the infrastructure adapters from the resolved configuration.
package controllers

import (
	"os"
	"path/filepath"
	"slices"
	"time"

	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/services"
)

const (
	defaultThreshold = 99.0
	defaultInterval  = 5 * time.Minute
	// defaultClientID is Claude Code's public OAuth client identifier, used to
	// refresh access tokens for backup accounts.
	defaultClientID  = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	defaultUsageBase = "https://api.anthropic.com"
	defaultTokenURL  = "https://platform.claude.com/v1/oauth/token" //nolint:gosec // public endpoint, not a secret
	pidFileName      = "monitor.pid"
	logFileName      = "monitor.log"

	// binaryName is the name ccswitch is installed and released under; its GitHub
	// releases live at releaseOwner/releaseRepository.
	binaryName        = "ccswitch"
	releaseOwner      = "rios0rios0"
	releaseRepository = "ccswitch"

	// verboseFlag turns on debug logging, and the monitor passes it on to the daemon
	// it starts.
	verboseFlag = "verbose"

	ensureCommandName     = "ensure"
	monitorCommandName    = "monitor"
	selfUpdateCommandName = "self-update"
	versionCommandName    = "version"
	// helpCommandName and completionCommandName are the commands cobra adds itself.
	helpCommandName       = "help"
	completionCommandName = "completion"
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

// NewRootCommand builds the ccswitch root command with every subcommand wired,
// updating itself from the GitHub releases of the version it was built as.
func NewRootCommand(version string) *cobra.Command {
	return newRootCommand(version, repositories.NewCliforgeSelfUpdateRepository(
		releaseOwner, releaseRepository, binaryName, version,
	))
}

// newRootCommand builds the root command over the given release channel, which
// tests replace so that no invocation reaches GitHub.
func newRootCommand(version string, updates domain.SelfUpdateRepository) *cobra.Command {
	cfg := defaultConfig()

	root := &cobra.Command{
		Use:           binaryName,
		Short:         "Monitor Claude Code usage and rotate between backup accounts",
		Long:          "ccswitch watches Claude Code usage limits and transparently rotates between enrolled backup accounts when the active account runs out.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	color := colorAuto
	bindPersistentFlags(root, cfg, &color)
	root.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		// Whether --threshold was named decides who wins between the flag and the
		// value `ccswitch threshold` persisted, so it has to be read after parsing.
		cfg.ThresholdExplicit = cmd.Flags().Changed("threshold")
		cfg.Output = outputStyle(color, os.Stdout)
		if cfg.Verbose {
			logger.SetLevel(logger.DebugLevel)
		}
		if checksForUpdates(cmd) {
			updates.CheckForUpdates()
		}
	}

	root.AddCommand(
		newEnrollCommand(cfg),
		newListCommand(cfg),
		newShowCommand(cfg),
		newUpdateCommand(cfg),
		newRemoveCommand(cfg),
		newReorderCommand(cfg),
		newStatusCommand(cfg),
		newUseCommand(cfg),
		newRotateCommand(cfg),
		newThresholdCommand(cfg),
		newEnsureCommand(cfg),
		newMonitorCommand(cfg),
		newSelfUpdateCommand(cfg, updates),
		newVersionCommand(version),
	)
	return root
}

// checksForUpdates reports whether running cmd also checks for a newer release.
//
// The commands about the version itself skip it, as does cobra's help, and so do
// the ones whose notice nobody would read. ensure runs before every claude launch
// and is promised to stay off the network. monitor's daemon would see its lookup
// answer, marking the day as checked with the warning written only to its log.
// Shell completion runs on every shell start and TAB press and exits at once, so
// it would use up the day's few lookups without an answer.
func checksForUpdates(cmd *cobra.Command) bool {
	// `completion bash` is cobra's completion command, so it is judged by the
	// command directly under the root.
	for cmd.HasParent() && cmd.Parent().HasParent() {
		cmd = cmd.Parent()
	}
	return !slices.Contains([]string{
		versionCommandName, selfUpdateCommandName,
		ensureCommandName, monitorCommandName,
		helpCommandName, completionCommandName, cobra.ShellCompRequestCmd,
	}, cmd.Name())
}

// bindPersistentFlags attaches the flags shared by all subcommands, writing into
// the shared config, and the color mode, which are read after cobra parses them.
func bindPersistentFlags(root *cobra.Command, cfg *entities.Config, color *colorMode) {
	flags := root.PersistentFlags()
	flags.Var(color, colorFlag,
		"when to color list and show: auto (on a terminal, unless NO_COLOR is set), always, or never")
	// pflag shows the first back-quoted word of a usage as the name of the flag's
	// value, so back-quotes in a usage name the value and nothing else.
	flags.Float64Var(&cfg.Threshold, "threshold", cfg.Threshold,
		"utilization `percent` (0-100) that triggers rotation; overrides the value stored by "+
			"the threshold command for this invocation only")
	flags.DurationVar(&cfg.Interval, "interval", cfg.Interval, "monitor poll interval")
	flags.BoolVar(&cfg.PreferPrimary, "prefer-primary", cfg.PreferPrimary,
		"always run on the highest-priority account that has capacity, returning to it "+
			"as soon as its limits reset (use --prefer-primary=false for round-robin)")
	flags.StringVar(&cfg.StorePath, "store", cfg.StorePath, "path to the ccswitch account store")
	flags.StringVar(&cfg.CredentialsPath, "credentials", cfg.CredentialsPath,
		"path to Claude Code .credentials.json")
	flags.StringVar(&cfg.ClaudeJSONPath, "claude-json", cfg.ClaudeJSONPath,
		"path to Claude Code ~/.claude.json (for the oauthAccount identity)")
	flags.BoolVarP(&cfg.Verbose, verboseFlag, "v", cfg.Verbose,
		"enable debug logging, which a monitor daemon started by this invocation keeps")
}

// newDeps constructs the infrastructure adapters from the resolved config. It is
// called inside each subcommand's RunE so it observes final flag values.
func newDeps(cfg *entities.Config) *deps {
	return &deps{
		config:      cfg,
		accounts:    repositories.NewJSONAccountsRepository(cfg.StorePath),
		credentials: newCredentialsRepository(cfg),
		usage:       repositories.NewAnthropicUsageRepository(cfg.UsageBaseURL, nil),
		tokens:      repositories.NewAnthropicTokensRepository(cfg.TokenURL, cfg.ClientID, nil),
		sessions:    newSessionsRepository(),
		daemon:      newDaemonService(cfg),
	}
}

// newDaemonService supervises the monitor daemon through the pidfile and log kept
// beside the store.
func newDaemonService(cfg *entities.Config) *services.DaemonService {
	stateDir := filepath.Dir(cfg.StorePath)
	return services.NewDaemonService(
		filepath.Join(stateDir, pidFileName),
		filepath.Join(stateDir, logFileName),
	)
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
