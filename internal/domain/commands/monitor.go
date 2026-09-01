package commands

import (
	"context"
	"fmt"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
)

const (
	defaultCooldown = time.Hour
	// backupPollInterval is how often an account other than the active one is
	// polled. The active account is polled every tick because the rotation
	// decision turns on it; a backup only has to be seen often enough to keep its
	// token alive and to notice its limits reset, and polling every account on
	// every tick multiplies request volume against an endpoint that rate-limits.
	backupPollInterval = 15 * time.Minute
)

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
		c.config.Interval, c.effectiveThreshold())
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

// effectiveThreshold reads the threshold that applies right now, which means
// reloading the store: `ccswitch threshold` persists a new value there and a
// running daemon has to honour it without being restarted. A store that cannot
// be read falls back to the configured value.
func (c *MonitorCommand) effectiveThreshold() float64 {
	store, err := c.accounts.Load()
	if err != nil {
		return c.config.Threshold
	}
	return c.config.ResolveThreshold(store.Settings)
}

// Tick performs one monitor cycle: capture the active credentials into the store,
// poll every account that can be polled, and apply the rotation policy.
func (c *MonitorCommand) Tick(now time.Time) error {
	store, err := c.accounts.Load()
	if err != nil {
		return err
	}
	if len(store.Accounts) == 0 {
		return nil
	}

	threshold := c.config.ResolveThreshold(store.Settings)
	store.Rotation.ClearExpired(now)
	c.syncActiveCredentials(store)

	current := c.resolveCurrent(store)
	if current == nil {
		return c.accounts.Save(store)
	}

	c.pollAll(store, current, now, threshold)
	c.reconcile(store, current, now)
	c.installPending(store)
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

// pollAll polls every account whose usage can be read — not only the active one —
// and rewrites its exhaustion marker from what the poll saw.
//
// Polling the backups is what keeps them usable. A refresh token is only valid
// for a few weeks and is rotated on every use, so an account that is never
// touched between rotations goes stale in the store; installing it then hands
// Claude Code a token the server has already forgotten, which answers
// invalid_grant, which is the logout. Polling every account also keeps the
// rotation decision honest: an exhausted account is released the moment its
// limits actually reset rather than when its recorded reset time says so, and a
// threshold change takes effect against fresh numbers instead of stale markers.
func (c *MonitorCommand) pollAll(
	store *entities.Store,
	current *entities.Account,
	now time.Time,
	threshold float64,
) {
	for _, ordered := range store.Ordered() {
		account := store.FindAccount(ordered.Email)
		if account == nil || !account.SupportsUsagePolling() {
			continue
		}
		if !shouldPoll(account, current, now) {
			continue
		}
		c.pollAccount(store, account, now, threshold)
	}
}

// shouldPoll reports whether this tick polls the given account.
func shouldPoll(account, current *entities.Account, now time.Time) bool {
	if account.Email == current.Email {
		return true
	}
	// A spent or stripped token is attended to at once rather than on the slow
	// cadence: a backup whose credentials have gone stale is precisely the one a
	// rotation is about to install, and refreshing it late is the logout.
	if account.Credentials.AccessTokenExpired(now.UnixMilli()) || account.Credentials.Degraded() {
		return true
	}
	return now.Sub(account.LastPolledAt) >= backupPollInterval
}

// pollAccount polls one account, persists whatever the poll refreshed, and marks
// or clears its exhaustion.
func (c *MonitorCommand) pollAccount(
	store *entities.Store,
	account *entities.Account,
	now time.Time,
	threshold float64,
) {
	previous := account.Credentials
	usage, creds, err := pollUsage(c.usage, c.tokens, &account.Credentials, now.UnixMilli())
	// Keep the credentials even when the poll failed: a refresh that already
	// succeeded rotated the token server-side and cannot be undone, so discarding
	// it would strand the store on a token the server has invalidated. When the
	// refresh itself failed, pollUsage hands back the untouched credentials and
	// both calls below are no-ops.
	account.Credentials = creds
	publishRefreshed(c.credentials, previous, account)

	// Record the attempt, not just the success: this timestamp is the backup poll
	// cadence's only input, so leaving it behind on a failure would make a failing
	// account the one polled on every single tick — and the endpoint pushing back
	// is exactly the condition the cadence exists to survive.
	account.LastPolledAt = now

	if err != nil {
		logger.Warnf("[ccswitch] usage poll for %s failed: %v", account.Email, err)
		return
	}

	account.LastUsage = usage
	if account.Credentials.Degraded() {
		// The repair refresh in pollUsage ran and the scopes still fail the
		// invariant, so the endpoint is refusing to mint them. Retrying forever
		// would burn a refresh token per poll, so say so instead of looping quietly.
		logger.Warnf("[ccswitch] %s still lacks the %q scope after a refresh; "+
			"Claude Code will discard its credentials — log in again with `claude` and re-enroll",
			account.Email, entities.ScopeInference)
	}

	if !usage.Exhausted(threshold) {
		store.Rotation.ClearExhausted(account.Email)
		return
	}
	recovers := usage.RecoversAt(threshold)
	if recovers.IsZero() {
		recovers = now.Add(defaultCooldown)
	}
	store.Rotation.MarkExhausted(account.Email, recovers)
}

// reconcile moves to whichever account the rotation policy selects, given the
// exhaustion markers pollAll has just refreshed.
func (c *MonitorCommand) reconcile(
	store *entities.Store,
	current *entities.Account,
	now time.Time,
) {
	exhausted := store.Rotation.IsExhausted(current.Email, now)

	target, ok := c.selectTarget(store, current, now, exhausted)
	if !ok {
		if exhausted {
			logger.Warnf("[ccswitch] %s exhausted and no account has capacity; recovers %s",
				current.Email, formatReset(store.Rotation.ExhaustedUntil[current.Email]))
		}
		return
	}
	c.switchTo(store, current, &target, switchReason(current.LastUsage, exhausted))
}

// switchReason renders the human-readable cause of a switch.
func switchReason(usage *entities.Usage, exhausted bool) string {
	if !exhausted || usage == nil {
		return "higher-priority account recovered"
	}
	binding, _ := usage.BindingLimit()
	return fmt.Sprintf("%s at %.0f%%", binding.Kind, binding.Percent)
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
	reason string,
) {
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

// installPending completes a switch that an earlier tick could only record.
//
// When a session is running switchTo moves the current pointer but leaves the
// credentials alone, and nothing retried the write: the swap waited on the shell
// wrapper calling `ensure`, and never happened at all for anyone not using it.
// Retrying here means the deferred switch lands as soon as the last session
// exits.
func (c *MonitorCommand) installPending(store *entities.Store) {
	account := store.FindAccount(store.Rotation.CurrentEmail)
	if account == nil {
		return
	}
	onDisk, identity, err := c.credentials.Read()
	if err != nil || onDisk == nil {
		return
	}
	if store.MatchAccount(*onDisk, identity) == account {
		return
	}
	// Without a usable identity, credentials whose refresh token has rotated match
	// nothing, and installing on that guess would overwrite the pair Claude Code
	// just refreshed with a stale one.
	if !identityKnown(identity) {
		return
	}
	if c.sessions != nil && c.sessions.ClaudeRunning() {
		return
	}

	if err = c.credentials.Write(&account.Credentials, &account.Identity); err != nil {
		logger.Warnf("[ccswitch] failed to install pending switch to %s: %v", account.Email, err)
		return
	}
	logger.Infof("[ccswitch] installed pending switch to %s", account.Email)
}

// syncActiveCredentials captures the on-disk credentials, which Claude Code may
// have refreshed, back into the matching stored account so backup tokens stay
// current. The account is resolved by identity rather than by refresh token,
// because the refresh token rotates on every refresh (see Store.MatchAccount).
func (c *MonitorCommand) syncActiveCredentials(store *entities.Store) {
	onDisk, identity, err := c.credentials.Read()
	if err != nil || onDisk == nil {
		return
	}
	// Claude Code blanks claudeAiOauth when a refresh comes back invalid_grant,
	// leaving empty tokens on disk rather than removing the block. Capturing that
	// would overwrite the account's last good credentials with the marker saying
	// they are gone, and flip it to LongLived so it is never polled again.
	if onDisk.Blank() {
		logger.Debugf("[ccswitch] installed credentials are blank; leaving the store untouched")
		return
	}

	account := store.MatchAccount(*onDisk, identity)
	if account == nil && !identityKnown(identity) {
		// With no identity, credentials whose refresh token has already rotated
		// match nothing, and skipping the capture would leave the store pinned to
		// the rotated-away token — the 401 loop this sync exists to prevent. The
		// monitor installs the current account and Claude Code only refreshes it
		// in place, so attribute them to it.
		account = store.FindAccount(store.Rotation.CurrentEmail)
		if account != nil {
			logger.Debugf("[ccswitch] attributed installed credentials to %s (no identity available)",
				account.Email)
		}
	}
	if account == nil {
		return
	}

	account.Credentials = *onDisk
	// Credentials carrying a refresh token come from an interactive login and can
	// be polled again, so an account previously enrolled from a long-lived token
	// recovers its monitoring as soon as it is logged in normally.
	account.LongLived = onDisk.RefreshToken == ""
	if identity != nil && identity.EmailAddress != "" {
		account.Identity = *identity
	}
}
