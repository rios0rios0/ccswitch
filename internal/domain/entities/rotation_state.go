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
	PinnedEmail string `json:"pinnedEmail,omitempty"`
	// PinnedThrough is when the exhaustion the pinned account was picked over ends: the
	// zero time when it had capacity as far as ccswitch knew.
	//
	// Picking an account the monitor counts exhausted overrules that reading, and the
	// reading cannot settle the matter against the person who picked it. The usage
	// endpoint reports whole percentages and caps them at 100, so an account it reads at
	// 100% may still be served while another, reading the same 100% field for field, is
	// refused. Released on the next poll, which can only read the account as spent
	// again, such a pin lasted one tick, and the person had to pick the account again
	// every few minutes. So the pin holds through that exhaustion (see
	// PinOverridesExhaustion) until it ends, or until a poll finds the account back
	// under the threshold; from then on it is an ordinary pin, which the next exhaustion
	// releases.
	PinnedThrough  time.Time            `json:"pinnedThrough,omitzero"`
	ExhaustedUntil map[string]time.Time `json:"exhaustedUntil,omitempty"`
}

// Pin makes the account current and pins it there, which is what picking it by hand
// does: the rotation policy keeps it active until it is exhausted or the pin is
// released. An account picked while it is marked exhausted is held through that
// exhaustion as well (see PinnedThrough); a marker whose time has passed holds nothing.
func (s *RotationState) Pin(email string) {
	s.CurrentEmail = email
	s.PinnedEmail = email
	s.PinnedThrough = s.ExhaustedUntil[email]
}

// Select makes the account current as the rotation policy's choice, or leaves no
// account current for an empty email. It releases any pin, since the account a person
// picked is no longer the one in use.
func (s *RotationState) Select(email string) {
	s.CurrentEmail = email
	s.Unpin()
}

// Pinned reports whether a person pinned the current account.
func (s *RotationState) Pinned() bool {
	return s.PinnedEmail != "" && s.PinnedEmail == s.CurrentEmail
}

// PinOverridesExhaustion reports whether the pin keeps the current account active at
// the given time even while it is marked exhausted: a person picked it when it already
// was, and the exhaustion they picked it over has not ended since (see PinnedThrough).
func (s *RotationState) PinOverridesExhaustion(now time.Time) bool {
	return s.Pinned() && now.Before(s.PinnedThrough)
}

// Unpin releases the pin, leaving the current account to the rotation policy.
func (s *RotationState) Unpin() {
	s.PinnedEmail = ""
	s.PinnedThrough = time.Time{}
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
//
// A pin held through that exhaustion (see PinnedThrough) lapses with the marker: the
// account has recovered, so the exhaustion it was picked over is over, and the next
// one releases the pin as it would any other.
func (s *RotationState) ClearExhausted(email string) {
	delete(s.ExhaustedUntil, email)
	if email == s.PinnedEmail {
		s.PinnedThrough = time.Time{}
	}
}
