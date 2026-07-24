package entities_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// threeAccountStore returns three interactively enrolled accounts. Each carries a
// refresh token, as real credentials from a `/login` do, which is what marks an
// account as pollable.
func threeAccountStore() *entities.Store {
	return &entities.Store{
		Accounts: []entities.Account{
			{Email: "first@example.com", Order: 0, Credentials: interactiveCreds("r1")},
			{Email: "second@example.com", Order: 1, Credentials: interactiveCreds("r2")},
			{Email: "third@example.com", Order: 2, Credentials: interactiveCreds("r3")},
		},
		Rotation: entities.RotationState{CurrentEmail: "first@example.com"},
	}
}

// interactiveCreds returns credentials shaped like those from an interactive
// login, which always carry a refresh token.
func interactiveCreds(refresh string) entities.OAuthCredentials {
	return entities.OAuthCredentials{AccessToken: "access", RefreshToken: refresh}
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

func TestStorePreferredAccount(t *testing.T) {
	t.Parallel()

	t.Run("should return the primary when it has capacity", func(t *testing.T) {
		t.Parallel()
		// given: sitting on the last account while the primary is healthy
		store := threeAccountStore()
		store.Rotation.CurrentEmail = "third@example.com"

		// when
		preferred, ok := store.PreferredAccount(time.Now())

		// then
		assert.True(t, ok)
		assert.Equal(t, "first@example.com", preferred.Email)
	})

	t.Run("should skip exhausted accounts and return the next highest priority", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := threeAccountStore()
		store.Rotation.MarkExhausted("first@example.com", now.Add(time.Hour))

		// when
		preferred, ok := store.PreferredAccount(now)

		// then
		assert.True(t, ok)
		assert.Equal(t, "second@example.com", preferred.Email)
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
		_, ok := store.PreferredAccount(now)

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

func TestStoreMatchAccount(t *testing.T) {
	t.Parallel()

	t.Run("should match by identity email when the refresh token has been rotated", func(t *testing.T) {
		t.Parallel()
		// given: the store holds the pre-rotation refresh token, as happens once
		// Claude Code refreshes the credentials on disk
		store := &entities.Store{Accounts: []entities.Account{{
			Email:       "first@example.com",
			Credentials: entities.OAuthCredentials{AccessToken: "old", RefreshToken: "stale"},
		}}}
		onDisk := entities.OAuthCredentials{AccessToken: "new", RefreshToken: "rotated"}

		// when
		matched := store.MatchAccount(onDisk, &entities.AccountIdentity{EmailAddress: "first@example.com"})

		// then
		require.NotNil(t, matched)
		assert.Equal(t, "first@example.com", matched.Email)
	})

	t.Run("should match by account uuid before email", func(t *testing.T) {
		t.Parallel()
		// given
		store := &entities.Store{Accounts: []entities.Account{
			{Email: "first@example.com", AccountUUID: "uuid-1"},
			{Email: "second@example.com", AccountUUID: "uuid-2"},
		}}

		// when
		matched := store.MatchAccount(entities.OAuthCredentials{}, &entities.AccountIdentity{
			EmailAddress: "first@example.com",
			AccountUUID:  "uuid-2",
		})

		// then
		require.NotNil(t, matched)
		assert.Equal(t, "second@example.com", matched.Email)
	})

	t.Run("should fall back to the refresh token when no identity is available", func(t *testing.T) {
		t.Parallel()
		// given
		store := &entities.Store{Accounts: []entities.Account{{
			Email:       "first@example.com",
			Credentials: entities.OAuthCredentials{AccessToken: "a", RefreshToken: "shared"},
		}}}

		// when
		matched := store.MatchAccount(entities.OAuthCredentials{RefreshToken: "shared"}, nil)

		// then
		require.NotNil(t, matched)
		assert.Equal(t, "first@example.com", matched.Email)
	})

	t.Run("should report no match when neither identity nor refresh token matches", func(t *testing.T) {
		t.Parallel()
		// given
		store := &entities.Store{Accounts: []entities.Account{{
			Email:       "first@example.com",
			Credentials: entities.OAuthCredentials{RefreshToken: "ra"},
		}}}

		// when
		matched := store.MatchAccount(entities.OAuthCredentials{RefreshToken: "other"},
			&entities.AccountIdentity{EmailAddress: "nobody@example.com"})

		// then
		assert.Nil(t, matched)
	})
}

func TestAccountSupportsUsagePolling(t *testing.T) {
	t.Parallel()

	t.Run("should be true for interactively enrolled credentials", func(t *testing.T) {
		t.Parallel()
		// given
		account := entities.Account{Credentials: interactiveCreds("r1")}

		// when & then
		assert.True(t, account.SupportsUsagePolling())
	})

	t.Run("should be false when the account is flagged long-lived", func(t *testing.T) {
		t.Parallel()
		// given
		account := entities.Account{Credentials: interactiveCreds("r1"), LongLived: true}

		// when & then
		assert.False(t, account.SupportsUsagePolling())
	})

	t.Run("should be false without a refresh token even when the flag is unset", func(t *testing.T) {
		t.Parallel()
		// given: an account enrolled before the LongLived flag existed
		account := entities.Account{Credentials: entities.OAuthCredentials{AccessToken: "long"}}

		// when & then
		assert.False(t, account.SupportsUsagePolling(),
			"legacy long-lived accounts must classify correctly without a store migration")
	})
}

func TestStoreSelectionSkipsLongLivedAccounts(t *testing.T) {
	t.Parallel()

	t.Run("should not prefer an account whose usage cannot be polled", func(t *testing.T) {
		t.Parallel()
		// given: the primary is exhausted and the only other account is long-lived
		now := time.Now()
		store := &entities.Store{
			Accounts: []entities.Account{
				{Email: "first@example.com", Order: 0, Credentials: interactiveCreds("r1")},
				{Email: "second@example.com", Order: 1, LongLived: true},
			},
			Rotation: entities.RotationState{CurrentEmail: "first@example.com"},
		}
		store.Rotation.MarkExhausted("first@example.com", now.Add(time.Hour))

		// when
		_, ok := store.PreferredAccount(now)

		// then
		assert.False(t, ok, "a long-lived account must never be selected automatically")
	})

	t.Run("should not rotate to an account whose usage cannot be polled", func(t *testing.T) {
		t.Parallel()
		// given
		store := &entities.Store{
			Accounts: []entities.Account{
				{Email: "first@example.com", Order: 0, Credentials: interactiveCreds("r1")},
				{Email: "second@example.com", Order: 1, LongLived: true},
			},
			Rotation: entities.RotationState{CurrentEmail: "first@example.com"},
		}

		// when
		next, ok := store.NextHealthyAccount(time.Now())

		// then
		assert.True(t, ok)
		assert.Equal(t, "first@example.com", next.Email, "it wraps back rather than picking the long-lived one")
	})
}
