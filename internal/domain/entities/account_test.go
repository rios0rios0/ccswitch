package entities_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

func threeAccountStore() *entities.Store {
	return &entities.Store{
		Accounts: []entities.Account{
			{Email: "first@example.com", Order: 0},
			{Email: "second@example.com", Order: 1},
			{Email: "third@example.com", Order: 2},
		},
		Rotation: entities.RotationState{CurrentEmail: "first@example.com"},
	}
}

func TestStoreNextHealthyAccount(t *testing.T) {
	t.Parallel()

	t.Run("should return the next account in order after the current one", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		next, ok := store.NextHealthyAccount(time.Now())

		// then
		assert.True(t, ok)
		assert.Equal(t, "second@example.com", next.Email)
	})

	t.Run("should skip exhausted accounts", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := threeAccountStore()
		store.Rotation.MarkExhausted("second@example.com", now.Add(time.Hour))

		// when
		next, ok := store.NextHealthyAccount(now)

		// then
		assert.True(t, ok)
		assert.Equal(t, "third@example.com", next.Email)
	})

	t.Run("should wrap around to the first account", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()
		store.Rotation.CurrentEmail = "third@example.com"

		// when
		next, ok := store.NextHealthyAccount(time.Now())

		// then
		assert.True(t, ok)
		assert.Equal(t, "first@example.com", next.Email)
	})

	t.Run("should report none when every account is exhausted", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := threeAccountStore()
		for _, email := range []string{"first@example.com", "second@example.com", "third@example.com"} {
			store.Rotation.MarkExhausted(email, now.Add(time.Hour))
		}

		// when
		_, ok := store.NextHealthyAccount(now)

		// then
		assert.False(t, ok)
	})
}

func TestStoreNextOrder(t *testing.T) {
	t.Parallel()
	// given
	store := threeAccountStore()

	// when
	order := store.NextOrder()

	// then
	assert.Equal(t, 3, order)
}
