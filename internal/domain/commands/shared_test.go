package commands_test

import (
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

const (
	thresholdPct = 90.0
	fullPct      = 100.0
	lowPct       = 20.0
	sessionPct   = 56.0
	weeklyPct    = 93.0
	// nearingPct sits past 80% of thresholdPct but short of it, where a figure is
	// shown as closing in on the threshold.
	nearingPct = 75.0
	// overflowPct is a figure past the limit it measures.
	overflowPct = 120.0
	// justUnderThresholdPct rounds to thresholdPct without reaching it.
	justUnderThresholdPct = 89.6
	// almostFullPct rounds to a full 100% without being at the limit.
	almostFullPct = 99.7
	longRecovery  = 72 * time.Hour
	// The reset offsets below sit well clear of the unit boundary below them, so
	// the countdown a test expects holds however long the test takes to get there.
	sessionResetIn    = 2*time.Hour + 13*time.Minute + 30*time.Second
	weeklyResetIn     = 4*24*time.Hour + 2*time.Hour + 30*time.Minute
	recoveryIn        = 31*time.Minute + 30*time.Second
	wholeHoursResetIn = 4*time.Hour + 30*time.Second
	imminentResetIn   = 40 * time.Second
	// staleFor is how long ago a reading's limit reset, for a reading kept from
	// before that reset.
	staleFor = 12*time.Minute + 30*time.Second
	// farFuture is an expiry no test run will reach, so a token carrying it is
	// refreshed only when something other than its age forces it.
	farFuture = int64(1) << 62
)

// loginScopes are the scopes an interactive Claude Code login carries. Real
// credentials always have them, and a set without them is treated as degraded
// and refreshed on sight, so fixtures have to carry them too.
func loginScopes() []string {
	return []string{"user:profile", "user:inference", "user:sessions:claude_code"}
}

// creds returns a complete credential set for the given token pair.
func creds(access, refresh string) entities.OAuthCredentials {
	return entities.OAuthCredentials{
		AccessToken:  access,
		RefreshToken: refresh,
		Scopes:       loginScopes(),
	}
}

// validCreds returns a complete credential set for the primary test account.
func validCreds() *entities.OAuthCredentials {
	set := creds("access", "refresh")
	return &set
}

// twoAccountStore returns a store with accounts "a" (current) and "b".
func twoAccountStore() *entities.Store {
	return &entities.Store{
		Accounts: []entities.Account{
			{
				Email:       "a@example.com",
				Order:       0,
				Credentials: creds("a", "ra"),
			},
			{
				Email:       "b@example.com",
				Order:       1,
				Credentials: creds("b", "rb"),
			},
		},
		Rotation: entities.RotationState{CurrentEmail: "a@example.com"},
	}
}

// livePairStore returns twoAccountStore with both access tokens well inside
// their validity. twoAccountStore leaves ExpiresAt unset, which counts as
// expired, so tests that care whether a poll refreshed need this instead.
func livePairStore() *entities.Store {
	store := twoAccountStore()
	for i := range store.Accounts {
		store.Accounts[i].Credentials.ExpiresAt = farFuture
	}
	return store
}

// threeAccountStore returns livePairStore with a third live account "c" at the
// end of the rotation order.
func threeAccountStore() *entities.Store {
	store := livePairStore()
	third := creds("c", "rc")
	third.ExpiresAt = farFuture
	store.Accounts = append(store.Accounts, entities.Account{
		Email:       "c@example.com",
		Order:       2,
		Credentials: third,
	})
	return store
}

// emailsInOrder returns the store's accounts in rotation order.
func emailsInOrder(store *entities.Store) []string {
	ordered := store.Ordered()
	emails := make([]string, 0, len(ordered))
	for i := range ordered {
		emails = append(emails, ordered[i].Email)
	}
	return emails
}

// momentIn renders a moment the way the commands print it, so a test does not
// depend on the time zone it runs in.
func momentIn(moment time.Time) string {
	return moment.Local().Format("Mon Jan 2 15:04")
}

// expiredPrimaryStore returns a store in which only the primary's access token is
// spent. A tick refreshes every account whose token has expired, so a test that
// counts refreshes has to leave exactly one account refreshable.
func expiredPrimaryStore() *entities.Store {
	store := livePairStore()
	store.Accounts[0].Credentials.ExpiresAt = 0
	return store
}

// longLivedOnlyStore returns a store holding a single account enrolled from a
// long-lived token, which carries no refresh token.
func longLivedOnlyStore() *entities.Store {
	return &entities.Store{
		Accounts: []entities.Account{{
			Email:       "long@example.com",
			Order:       0,
			Credentials: entities.OAuthCredentials{AccessToken: "long"},
			LongLived:   true,
		}},
		Rotation: entities.RotationState{CurrentEmail: "long@example.com"},
	}
}

// longLivedFallbackStore returns a store whose primary is a normal pollable
// account and whose backup was enrolled from a long-lived token, with the
// long-lived one currently selected.
func longLivedFallbackStore() *entities.Store {
	return &entities.Store{
		Accounts: []entities.Account{
			{
				Email:       "a@example.com",
				Order:       0,
				Credentials: creds("a", "ra"),
			},
			{
				Email:       "long@example.com",
				Order:       1,
				Credentials: entities.OAuthCredentials{AccessToken: "long"},
				LongLived:   true,
			},
		},
		Rotation: entities.RotationState{CurrentEmail: "long@example.com"},
	}
}

// monitorConfig returns a config using the default rotation threshold with the
// prefer-primary policy enabled.
func monitorConfig() *entities.Config {
	return &entities.Config{Threshold: thresholdPct, PreferPrimary: true}
}

// roundRobinConfig returns a config that cycles forward through the accounts
// instead of returning to the primary.
func roundRobinConfig() *entities.Config {
	return &entities.Config{Threshold: thresholdPct, PreferPrimary: false}
}

// perAccountUsage keys canned usage by access token, which is what the monitor
// needs now that a tick polls every enrolled account rather than only the active
// one: a single canned response would make every account look identical.
func perAccountUsage(byToken map[string]*entities.Usage) *doubles.StubUsageRepository {
	return &doubles.StubUsageRepository{ByToken: byToken}
}

// exhaustedPrimaryUsage returns a usage stub in which the primary "a" is spent
// and the backup "b" still has capacity.
func exhaustedPrimaryUsage() *doubles.StubUsageRepository {
	return perAccountUsage(map[string]*entities.Usage{
		"a": exhaustedUsage(),
		"b": healthyUsage(),
	})
}

// exhaustedUsage returns usage whose active scoped limit is fully consumed.
func exhaustedUsage() *entities.Usage {
	return &entities.Usage{Limits: []entities.Limit{
		{
			Kind:     "weekly_scoped",
			Percent:  fullPct,
			Severity: entities.SeverityCritical,
			IsActive: true,
			ResetsAt: time.Now().Add(time.Hour),
		},
	}}
}

// healthyUsage returns usage well below the rotation threshold.
func healthyUsage() *entities.Usage {
	return &entities.Usage{Limits: []entities.Limit{
		{Kind: "session", Percent: lowPct, Severity: entities.SeverityNormal, IsActive: true},
	}}
}
