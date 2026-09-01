package entities

import "time"

// Severity levels reported by the Claude usage endpoint for a single limit.
const (
	SeverityNormal   = "normal"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Window describes utilization of a single rolling usage window, expressed as a
// percentage in the range 0-100.
type Window struct {
	Utilization float64   `json:"utilization"`
	ResetsAt    time.Time `json:"resetsAt"`
}

// Limit describes one usage limit as reported by the Claude usage endpoint.
type Limit struct {
	Kind     string    `json:"kind"`
	Group    string    `json:"group"`
	Percent  float64   `json:"percent"`
	Severity string    `json:"severity"`
	IsActive bool      `json:"isActive"`
	ResetsAt time.Time `json:"resetsAt"`
}

// Usage is the parsed response of the Claude usage endpoint for one account.
type Usage struct {
	FiveHour   Window  `json:"fiveHour"`
	SevenDay   Window  `json:"sevenDay"`
	Limits     []Limit `json:"limits"`
	ExtraUsage bool    `json:"extraUsage"`
}

// Exhausted reports whether any active limit is spent at the given threshold.
func (u Usage) Exhausted(thresholdPercent float64) bool {
	for _, limit := range u.Limits {
		if limit.IsActive && limitSpent(limit, thresholdPercent) {
			return true
		}
	}
	return false
}

// limitSpent reports whether one limit counts as exhausted at the given
// threshold. The utilization percentage is the sole criterion: the threshold is
// the tolerance the user set, so the number they choose is the number that
// applies.
//
// The server's `critical` severity is deliberately not a trigger of its own. It
// is a display band rather than a ceiling — it is reported from around 95% with
// `locked_reason` still null, i.e. while the account is perfectly usable — so
// honouring it independently capped every threshold at the point the warning
// fires and made 99 behave exactly like 90.
func limitSpent(limit Limit, thresholdPercent float64) bool {
	return limit.Percent >= thresholdPercent
}

// BindingLimit returns the active limit with the highest utilization and whether
// such a limit exists. It is the limit that will run out first.
func (u Usage) BindingLimit() (Limit, bool) {
	var worst Limit
	found := false
	for _, limit := range u.Limits {
		if !limit.IsActive {
			continue
		}
		if !found || limit.Percent > worst.Percent {
			worst = limit
			found = true
		}
	}
	return worst, found
}

// RecoversAt returns the time at which every limit that currently exceeds the
// threshold will have reset, i.e. the earliest moment the account has capacity
// again. It returns the zero time when nothing is over the threshold.
//
// The latest reset is used deliberately: an account is only usable again once
// all of its exhausted limits have reset. Using the soonest reset would free the
// account while a longer window (for example the weekly limit) is still
// saturated, causing it to be selected and immediately exhausted again.
func (u Usage) RecoversAt(thresholdPercent float64) time.Time {
	var latest time.Time
	for _, limit := range u.Limits {
		if !limit.IsActive || !limitSpent(limit, thresholdPercent) {
			continue
		}
		if limit.ResetsAt.After(latest) {
			latest = limit.ResetsAt
		}
	}
	return latest
}
