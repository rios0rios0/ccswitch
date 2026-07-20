package entities

import "time"

// RotationState records which account is currently active and when exhausted
// accounts are expected to become usable again.
type RotationState struct {
	CurrentEmail   string               `json:"currentEmail"`
	ExhaustedUntil map[string]time.Time `json:"exhaustedUntil,omitempty"`
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
