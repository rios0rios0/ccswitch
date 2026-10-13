package commands

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

// SetThresholdCommand persists the rotation threshold and applies it on the spot.
type SetThresholdCommand struct {
	config *entities.Config
	// accounts is the store the threshold is persisted in: the first pool's, which
	// holds the settings every provider shares.
	accounts repositories.AccountsRepository
	// pools are the providers the threshold is applied to, the first one being the one
	// the constructor was given.
	pools []Pool
	// monitor re-applies the rotation policy under the new threshold. Reusing it
	// rather than reimplementing the selection is what makes `threshold` and the
	// daemon agree on which account should be active.
	monitor *MonitorCommand
	now     func() time.Time
}

// NewSetThresholdCommand creates a SetThresholdCommand over the accounts of one
// provider, whose store holds the settings. More providers join through WithPool.
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
		pools: []Pool{{
			Accounts:    accounts,
			Credentials: credentials,
			Usage:       usage,
			Tokens:      tokens,
			Sessions:    sessions,
		}},
		monitor: NewMonitorCommand(config, accounts, credentials, usage, tokens, sessions),
		now:     time.Now,
	}
}

// WithPool applies the threshold to another provider's accounts as well. The
// threshold itself stays in the first pool's store, which every provider reads it
// from.
func (c *SetThresholdCommand) WithPool(pool Pool) *SetThresholdCommand {
	c.pools = append(c.pools, pool)
	c.monitor.WithPool(pool)
	return c
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
			c.config.ResolveThreshold(store.Settings))
		return nil
	}

	previous := *store.Settings.Threshold
	store.Settings.Threshold = nil
	if err = c.accounts.Save(store); err != nil {
		return err
	}
	// Report what will actually apply, which is the flag when one was passed --
	// announcing the built-in default there would name a number nothing uses.
	fmt.Fprintf(os.Stdout, "[ccswitch] rotation threshold reset to %.0f%% (was %.0f%%)\n",
		c.config.ResolveThreshold(store.Settings), previous)
	c.warnShadowed()
	return c.reapply()
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
	// NaN is rejected explicitly: every comparison against it is false, so a bare
	// range check admits it — and `strconv.ParseFloat` accepts the literal "NaN".
	// Stored, it would make `Percent >= threshold` false for every limit and
	// silently disable rotation altogether.
	if math.IsNaN(threshold) || threshold <= 0 || threshold > entities.MaxThreshold {
		return fmt.Errorf(
			"threshold must be greater than 0 and at most %.0f, got %v",
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

	c.warnShadowed()
	return c.reapply()
}

// reapply runs a monitor cycle under the threshold now in force, in every pool, and
// reports which account each provider has active afterwards. With nothing enrolled
// anywhere there is nothing to apply it to.
func (c *SetThresholdCommand) reapply() error {
	stores, err := loadStores(c.pools)
	if err != nil {
		return err
	}
	if enrolledCount(stores) == 0 {
		return nil
	}

	if err = c.monitor.Tick(c.now()); err != nil {
		return fmt.Errorf("failed to re-apply the rotation policy: %w", err)
	}
	return c.report()
}

// warnShadowed points out that an explicit --threshold outranks the stored value
// for this invocation, so a command that just wrote one does not look ignored.
func (c *SetThresholdCommand) warnShadowed() {
	if !c.config.ThresholdExplicit {
		return
	}
	fmt.Fprintf(os.Stderr,
		"[ccswitch] WARN: --threshold %.0f was also passed and shadows the stored value "+
			"for this invocation\n", c.config.Threshold)
}

// report prints which account the re-applied policy selected for each provider that
// has accounts, and whether the switch is still waiting on a running session. Once a
// provider other than Claude has accounts, each line names its provider.
func (c *SetThresholdCommand) report() error {
	stores, err := loadStores(c.pools)
	if err != nil {
		return err
	}

	named := namesProviders(stores)
	for i, store := range stores {
		if len(store.Accounts) == 0 {
			continue
		}
		reportActive(c.pools[i], store, named)
	}
	return nil
}

// reportActive prints the account one provider has active, naming the provider when
// named.
func reportActive(pool Pool, store *entities.Store, named bool) {
	whose := ""
	if named {
		whose = store.Provider.Name() + " "
	}

	current := store.FindAccount(store.Rotation.CurrentEmail)
	if current == nil {
		fmt.Fprintf(os.Stdout, "[ccswitch] no %saccount selected; run `ccswitch enroll`\n", whose)
		return
	}
	if pendingLaunch(pool, store, current) {
		fmt.Fprintf(os.Stdout,
			"[ccswitch] active %saccount: %s (installed on next %s launch; a session is running)\n",
			whose, current.Email, store.Provider.Command())
		return
	}
	fmt.Fprintf(os.Stdout, "[ccswitch] active %saccount: %s\n", whose, current.Email)
}

// pendingLaunch reports whether the selected account is chosen but not yet
// installed, which is how a switch looks while a session of the provider's CLI holds
// the old credentials.
func pendingLaunch(pool Pool, store *entities.Store, current *entities.Account) bool {
	onDisk, identity, err := pool.Credentials.Read()
	if err != nil || onDisk == nil {
		return false
	}
	return store.MatchAccount(*onDisk, identity) != current
}
