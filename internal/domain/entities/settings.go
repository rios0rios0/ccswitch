package entities

// MaxThreshold is the highest utilization percentage a threshold may be set to.
// At 100 an account is only ever considered exhausted once a limit actually
// reaches its ceiling or the server flags it critical.
const MaxThreshold = 100.0

// Settings holds the runtime settings that live in the store rather than on the
// command line, so that changing one takes effect without restarting the daemon:
// the monitor reloads the store on every tick and reads them there.
type Settings struct {
	// Threshold overrides the built-in rotation threshold. A nil pointer means
	// "never set", which is what keeps an unset value distinguishable from a
	// deliberate 0.
	Threshold *float64 `json:"threshold,omitempty"`
}

// ThresholdOr returns the persisted threshold, or the given fallback when none
// has been set.
func (s *Settings) ThresholdOr(fallback float64) float64 {
	if s.Threshold == nil {
		return fallback
	}
	return *s.Threshold
}

// SetThreshold records a persisted rotation threshold.
func (s *Settings) SetThreshold(threshold float64) {
	s.Threshold = &threshold
}
