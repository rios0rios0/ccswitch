package commands_test

import (
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

const (
	thresholdPct = 90.0
	fullPct      = 100.0
	lowPct       = 20.0
	longRecovery = 72 * time.Hour
)

// validCreds returns a complete credential set for the primary test account.
func validCreds() *entities.OAuthCredentials {
	return &entities.OAuthCredentials{AccessToken: "access", RefreshToken: "refresh"}
}

// twoAccountStore returns a store with accounts "a" (current) and "b".
func twoAccountStore() *entities.Store {
	return &entities.Store{
		Accounts: []entities.Account{
			{
				Email:       "a@example.com",
				Order:       0,
				Credentials: entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra"},
			},
			{
				Email:       "b@example.com",
				Order:       1,
				Credentials: entities.OAuthCredentials{AccessToken: "b", RefreshToken: "rb"},
			},
		},
		Rotation: entities.RotationState{CurrentEmail: "a@example.com"},
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
