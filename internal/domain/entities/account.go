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
}

// Store is the persisted ccswitch state: the enrolled accounts plus the rotation
// state that tracks the active account and exhaustion windows.
type Store struct {
	Accounts []Account     `json:"accounts"`
	Rotation RotationState `json:"rotation"`
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
// is the primary. The boolean is false when every account is exhausted.
func (s *Store) PreferredAccount(now time.Time) (Account, bool) {
	ordered := s.Ordered()
	for i := range ordered {
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
// healthy account. The boolean is false when no healthy account exists.
func (s *Store) NextHealthyAccount(now time.Time) (Account, bool) {
	ordered := s.Ordered()
	if len(ordered) == 0 {
		var none Account
		return none, false
	}

	start := s.currentIndex(ordered)
	for offset := 1; offset <= len(ordered); offset++ {
		candidate := ordered[(start+offset)%len(ordered)]
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
