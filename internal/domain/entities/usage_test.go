package entities_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

const (
	thresholdPct    = 90.0
	sessionPct      = 56.0
	weeklyPct       = 93.0
	scopedPct       = 100.0
	longWeeklyReset = 72 * time.Hour
)

func TestUsageExhausted(t *testing.T) {
	t.Parallel()

	t.Run("should report exhausted when an active limit reaches the threshold", func(t *testing.T) {
		t.Parallel()
		// given
		usage := entities.Usage{Limits: []entities.Limit{
			{Kind: "session", Percent: sessionPct, Severity: entities.SeverityNormal, IsActive: true},
			{Kind: "weekly_scoped", Percent: scopedPct, Severity: entities.SeverityCritical, IsActive: true},
		}}

		// when
		result := usage.Exhausted(thresholdPct)

		// then
		assert.True(t, result)
	})

	t.Run("should ignore inactive limits even when critical", func(t *testing.T) {
		t.Parallel()
		// given
		usage := entities.Usage{Limits: []entities.Limit{
			{Kind: "weekly_all", Percent: scopedPct, Severity: entities.SeverityCritical, IsActive: false},
		}}

		// when
		result := usage.Exhausted(thresholdPct)

		// then
		assert.False(t, result)
	})

	t.Run("should report not exhausted when active usage is below threshold", func(t *testing.T) {
		t.Parallel()
		// given
		usage := entities.Usage{Limits: []entities.Limit{
			{Kind: "session", Percent: sessionPct, Severity: entities.SeverityNormal, IsActive: true},
		}}

		// when
		result := usage.Exhausted(thresholdPct)

		// then
		assert.False(t, result)
	})
}

func TestUsageBindingLimit(t *testing.T) {
	t.Parallel()

	t.Run("should return the highest-utilization active limit", func(t *testing.T) {
		t.Parallel()
		// given
		usage := entities.Usage{Limits: []entities.Limit{
			{Kind: "session", Percent: sessionPct, IsActive: true},
			{Kind: "weekly", Percent: weeklyPct, IsActive: true},
			{Kind: "inactive", Percent: scopedPct, IsActive: false},
		}}

		// when
		limit, ok := usage.BindingLimit()

		// then
		assert.True(t, ok)
		assert.Equal(t, "weekly", limit.Kind)
	})

	t.Run("should report no binding limit when none are active", func(t *testing.T) {
		t.Parallel()
		// given
		usage := entities.Usage{Limits: []entities.Limit{{Kind: "session", IsActive: false}}}

		// when
		_, ok := usage.BindingLimit()

		// then
		assert.False(t, ok)
	})
}

func TestUsageRecoversAt(t *testing.T) {
	t.Parallel()

	t.Run("should return the latest reset among the exhausted limits", func(t *testing.T) {
		t.Parallel()
		// given: the session window resets soon, but the saturated weekly window
		// is what actually keeps the account unusable
		soon := time.Now().Truncate(time.Second)
		later := soon.Add(longWeeklyReset)
		usage := entities.Usage{Limits: []entities.Limit{
			{Kind: "session", Percent: sessionPct, Severity: entities.SeverityNormal, IsActive: true, ResetsAt: soon},
			{
				Kind:     "weekly_scoped",
				Percent:  scopedPct,
				Severity: entities.SeverityCritical,
				IsActive: true,
				ResetsAt: later,
			},
		}}

		// when
		recovers := usage.RecoversAt(thresholdPct)

		// then
		assert.Equal(t, later, recovers)
	})

	t.Run("should report the zero time when nothing is over the threshold", func(t *testing.T) {
		t.Parallel()
		// given
		usage := entities.Usage{Limits: []entities.Limit{
			{
				Kind:     "session",
				Percent:  sessionPct,
				Severity: entities.SeverityNormal,
				IsActive: true,
				ResetsAt: time.Now(),
			},
		}}

		// when
		recovers := usage.RecoversAt(thresholdPct)

		// then
		assert.True(t, recovers.IsZero())
	})
}

func TestUsageExhaustedIgnoresTheServerSeverity(t *testing.T) {
	t.Parallel()

	// criticalAt returns usage whose only active limit carries the server's early
	// warning at the given percentage. The endpoint reports `critical` from around
	// 95%, well before the account is actually spent.
	criticalAt := func(percent float64) entities.Usage {
		return entities.Usage{Limits: []entities.Limit{{
			Kind:     "weekly_all",
			Percent:  percent,
			Severity: entities.SeverityCritical,
			IsActive: true,
		}}}
	}

	t.Run("should ignore the critical warning at the maximum threshold", func(t *testing.T) {
		t.Parallel()
		// given: an account the server calls critical while 5% of the window is left
		usage := criticalAt(95)

		// when
		exhausted := usage.Exhausted(entities.MaxThreshold)

		// then: asking for 100 means running the account to the wire
		assert.False(t, exhausted)
	})

	t.Run("should ignore the critical warning below the maximum threshold too", func(t *testing.T) {
		t.Parallel()
		// given: the severity is a display band, not a ceiling -- honouring it would
		// cap every threshold at the point the warning fires, making 99 behave like 90
		usage := criticalAt(95)

		// when
		exhausted := usage.Exhausted(99)

		// then
		assert.False(t, exhausted)
	})

	t.Run("should exhaust once a limit reaches the threshold", func(t *testing.T) {
		t.Parallel()
		// given
		usage := criticalAt(99)

		// when
		exhausted := usage.Exhausted(99)

		// then
		assert.True(t, exhausted)
	})

	t.Run("should not report a recovery time for a limit it does not count as spent", func(t *testing.T) {
		t.Parallel()
		// given: RecoversAt has to agree with Exhausted, or an account is held with a
		// recovery time of zero and falls back to a blind cooldown
		usage := criticalAt(95)

		// when
		recovers := usage.RecoversAt(entities.MaxThreshold)

		// then
		assert.True(t, recovers.IsZero())
	})
}
