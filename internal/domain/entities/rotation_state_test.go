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

func TestRotationStatePin(t *testing.T) {
	t.Parallel()

	t.Run("should make the account current and pinned when a person pins it", func(t *testing.T) {
		t.Parallel()
		// given
		state := entities.RotationState{CurrentEmail: "a@example.com"}

		// when
		state.Pin("b@example.com")

		// then
		assert.Equal(t, "b@example.com", state.CurrentEmail)
		assert.Equal(t, "b@example.com", state.PinnedEmail)
		assert.True(t, state.Pinned())
	})

	t.Run("should release the pin and keep the current account when unpinned", func(t *testing.T) {
		t.Parallel()
		// given
		state := entities.RotationState{}
		state.Pin("b@example.com")

		// when
		state.Unpin()

		// then
		assert.Equal(t, "b@example.com", state.CurrentEmail)
		assert.Empty(t, state.PinnedEmail)
		assert.False(t, state.Pinned())
	})

	t.Run("should release the pin when the rotation policy selects an account", func(t *testing.T) {
		t.Parallel()
		// given
		state := entities.RotationState{}
		state.Pin("b@example.com")

		// when
		state.Select("a@example.com")

		// then: the pin must not follow the policy's choice, nor survive to come back
		// into force should the policy return to the pinned account later
		assert.Equal(t, "a@example.com", state.CurrentEmail)
		assert.Empty(t, state.PinnedEmail)
		assert.False(t, state.Pinned())
	})

	t.Run("should leave no account current when the rotation policy selects none", func(t *testing.T) {
		t.Parallel()
		// given
		state := entities.RotationState{}
		state.Pin("b@example.com")

		// when
		state.Select("")

		// then
		assert.Empty(t, state.CurrentEmail)
		assert.False(t, state.Pinned())
	})

	t.Run("should not count a pin on an account other than the current one", func(t *testing.T) {
		t.Parallel()
		// given: a pin left behind on an account that is no longer current
		state := entities.RotationState{CurrentEmail: "a@example.com", PinnedEmail: "b@example.com"}

		// when
		pinned := state.Pinned()

		// then
		assert.False(t, pinned)
	})

	t.Run("should pin nothing in a store written before pins existed", func(t *testing.T) {
		t.Parallel()
		// given
		state := entities.RotationState{CurrentEmail: "a@example.com"}

		// when
		pinned := state.Pinned()

		// then
		assert.False(t, pinned)
	})
}
