package entities

import "time"

// Severity levels reported by the Claude usage endpoint for a single limit.
const (
	SeverityNormal   = "normal"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Limit kinds reported by the Claude usage endpoint. The first two are the
// rolling 5-hour and 7-day allowances that Usage.FiveHour and Usage.SevenDay also
// report; the scoped kind is a weekly limit on one model family.
const (
	LimitKindSession      = "session"
	LimitKindWeeklyAll    = "weekly_all"
	LimitKindWeeklyScoped = "weekly_scoped"
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

// Usage is the parsed response of a usage endpoint for one account.
type Usage struct {
	FiveHour   Window  `json:"fiveHour"`
	SevenDay   Window  `json:"sevenDay"`
	Limits     []Limit `json:"limits"`
	ExtraUsage bool    `json:"extraUsage"`
	// LimitsOnly reports that every utilization figure is one of Limits, and that
	// FiveHour and SevenDay mean nothing. Codex's endpoint reports usage that way: the
	// rolling windows of an account depend on its plan (a free plan has a single
	// 30-day one), so a window it did not report is not an empty one, and listing a
	// 5-hour and a 7-day window at 0% would describe limits the account does not have.
	// The zero value is the shape of Claude's endpoint, which always reports both.
	LimitsOnly bool `json:"limitsOnly,omitempty"`
	// Blocked reports that the endpoint refuses the account outright, whatever its
	// percentages say. Codex answers `allowed: false` once it stops serving an account,
	// which a workspace whose credits ran out reaches without any window being full.
	Blocked bool `json:"blocked,omitempty"`
}

// Reading is one utilization figure worth showing for an account, a rolling
// window or another limit, together with the time it resets.
type Reading struct {
	Kind     string
	Percent  float64
	ResetsAt time.Time
}

// Readings returns every utilization figure the endpoint reported: the 5-hour and
// 7-day windows first, then each other limit in the order it was reported.
//
// The windows and the `session` and `weekly_all` limits describe the same two
// allowances, so those limits are folded into the windows instead of being listed
// twice, and a window the endpoint left empty is filled from its limit. Every
// other limit (a weekly limit scoped to one model family, say) is listed on its
// own whether or not it is active. It is often the one that exhausts an account,
// and what a reader wants from it is when it resets.
//
// A usage reported as limits only (see LimitsOnly) has no fixed windows, so its
// readings are its limits, in the order they were reported.
func (u Usage) Readings() []Reading {
	if u.LimitsOnly {
		readings := make([]Reading, 0, len(u.Limits))
		for _, limit := range u.Limits {
			readings = append(readings, Reading{Kind: limit.Kind, Percent: limit.Percent, ResetsAt: limit.ResetsAt})
		}
		return readings
	}

	readings := []Reading{
		u.windowReading(LimitKindSession, u.FiveHour),
		u.windowReading(LimitKindWeeklyAll, u.SevenDay),
	}
	for _, limit := range u.Limits {
		if limit.Kind == LimitKindSession || limit.Kind == LimitKindWeeklyAll {
			continue
		}
		readings = append(readings, Reading{Kind: limit.Kind, Percent: limit.Percent, ResetsAt: limit.ResetsAt})
	}
	return readings
}

// Spent reports whether the figure has reached the given threshold, by the same
// test that decides when an active limit exhausts its account, so a figure shown
// as spent is one that would rotate the account away.
func (r Reading) Spent(thresholdPercent float64) bool {
	return percentSpent(r.Percent, thresholdPercent)
}

// windowReading reports one rolling window, falling back to the limit of the same
// kind when the endpoint did not fill the window in.
func (u Usage) windowReading(kind string, window Window) Reading {
	if window.ResetsAt.IsZero() && window.Utilization == 0 {
		for _, limit := range u.Limits {
			if limit.Kind == kind {
				return Reading{Kind: kind, Percent: limit.Percent, ResetsAt: limit.ResetsAt}
			}
		}
	}
	return Reading{Kind: kind, Percent: window.Utilization, ResetsAt: window.ResetsAt}
}

// Exhausted reports whether any active limit is spent at the given threshold, or the
// endpoint refuses the account outright (see Blocked).
//
// A refusal is not the display band the threshold must outrank (see limitSpent): it
// means requests fail, which no tolerance the user sets can make usable.
func (u Usage) Exhausted(thresholdPercent float64) bool {
	if u.Blocked {
		return true
	}
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
	return percentSpent(limit.Percent, thresholdPercent)
}

// percentSpent is the test limitSpent applies, shared with Reading.Spent.
func percentSpent(percent, thresholdPercent float64) bool {
	return percent >= thresholdPercent
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
//
// A refusal (see Blocked) says nothing of when it ends, so an account blocked with no
// limit over the threshold has no recovery time either, and the caller decides how
// long to hold it.
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
