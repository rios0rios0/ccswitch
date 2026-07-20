package entities_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

func TestRotationStateIsExhausted(t *testing.T) {
	t.Parallel()

	t.Run("should report exhausted before the reset time", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		state := entities.RotationState{ExhaustedUntil: map[string]time.Time{"a@example.com": now.Add(time.Hour)}}

		// when
		result := state.IsExhausted("a@example.com", now)

		// then
		assert.True(t, result)
	})

	t.Run("should report not exhausted after the reset time", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		state := entities.RotationState{ExhaustedUntil: map[string]time.Time{"a@example.com": now.Add(-time.Hour)}}

		// when
		result := state.IsExhausted("a@example.com", now)

		// then
		assert.False(t, result)
	})

	t.Run("should report not exhausted for an unknown account", func(t *testing.T) {
		t.Parallel()
		// given
		state := entities.RotationState{}

		// when
		result := state.IsExhausted("unknown@example.com", time.Now())

		// then
		assert.False(t, result)
	})
}

func TestRotationStateMarkAndClear(t *testing.T) {
	t.Parallel()

	t.Run("should mark an account exhausted until the given time", func(t *testing.T) {
		t.Parallel()
		// given
		state := entities.RotationState{}
		until := time.Now().Add(time.Hour)

		// when
		state.MarkExhausted("a@example.com", until)

		// then
		assert.True(t, state.IsExhausted("a@example.com", time.Now()))
	})

	t.Run("should clear markers whose reset time has passed", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		state := entities.RotationState{ExhaustedUntil: map[string]time.Time{
			"expired@example.com": now.Add(-time.Hour),
			"active@example.com":  now.Add(time.Hour),
		}}

		// when
		state.ClearExpired(now)

		// then
		assert.NotContains(t, state.ExhaustedUntil, "expired@example.com")
		assert.Contains(t, state.ExhaustedUntil, "active@example.com")
	})
}
