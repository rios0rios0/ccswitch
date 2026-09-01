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
	longRecovery = 72 * time.Hour
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
