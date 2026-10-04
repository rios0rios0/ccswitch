package entities_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

const (
	firstEmail  = "first@example.com"
	secondEmail = "second@example.com"
	thirdEmail  = "third@example.com"
	enrolled    = 3
)

// emailsInOrder returns the store's accounts in rotation order, which is what a
// test of the ordering operations asserts on.
func emailsInOrder(store *entities.Store) []string {
	ordered := store.Ordered()
	emails := make([]string, 0, len(ordered))
	for i := range ordered {
		emails = append(emails, ordered[i].Email)
	}
	return emails
}

// orderValues returns each account's stored order in rotation order, to show the
// order was renumbered rather than merely re-sorted.
func orderValues(store *entities.Store) []int {
	ordered := store.Ordered()
	values := make([]int, 0, len(ordered))
	for i := range ordered {
		values = append(values, ordered[i].Order)
	}
	return values
}

// mustParsePriority parses a priority the test knows to be valid.
func mustParsePriority(t *testing.T, spec string) entities.Priority {
	t.Helper()
	priority, err := entities.ParsePriority(spec)
	require.NoError(t, err)
	return priority
}

func TestParsePriority(t *testing.T) {
	t.Parallel()

	t.Run("should accept a keyword regardless of case and surrounding space", func(t *testing.T) {
		t.Parallel()
		// given
		spec := "  Top "

		// when
		priority, err := entities.ParsePriority(spec)

		// then
		require.NoError(t, err)
		position, err := priority.Position(enrolled, enrolled)
		require.NoError(t, err)
		assert.Equal(t, 1, position)
	})

	t.Run("should reject a position that is not counted from 1", func(t *testing.T) {
		t.Parallel()
		// given
		specs := []string{"0", "-1", "first", ""}

		for _, spec := range specs {
			// when
			_, err := entities.ParsePriority(spec)

			// then
			require.ErrorIs(t, err, entities.ErrInvalidPriority, "spec %q", spec)
		}
	})
}

func TestPriorityPosition(t *testing.T) {
	t.Parallel()

	t.Run("should name the primary's place for top", func(t *testing.T) {
		t.Parallel()
		// given
		priority := mustParsePriority(t, entities.PriorityTop)

		// when
		position, err := priority.Position(enrolled, enrolled)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, position)
	})

	t.Run("should name the last place for bottom", func(t *testing.T) {
		t.Parallel()
		// given
		priority := mustParsePriority(t, entities.PriorityBottom)

		// when
		position, err := priority.Position(1, enrolled)

		// then
		require.NoError(t, err)
		assert.Equal(t, enrolled, position)
	})

	t.Run("should move one place up and one place down", func(t *testing.T) {
		t.Parallel()
		// given
		up := mustParsePriority(t, entities.PriorityUp)
		down := mustParsePriority(t, entities.PriorityDown)

		// when
		raised, upErr := up.Position(enrolled, enrolled)
		lowered, downErr := down.Position(1, enrolled)

		// then
		require.NoError(t, upErr)
		require.NoError(t, downErr)
		assert.Equal(t, enrolled-1, raised)
		assert.Equal(t, 2, lowered)
	})

	t.Run("should stop at either end instead of wrapping around", func(t *testing.T) {
		t.Parallel()
		// given
		up := mustParsePriority(t, entities.PriorityUp)
		down := mustParsePriority(t, entities.PriorityDown)

		// when
		primary, upErr := up.Position(1, enrolled)
		last, downErr := down.Position(enrolled, enrolled)

		// then
		require.NoError(t, upErr)
		require.NoError(t, downErr)
		assert.Equal(t, 1, primary, "moving the primary up leaves it where it is")
		assert.Equal(t, enrolled, last, "moving the last account down leaves it where it is")
	})

	t.Run("should name a numbered position as given", func(t *testing.T) {
		t.Parallel()
		// given
		priority := mustParsePriority(t, "2")

		// when
		position, err := priority.Position(enrolled, enrolled)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, position)
	})

	t.Run("should reject a position past the last account", func(t *testing.T) {
		t.Parallel()
		// given
		priority := mustParsePriority(t, "4")

		// when
		_, err := priority.Position(1, enrolled)

		// then
		require.ErrorIs(t, err, entities.ErrInvalidPriority)
	})

	t.Run("should reject the zero priority, which names no place", func(t *testing.T) {
		t.Parallel()
		// given
		var priority entities.Priority

		// when
		_, err := priority.Position(1, enrolled)

		// then
		require.ErrorIs(t, err, entities.ErrInvalidPriority)
	})
}

func TestStorePosition(t *testing.T) {
	t.Parallel()

	t.Run("should count from 1 for the primary", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		primary := store.Position(firstEmail)
		last := store.Position(thirdEmail)

		// then
		assert.Equal(t, 1, primary)
		assert.Equal(t, enrolled, last)
	})

	t.Run("should report 0 for an account that is not enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		position := store.Position("nobody@example.com")

		// then
		assert.Zero(t, position)
	})
}

func TestStoreMove(t *testing.T) {
	t.Parallel()

	t.Run("should promote an account to primary and shift the others down", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		err := store.Move(thirdEmail, 1)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{thirdEmail, firstEmail, secondEmail}, emailsInOrder(store))
		assert.Equal(t, []int{0, 1, 2}, orderValues(store))
	})

	t.Run("should demote the primary to the last place", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		err := store.Move(firstEmail, enrolled)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{secondEmail, thirdEmail, firstEmail}, emailsInOrder(store))
	})

	t.Run("should close the gaps and ties a hand-edited order leaves", func(t *testing.T) {
		t.Parallel()
		// given: two accounts tied on 4 and a gap up to 9, as a hand edit can leave
		store := threeAccountStore()
		store.Accounts[0].Order = 4
		store.Accounts[1].Order = 4
		store.Accounts[2].Order = 9

		// when
		err := store.Move(thirdEmail, 2)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{firstEmail, thirdEmail, secondEmail}, emailsInOrder(store))
		assert.Equal(t, []int{0, 1, 2}, orderValues(store))
	})

	t.Run("should keep a pointer into the store naming the same account", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()
		held := store.FindAccount(firstEmail)

		// when
		err := store.Move(firstEmail, enrolled)

		// then
		require.NoError(t, err)
		assert.Equal(t, firstEmail, held.Email)
		assert.Equal(t, enrolled-1, held.Order)
	})

	t.Run("should reject an account that is not enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		err := store.Move("nobody@example.com", 1)

		// then
		require.ErrorIs(t, err, entities.ErrAccountNotEnrolled)
	})

	t.Run("should reject a position outside the order and leave it untouched", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		pastLast := store.Move(firstEmail, enrolled+1)
		beforeFirst := store.Move(firstEmail, 0)

		// then
		require.ErrorIs(t, pastLast, entities.ErrInvalidPriority)
		require.ErrorIs(t, beforeFirst, entities.ErrInvalidPriority)
		assert.Equal(t, []string{firstEmail, secondEmail, thirdEmail}, emailsInOrder(store))
	})
}

func TestStoreReorder(t *testing.T) {
	t.Parallel()

	t.Run("should put the named accounts first and keep the rest in their order", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		err := store.Reorder([]string{thirdEmail})

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{thirdEmail, firstEmail, secondEmail}, emailsInOrder(store))
		assert.Equal(t, []int{0, 1, 2}, orderValues(store))
	})

	t.Run("should follow the order given when every account is named", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		err := store.Reorder([]string{secondEmail, thirdEmail, firstEmail})

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{secondEmail, thirdEmail, firstEmail}, emailsInOrder(store))
	})

	t.Run("should reject an account that is not enrolled and leave the order untouched", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		err := store.Reorder([]string{thirdEmail, "nobody@example.com"})

		// then
		require.ErrorIs(t, err, entities.ErrAccountNotEnrolled)
		assert.Equal(t, []string{firstEmail, secondEmail, thirdEmail}, emailsInOrder(store))
	})

	t.Run("should reject an account named twice", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		err := store.Reorder([]string{thirdEmail, thirdEmail})

		// then
		require.Error(t, err)
		assert.Equal(t, []string{firstEmail, secondEmail, thirdEmail}, emailsInOrder(store))
	})

	t.Run("should reject an empty list", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		err := store.Reorder(nil)

		// then
		require.Error(t, err)
	})
}

func TestStoreRemove(t *testing.T) {
	t.Parallel()

	t.Run("should drop the account and its exhaustion marker and close the gap", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := threeAccountStore()
		store.Rotation.MarkExhausted(secondEmail, now.Add(time.Hour))

		// when
		removed := store.Remove(secondEmail)

		// then
		assert.True(t, removed)
		assert.Nil(t, store.FindAccount(secondEmail))
		assert.False(t, store.Rotation.IsExhausted(secondEmail, now))
		assert.Equal(t, []string{firstEmail, thirdEmail}, emailsInOrder(store))
		assert.Equal(t, []int{0, 1}, orderValues(store))
	})

	t.Run("should report false for an account that is not enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		store := threeAccountStore()

		// when
		removed := store.Remove("nobody@example.com")

		// then
		assert.False(t, removed)
		assert.Len(t, store.Accounts, enrolled)
	})
}

func TestStoreSuccessor(t *testing.T) {
	t.Parallel()

	t.Run("should pick the highest-priority account with capacity", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := threeAccountStore()
		store.Rotation.MarkExhausted(firstEmail, now.Add(time.Hour))

		// when
		successor, ok := store.Successor(now)

		// then
		assert.True(t, ok)
		assert.Equal(t, secondEmail, successor.Email)
	})

	t.Run("should fall back to the highest-priority pollable account when all are exhausted", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		store := threeAccountStore()
		for _, email := range []string{firstEmail, secondEmail, thirdEmail} {
			store.Rotation.MarkExhausted(email, now.Add(time.Hour))
		}

		// when
		successor, ok := store.Successor(now)

		// then
		assert.True(t, ok)
		assert.Equal(t, firstEmail, successor.Email)
	})

	t.Run("should never pick an account enrolled from a long-lived token", func(t *testing.T) {
		t.Parallel()
		// given
		store := &entities.Store{Accounts: []entities.Account{
			{Email: firstEmail, Order: 0, LongLived: true},
		}}

		// when
		_, ok := store.Successor(time.Now())

		// then
		assert.False(t, ok, "a long-lived account is only ever selected by hand")
	})
}
