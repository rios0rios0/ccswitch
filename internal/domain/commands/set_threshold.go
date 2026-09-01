package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// SetThresholdCommand persists the rotation threshold and applies it on the spot.
type SetThresholdCommand struct {
	config   *entities.Config
	accounts repositories.AccountsRepository
	// monitor re-applies the rotation policy under the new threshold. Reusing it
	// rather than reimplementing the selection is what makes `threshold` and the
	// daemon agree on which account should be active.
	monitor *MonitorCommand
	now     func() time.Time
}

// NewSetThresholdCommand creates a SetThresholdCommand.
func NewSetThresholdCommand(
	config *entities.Config,
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
	usage repositories.UsageRepository,
	tokens repositories.TokensRepository,
	sessions repositories.SessionsRepository,
) *SetThresholdCommand {
	return &SetThresholdCommand{
		config:   config,
		accounts: accounts,
		monitor:  NewMonitorCommand(config, accounts, credentials, usage, tokens, sessions),
		now:      time.Now,
	}
}

// Show prints the threshold in force and where it comes from.
func (c *SetThresholdCommand) Show() error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "rotation threshold: %.0f%% (%s)\n",
		c.config.ResolveThreshold(store.Settings), c.source(store.Settings))
	return nil
}

// source names where the threshold in force comes from, so that a --threshold
// shadowing a persisted value is visible rather than confusing.
func (c *SetThresholdCommand) source(settings entities.Settings) string {
	switch {
	case c.config.ThresholdExplicit && settings.Threshold != nil:
		return fmt.Sprintf("from --threshold, overriding the stored %.0f%%", *settings.Threshold)
	case c.config.ThresholdExplicit:
		return "from --threshold"
	case settings.Threshold != nil:
		return "stored"
	default:
		return "default"
	}
}

// Reset drops the persisted threshold so the built-in default applies again, and
// re-applies the rotation policy under it.
func (c *SetThresholdCommand) Reset() error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	if store.Settings.Threshold == nil {
		fmt.Fprintf(os.Stdout, "[ccswitch] rotation threshold already at the default %.0f%%\n",
			c.config.Threshold)
		return nil
	}

	previous := *store.Settings.Threshold
	store.Settings.Threshold = nil
	if err = c.accounts.Save(store); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "[ccswitch] rotation threshold reset to the default %.0f%% (was %.0f%%)\n",
		c.config.Threshold, previous)

	if len(store.Accounts) == 0 {
		return nil
	}
	if err = c.monitor.Tick(c.now()); err != nil {
		return fmt.Errorf("failed to re-apply the rotation policy: %w", err)
	}
	return c.report()
}

// Execute persists the given threshold and immediately re-applies the rotation
// policy under it.
//
// Persisting it is what makes the change take hold in flight: the daemon reloads
// the store on every tick and reads the threshold from there, so it retunes
// without being restarted. Re-applying it here is what makes the change take
// hold *now* rather than at the next poll — the monitor cycle repolls every
// account, rewrites the exhaustion markers that the old threshold produced, and
// activates the first account in rotation order whose utilization is below the
// new one.
func (c *SetThresholdCommand) Execute(threshold float64) error {
	if threshold <= 0 || threshold > entities.MaxThreshold {
		return fmt.Errorf(
			"threshold must be greater than 0 and at most %.0f, got %.0f",
			entities.MaxThreshold, threshold)
	}

	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	previous := c.config.ResolveThreshold(store.Settings)
	store.Settings.SetThreshold(threshold)
	if err = c.accounts.Save(store); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "[ccswitch] rotation threshold set to %.0f%% (was %.0f%%)\n",
		threshold, previous)

	if c.config.ThresholdExplicit {
		fmt.Fprintf(os.Stderr,
			"[ccswitch] WARN: --threshold %.0f was also passed and shadows the stored value "+
				"for this invocation\n", c.config.Threshold)
	}
	if len(store.Accounts) == 0 {
		return nil
	}

	if err = c.monitor.Tick(c.now()); err != nil {
		return fmt.Errorf("failed to re-apply the rotation policy: %w", err)
	}
	return c.report()
}

// report prints which account the re-applied policy selected, and whether the
// switch is still waiting on a running session.
func (c *SetThresholdCommand) report() error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	current := store.FindAccount(store.Rotation.CurrentEmail)
	if current == nil {
		fmt.Fprintln(os.Stdout, "[ccswitch] no account selected; run `ccswitch enroll`")
		return nil
	}

	if c.pendingLaunch(store, current) {
		fmt.Fprintf(os.Stdout,
			"[ccswitch] active account: %s (installed on next claude launch; a session is running)\n",
			current.Email)
		return nil
	}
	fmt.Fprintf(os.Stdout, "[ccswitch] active account: %s\n", current.Email)
	return nil
}

// pendingLaunch reports whether the selected account is chosen but not yet
// installed, which is how a switch looks while a claude session holds the old
// credentials.
func (c *SetThresholdCommand) pendingLaunch(
	store *entities.Store,
	current *entities.Account,
) bool {
	onDisk, identity, err := c.monitor.credentials.Read()
	if err != nil || onDisk == nil {
		return false
	}
	return store.MatchAccount(*onDisk, identity) != current
}
