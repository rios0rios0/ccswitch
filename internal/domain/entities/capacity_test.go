package entities_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// exhaustAll marks every account in the store exhausted for the given durations
// from now, in rotation order.
func exhaustAll(store *entities.Store, now time.Time, durations ...time.Duration) {
	for i, account := range store.Ordered() {
		store.Rotation.MarkExhausted(account.Email, now.Add(durations[i]))
	}
}

func TestStoreCapacity(t *testing.T) {
	t.Parallel()

	t.Run("should be available when the current account is not exhausted", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		capacity := store.Capacity(time.Now())

		// then
		assert.True(t, capacity.Available)
	})

	t.Run("should be available while a lower-priority account still has capacity", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := threeAccountStore()
		store.Rotation.MarkExhausted("first@example.com", now.Add(time.Hour))
		store.Rotation.MarkExhausted("second@example.com", now.Add(time.Hour))

		// when
		capacity := store.Capacity(now)

		// then
		assert.True(t, capacity.Available)
	})

	t.Run("should name the account that recovers first when every account is exhausted", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := threeAccountStore()
		exhaustAll(store, now, 3*time.Hour, time.Hour, 2*time.Hour)

		// when
		capacity := store.Capacity(now)

		// then
		assert.False(t, capacity.Available)
		assert.Equal(t, "second@example.com", capacity.Next.Email)
		assert.Equal(t, now.Add(time.Hour), capacity.Recovers)
	})

	t.Run("should prefer the higher-priority account when two recover together", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := threeAccountStore()
		exhaustAll(store, now, 2*time.Hour, time.Hour, time.Hour)

		// when
		capacity := store.Capacity(now)

		// then
		assert.False(t, capacity.Available)
		assert.Equal(t, "second@example.com", capacity.Next.Email)
	})

	t.Run("should count an exhaustion marker that has passed as recovered", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := threeAccountStore()
		exhaustAll(store, now, time.Hour, time.Hour, -time.Minute)

		// when
		capacity := store.Capacity(now)

		// then
		assert.True(t, capacity.Available)
	})

	t.Run("should be available when nothing is enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		store := &entities.Store{}

		// when
		capacity := store.Capacity(time.Now())

		// then
		assert.True(t, capacity.Available)
	})

	t.Run("should be available when the current account is a long-lived one picked by hand", func(t *testing.T) {
		t.Parallel()
		// given: the pollable accounts are exhausted, but `use` installed a long-lived
		// one whose usage cannot be read
		now := time.Now()
		store := threeAccountStore()
		exhaustAll(store, now, time.Hour, time.Hour, time.Hour)
		store.Accounts = append(store.Accounts, entities.Account{
			Email:       "long@example.com",
			Order:       3,
			Credentials: entities.OAuthCredentials{AccessToken: "long"},
			LongLived:   true,
		})
		store.Rotation.CurrentEmail = "long@example.com"

		// when
		capacity := store.Capacity(now)

		// then
		assert.True(t, capacity.Available)
	})

	t.Run("should be available when no enrolled account can be polled", func(t *testing.T) {
		t.Parallel()
		// given
		store := &entities.Store{
			Accounts: []entities.Account{{
				Email:       "long@example.com",
				Credentials: entities.OAuthCredentials{AccessToken: "long"},
				LongLived:   true,
			}},
		}

		// when
		capacity := store.Capacity(time.Now())

		// then
		assert.True(t, capacity.Available)
	})

	t.Run("should leave a long-lived backup out of the account it names", func(t *testing.T) {
		t.Parallel()
		// given: a long-lived backup is never selected automatically, so it neither
		// supplies capacity nor recovers
		now := time.Now()
		store := threeAccountStore()
		exhaustAll(store, now, 3*time.Hour, 2*time.Hour, time.Hour)
		store.Accounts = append(store.Accounts, entities.Account{
			Email:       "long@example.com",
			Order:       3,
			Credentials: entities.OAuthCredentials{AccessToken: "long"},
			LongLived:   true,
		})

		// when
		capacity := store.Capacity(now)

		// then
		assert.False(t, capacity.Available)
		assert.Equal(t, "third@example.com", capacity.Next.Email)
	})
}
