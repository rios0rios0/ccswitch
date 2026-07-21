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

// Exhausted reports whether any active limit has reached the given percent
// threshold or has been flagged critical by the server.
func (u Usage) Exhausted(thresholdPercent float64) bool {
	for _, limit := range u.Limits {
		if !limit.IsActive {
			continue
		}
		if limit.Severity == SeverityCritical || limit.Percent >= thresholdPercent {
			return true
		}
	}
	return false
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
		if !limit.IsActive {
			continue
		}
		if limit.Severity != SeverityCritical && limit.Percent < thresholdPercent {
			continue
		}
		if limit.ResetsAt.After(latest) {
			latest = limit.ResetsAt
		}
	}
	return latest
}
