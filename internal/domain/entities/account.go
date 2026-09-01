package entities

import (
	"sort"
	"time"
)

// Account is an enrolled Claude account together with its persisted credentials,
// identity, and last observed usage.
type Account struct {
	Email        string           `json:"email"`
	AccountUUID  string           `json:"accountUuid"`
	Order        int              `json:"order"`
	Credentials  OAuthCredentials `json:"credentials"`
	Identity     AccountIdentity  `json:"identity"`
	LastUsage    *Usage           `json:"lastUsage,omitempty"`
	LastPolledAt time.Time        `json:"lastPolledAt,omitzero"`
	// LongLived marks an account enrolled from a long-lived token (`claude
	// setup-token`) instead of an interactive login. Such tokens are minted
	// without the `user:profile` scope, so the usage endpoint rejects them with
	// 403 and their utilization cannot be read. The zero value is false, so
	// accounts enrolled before this field existed stay pollable.
	LongLived bool `json:"longLived,omitempty"`
}

// SupportsUsagePolling reports whether ccswitch can read this account's usage.
// It is false for long-lived tokens, which lack the scope the usage endpoint
// requires; such accounts are never polled and never selected automatically,
// serving only as a manual fallback via `ccswitch use`.
//
// A missing refresh token is treated the same way. Interactive credentials
// always carry one, so its absence means the account came from a long-lived
// token — which also makes accounts enrolled before LongLived existed classify
// correctly, without a store migration.
func (a Account) SupportsUsagePolling() bool {
	return !a.LongLived && a.Credentials.RefreshToken != ""
}

// Store is the persisted ccswitch state: the enrolled accounts, the rotation
// state that tracks the active account and exhaustion windows, and the settings
// that can be changed while the daemon runs.
type Store struct {
	Accounts []Account     `json:"accounts"`
	Rotation RotationState `json:"rotation"`
	Settings Settings      `json:"settings,omitzero"`
}

// FindAccount returns a pointer to the account with the given email, or nil when
// no such account is enrolled.
func (s *Store) FindAccount(email string) *Account {
	for i := range s.Accounts {
		if s.Accounts[i].Email == email {
			return &s.Accounts[i]
		}
	}
	return nil
}

// MatchAccount returns the enrolled account that the given installed credentials
// belong to, or nil when none matches.
//
// The identity is preferred over the credentials because the OAuth refresh token
// is rotated by the server on every refresh: once Claude Code refreshes the
// credentials on disk, the stored refresh token no longer equals the installed
// one. Matching on the refresh token alone would then stop recognizing the
// account, leaving the store frozen on a refresh token that has been rotated
// away — which fails every later refresh with 401. The refresh-token comparison
// is kept only as a fallback for when no identity is available.
func (s *Store) MatchAccount(creds OAuthCredentials, identity *AccountIdentity) *Account {
	if identity != nil {
		if identity.AccountUUID != "" {
			for i := range s.Accounts {
				if s.Accounts[i].AccountUUID == identity.AccountUUID {
					return &s.Accounts[i]
				}
			}
		}
		if account := s.FindAccount(identity.EmailAddress); account != nil {
			return account
		}
	}

	for i := range s.Accounts {
		if s.Accounts[i].Credentials.SameAccountAs(creds) {
			return &s.Accounts[i]
		}
	}
	return nil
}

// Ordered returns a copy of the accounts sorted by their rotation order.
func (s *Store) Ordered() []Account {
	ordered := make([]Account, len(s.Accounts))
	copy(ordered, s.Accounts)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Order < ordered[j].Order
	})
	return ordered
}

// NextOrder returns the order value to assign to a newly enrolled account so it
// sorts after every existing account.
func (s *Store) NextOrder() int {
	highest := -1
	for i := range s.Accounts {
		if s.Accounts[i].Order > highest {
			highest = s.Accounts[i].Order
		}
	}
	return highest + 1
}

// PreferredAccount returns the highest-priority account that is not exhausted at
// the given time. Priority is the rotation order, so the first enrolled account
// is the primary. Accounts whose usage cannot be polled are skipped, because
// there is no way to tell whether they still have capacity. The boolean is false
// when no such account is available.
func (s *Store) PreferredAccount(now time.Time) (Account, bool) {
	ordered := s.Ordered()
	for i := range ordered {
		if !ordered[i].SupportsUsagePolling() {
			continue
		}
		if !s.Rotation.IsExhausted(ordered[i].Email, now) {
			return ordered[i], true
		}
	}

	var none Account
	return none, false
}

// NextHealthyAccount returns the next account after the current one, in rotation
// order, that is not exhausted at the given time. The search wraps around and
// considers the current account last, so it is returned only when it is the sole
// healthy account. Accounts whose usage cannot be polled are skipped, for the
// same reason as in PreferredAccount. The boolean is false when no healthy
// account exists.
func (s *Store) NextHealthyAccount(now time.Time) (Account, bool) {
	ordered := s.Ordered()
	if len(ordered) == 0 {
		var none Account
		return none, false
	}

	start := s.currentIndex(ordered)
	for offset := 1; offset <= len(ordered); offset++ {
		candidate := ordered[(start+offset)%len(ordered)]
		if !candidate.SupportsUsagePolling() {
			continue
		}
		if !s.Rotation.IsExhausted(candidate.Email, now) {
			return candidate, true
		}
	}

	var none Account
	return none, false
}

// currentIndex returns the position of the current account within the ordered
// slice, or 0 when the current account is unknown.
func (s *Store) currentIndex(ordered []Account) int {
	for i := range ordered {
		if ordered[i].Email == s.Rotation.CurrentEmail {
			return i
		}
	}
	return 0
}
