package entities_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

const (
	thresholdPct = 90.0
	sessionPct   = 56.0
	weeklyPct    = 93.0
	scopedPct    = 100.0
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

func TestUsageEarliestReset(t *testing.T) {
	t.Parallel()
	// given
	early := time.Now().Truncate(time.Second)
	late := early.Add(time.Hour)
	usage := entities.Usage{Limits: []entities.Limit{
		{IsActive: true, ResetsAt: late},
		{IsActive: true, ResetsAt: early},
		{IsActive: false, ResetsAt: early.Add(-time.Hour)},
	}}

	// when
	reset := usage.EarliestReset()

	// then
	assert.Equal(t, early, reset)
}
