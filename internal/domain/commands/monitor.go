package commands

import (
	"context"
	"fmt"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

const defaultCooldown = time.Hour

// MonitorCommand is the always-on daemon loop that polls usage and rotates the
// active account when it is exhausted.
type MonitorCommand struct {
	config      *entities.Config
	accounts    repositories.AccountsRepository
	credentials repositories.CredentialsRepository
	usage       repositories.UsageRepository
	tokens      repositories.TokensRepository
	sessions    repositories.SessionsRepository
	now         func() time.Time
}

// NewMonitorCommand creates a MonitorCommand.
func NewMonitorCommand(
	config *entities.Config,
	accounts repositories.AccountsRepository,
	credentials repositories.CredentialsRepository,
	usage repositories.UsageRepository,
	tokens repositories.TokensRepository,
	sessions repositories.SessionsRepository,
) *MonitorCommand {
	return &MonitorCommand{
		config:      config,
		accounts:    accounts,
		credentials: credentials,
		usage:       usage,
		tokens:      tokens,
		sessions:    sessions,
		now:         time.Now,
	}
}

// Run polls at the configured interval until the context is cancelled.
func (c *MonitorCommand) Run(ctx context.Context) error {
	logger.Infof("[ccswitch] monitor started (interval %s, threshold %.0f%%)",
		c.config.Interval, c.config.Threshold)
	c.tickOnce()

	ticker := time.NewTicker(c.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("[ccswitch] monitor stopping")
			return nil
		case <-ticker.C:
			c.tickOnce()
		}
	}
}

// tickOnce runs one cycle and logs any error without stopping the loop.
func (c *MonitorCommand) tickOnce() {
	if err := c.Tick(c.now()); err != nil {
		logger.Warnf("[ccswitch] monitor tick failed: %v", err)
	}
}

// Tick performs one monitor cycle: capture the active credentials into the store,
// poll usage for the current account, and rotate when it is exhausted.
func (c *MonitorCommand) Tick(now time.Time) error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	if len(store.Accounts) == 0 {
		return nil
	}

	store.Rotation.ClearExpired(now)
	c.syncActiveCredentials(store)

	current := c.resolveCurrent(store)
	if current == nil {
		return c.accounts.Save(store)
	}

	usage, creds, pollErr := pollUsage(c.usage, c.tokens, &current.Credentials, now.UnixMilli())
	if pollErr != nil {
		logger.Warnf("[ccswitch] usage poll for %s failed: %v", current.Email, pollErr)
		return c.accounts.Save(store)
	}

	current.Credentials = creds
	current.LastUsage = usage
	current.LastPolledAt = now

	c.reconcile(store, current, usage, now)
	return c.accounts.Save(store)
}

// resolveCurrent returns the current account, defaulting to the first ordered
// account when the current pointer is unset or dangling.
func (c *MonitorCommand) resolveCurrent(store *entities.Store) *entities.Account {
	if account := store.FindAccount(store.Rotation.CurrentEmail); account != nil {
		return account
	}
	ordered := store.Ordered()
	if len(ordered) == 0 {
		return nil
	}
	store.Rotation.CurrentEmail = ordered[0].Email
	return store.FindAccount(store.Rotation.CurrentEmail)
}

// syncActiveCredentials captures the on-disk credentials, which Claude Code may
// have refreshed, back into the matching stored account so backup tokens stay
// current.
func (c *MonitorCommand) syncActiveCredentials(store *entities.Store) {
	onDisk, identity, err := c.credentials.Read()
	if err != nil {
		return
	}
	for i := range store.Accounts {
		if store.Accounts[i].Credentials.SameAccountAs(*onDisk) {
			store.Accounts[i].Credentials = *onDisk
			if identity != nil && identity.EmailAddress != "" {
				store.Accounts[i].Identity = *identity
			}
			return
		}
	}
}

// reconcile records how long the current account stays exhausted and then moves
// to whichever account the rotation policy selects. The exhaustion window is
// marked before the selection runs so the current account is excluded from
// being chosen as its own replacement.
func (c *MonitorCommand) reconcile(
	store *entities.Store,
	current *entities.Account,
	usage *entities.Usage,
	now time.Time,
) {
	exhausted := usage.Exhausted(c.config.Threshold)
	if exhausted {
		recovers := usage.RecoversAt(c.config.Threshold)
		if recovers.IsZero() {
			recovers = now.Add(defaultCooldown)
		}
		store.Rotation.MarkExhausted(current.Email, recovers)
	}

	target, ok := c.selectTarget(store, current, now, exhausted)
	if !ok {
		if exhausted {
			logger.Warnf("[ccswitch] %s exhausted and no account has capacity; recovers %s",
				current.Email, formatReset(store.Rotation.ExhaustedUntil[current.Email]))
		}
		return
	}
	c.switchTo(store, current, &target, usage, exhausted)
}

// selectTarget returns the account the rotation policy wants active and whether a
// switch is required. With PreferPrimary the highest-priority account that has
// capacity always wins, so the monitor returns to the primary as soon as its
// limits reset. Otherwise the next account is taken only once the current one is
// exhausted.
func (c *MonitorCommand) selectTarget(
	store *entities.Store,
	current *entities.Account,
	now time.Time,
	exhausted bool,
) (entities.Account, bool) {
	var none entities.Account

	if c.config.PreferPrimary {
		preferred, ok := store.PreferredAccount(now)
		if !ok || preferred.Email == current.Email {
			return none, false
		}
		// While the current account still has capacity, only move to one that
		// outranks it; never drop to a lower-priority account.
		if !exhausted && preferred.Order >= current.Order {
			return none, false
		}
		return preferred, true
	}

	if !exhausted {
		return none, false
	}
	next, ok := store.NextHealthyAccount(now)
	if !ok || next.Email == current.Email {
		return none, false
	}
	return next, true
}

// switchTo installs the target account, deferring the on-disk write while a
// claude session is running so a live process is never swapped underneath.
func (c *MonitorCommand) switchTo(
	store *entities.Store,
	current *entities.Account,
	target *entities.Account,
	usage *entities.Usage,
	exhausted bool,
) {
	reason := "higher-priority account recovered"
	if exhausted {
		binding, _ := usage.BindingLimit()
		reason = fmt.Sprintf("%s at %.0f%%", binding.Kind, binding.Percent)
	}

	if c.sessions != nil && c.sessions.ClaudeRunning() {
		store.Rotation.CurrentEmail = target.Email
		logger.Infof("[ccswitch] %s -> %s on next launch (%s; session active)",
			current.Email, target.Email, reason)
		return
	}

	if err := c.credentials.Write(&target.Credentials, &target.Identity); err != nil {
		logger.Warnf("[ccswitch] failed to install %s: %v", target.Email, err)
		return
	}
	store.Rotation.CurrentEmail = target.Email
	logger.Infof("[ccswitch] switched %s -> %s (%s)", current.Email, target.Email, reason)
}
