package entities

import "time"

// Capacity is whether ccswitch has an account to run Claude Code on at a given
// moment and, when it has none, which enrolled account recovers first and when.
type Capacity struct {
	// Available is false only when every account the rotation can select is
	// exhausted. It is true whenever ccswitch cannot tell, because an answer of
	// "none left" is acted on -- the shell integration points at another model --
	// and must not be given on a guess.
	Available bool
	// Next is the account whose exhaustion ends soonest. It is set only when
	// Available is false.
	Next Account
	// Recovers is when Next has capacity again.
	Recovers time.Time
}

// Capacity reports, from the exhaustion markers alone, whether an account has
// capacity at the given time. It is the same question the monitor asks before it
// gives up with "no account has capacity", answered without polling: the markers
// are what the monitor's last polls saw.
//
// Capacity is available when the rotation policy has an account to select, and
// also when ccswitch cannot tell: nothing is enrolled, no enrolled account can be
// polled, or the current account is a long-lived one picked by hand with `use`,
// whose usage cannot be read. Only when every pollable account is exhausted is it
// unavailable, with the account that recovers first named in the result.
func (s *Store) Capacity(now time.Time) Capacity {
	if current := s.FindAccount(s.Rotation.CurrentEmail); current != nil && !current.SupportsUsagePolling() {
		return Capacity{Available: true}
	}
	if _, ok := s.PreferredAccount(now); ok {
		return Capacity{Available: true}
	}

	capacity := Capacity{Available: true}
	ordered := s.Ordered()
	for i := range ordered {
		if !ordered[i].SupportsUsagePolling() {
			continue
		}
		recovers := s.Rotation.ExhaustedUntil[ordered[i].Email]
		if capacity.Available || recovers.Before(capacity.Recovers) {
			capacity = Capacity{Next: ordered[i], Recovers: recovers}
		}
	}
	return capacity
}
