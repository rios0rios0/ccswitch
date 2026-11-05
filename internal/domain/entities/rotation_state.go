package entities

import "time"

// RotationState records which account is currently active, whether a person pinned
// it there, and when exhausted accounts are expected to become usable again.
type RotationState struct {
	CurrentEmail string `json:"currentEmail"`
	// PinnedEmail is the account a person picked by hand, with `ccswitch use` or
	// `ccswitch rotate`. While it is the current account the rotation policy keeps it
	// active for as long as it has capacity, even when an account of higher priority
	// has more, so the choice outlives the monitor's next poll. It names the current
	// account or nothing: every switch the policy makes goes through Select, which
	// releases it. Empty, as in every store written before pins existed, it pins
	// nothing.
	PinnedEmail    string               `json:"pinnedEmail,omitempty"`
	ExhaustedUntil map[string]time.Time `json:"exhaustedUntil,omitempty"`
}

// Pin makes the account current and pins it there, which is what picking it by hand
// does: the rotation policy keeps it active until it is exhausted or the pin is
// released.
func (s *RotationState) Pin(email string) {
	s.CurrentEmail = email
	s.PinnedEmail = email
}

// Select makes the account current as the rotation policy's choice, or leaves no
// account current for an empty email. It releases any pin, since the account a person
// picked is no longer the one in use.
func (s *RotationState) Select(email string) {
	s.CurrentEmail = email
	s.PinnedEmail = ""
}

// Pinned reports whether a person pinned the current account.
func (s *RotationState) Pinned() bool {
	return s.PinnedEmail != "" && s.PinnedEmail == s.CurrentEmail
}

// Unpin releases the pin, leaving the current account to the rotation policy.
func (s *RotationState) Unpin() {
	s.PinnedEmail = ""
}

// IsExhausted reports whether the account is marked exhausted at the given time.
func (s *RotationState) IsExhausted(email string, now time.Time) bool {
	until, ok := s.ExhaustedUntil[email]
	if !ok {
		return false
	}
	return now.Before(until)
}

// MarkExhausted records that the account is exhausted until the given time.
func (s *RotationState) MarkExhausted(email string, until time.Time) {
	if s.ExhaustedUntil == nil {
		s.ExhaustedUntil = make(map[string]time.Time)
	}
	s.ExhaustedUntil[email] = until
}

// ClearExpired removes exhaustion markers whose reset time has already passed.
func (s *RotationState) ClearExpired(now time.Time) {
	for email, until := range s.ExhaustedUntil {
		if !now.Before(until) {
			delete(s.ExhaustedUntil, email)
		}
	}
}

// ClearExhausted removes the exhaustion marker for one account, which a fresh
// poll showing it back under the threshold makes stale.
//
// A marker is a cache of what the last poll saw, not a lease: it is recorded so
// that an account nobody is polling still looks exhausted, and so the reason is
// visible. Once a poll contradicts it — because the limits reset early, or
// because the threshold was raised — keeping it would strand the account.
func (s *RotationState) ClearExhausted(email string) {
	delete(s.ExhaustedUntil, email)
}
