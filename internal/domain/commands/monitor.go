package commands

import (
	"context"
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

	if usage.Exhausted(c.config.Threshold) {
		c.rotateAway(store, current, usage, now)
	}
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

// rotateAway marks the exhausted account and switches to the next healthy one,
// deferring the on-disk switch when a claude session is running.
func (c *MonitorCommand) rotateAway(
	store *entities.Store,
	current *entities.Account,
	usage *entities.Usage,
	now time.Time,
) {
	reset := usage.EarliestReset()
	if reset.IsZero() {
		reset = now.Add(defaultCooldown)
	}
	store.Rotation.MarkExhausted(current.Email, reset)

	next, ok := store.NextHealthyAccount(now)
	if !ok || next.Email == current.Email {
		logger.Warnf("[ccswitch] %s exhausted and no healthy backup; soonest reset %s",
			current.Email, formatReset(reset))
		return
	}

	if c.sessions != nil && c.sessions.ClaudeRunning() {
		store.Rotation.CurrentEmail = next.Email
		logger.Infof("[ccswitch] %s exhausted; will switch to %s on next launch (session active)",
			current.Email, next.Email)
		return
	}

	if err := c.credentials.Write(&next.Credentials, &next.Identity); err != nil {
		logger.Warnf("[ccswitch] failed to install %s: %v", next.Email, err)
		return
	}
	store.Rotation.CurrentEmail = next.Email
	binding, _ := usage.BindingLimit()
	logger.Infof("[ccswitch] rotated %s -> %s (%s at %.0f%%)",
		current.Email, next.Email, binding.Kind, binding.Percent)
}
