package commands_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

// useOutput runs `use` for the email over the store, and returns what it printed and
// what it warned.
func useOutput(t *testing.T, store *entities.Store, email string) (string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	err := commands.NewUseAccountCommand(
		&doubles.InMemoryAccountsRepository{Store: store}, &doubles.StubCredentialsRepository{},
	).WithOutput(&out).WithErrorOutput(&errOut).Execute(email)
	require.NoError(t, err)
	return out.String(), errOut.String()
}

func TestUseAccountCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("should switch to the named account", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{}
		command := commands.NewUseAccountCommand(accounts, credentials).WithOutput(&bytes.Buffer{})

		// when
		err := command.Execute("b@example.com")

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Equal(t, 1, credentials.WriteCalls)
	})

	t.Run("should pin the account it switches to so the monitor keeps it", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		command := commands.NewUseAccountCommand(accounts, &doubles.StubCredentialsRepository{}).
			WithOutput(&bytes.Buffer{})

		// when
		err := command.Execute("b@example.com")

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.PinnedEmail)
		assert.True(t, accounts.Store.Rotation.Pinned())
	})

	t.Run("should say the account stays until it is exhausted and how to release it", func(t *testing.T) {
		t.Parallel()
		// given
		store := twoAccountStore()

		// when
		out, errOut := useOutput(t, store, "b@example.com")

		// then
		assert.Equal(t, "[ccswitch] switched to b@example.com\n"+
			"[ccswitch] pinned until it is exhausted; `ccswitch unpin` lets the rotation policy choose again\n", out)
		assert.Empty(t, errOut)
	})

	t.Run("should say a long-lived account stays until another account is picked", func(t *testing.T) {
		t.Parallel()
		// given: its usage cannot be read, so ccswitch never sees it run out
		store := longLivedFallbackStore()
		store.Rotation.Select("a@example.com")

		// when
		out, _ := useOutput(t, store, "long@example.com")

		// then
		assert.Contains(t, out, "[ccswitch] pinned until you pick another account, since its usage cannot "+
			"be polled; `ccswitch unpin` lets the rotation policy choose again\n")
	})

	t.Run("should hold a pick of an account the monitor read exhausted through that exhaustion",
		func(t *testing.T) {
			t.Parallel()
			// given: an account read at its limit, which may still be served
			recovers := time.Now().Add(recoveryIn)
			store := twoAccountStore()
			store.Rotation.MarkExhausted("b@example.com", recovers)

			// when
			out, errOut := useOutput(t, store, "b@example.com")

			// then: the pick overrules the reading, and says who has to move on if it is wrong
			assert.Equal(t, "[ccswitch] switched to b@example.com\n"+
				"[ccswitch] pinned through the exhaustion the monitor last read, and after that until it is "+
				"exhausted again; `ccswitch unpin` lets the rotation policy choose again\n", out)
			assert.Equal(t, "[ccswitch] WARN: b@example.com last read exhausted, available again in 31m ("+
				momentIn(recovers)+"); it stays pinned until then regardless, since an account at its limit "+
				"may still be served -- if claude refuses it, pick another account or run `ccswitch unpin`\n",
				errOut)
			assert.True(t, store.Rotation.PinOverridesExhaustion(time.Now()))
		})

	t.Run("should warn that the monitor releases the pin on an account its provider refused",
		func(t *testing.T) {
			t.Parallel()
			// given: a refusal, not a reading, so the pick cannot overrule it
			recovers := time.Now().Add(recoveryIn)
			store := twoAccountStore()
			store.Accounts[1].LastUsage = &entities.Usage{Blocked: true}
			store.Rotation.MarkExhausted("b@example.com", recovers)

			// when
			out, errOut := useOutput(t, store, "b@example.com")

			// then: the pick is honored, since the refusal may have ended since the last poll
			assert.Contains(t, out, "[ccswitch] pinned until it is exhausted; ")
			assert.Equal(t, "[ccswitch] WARN: b@example.com is exhausted, available again in 31m ("+
				momentIn(recovers)+"); unless its limits have reset since, the monitor's next poll releases "+
				"the pin and rotates away from it\n", errOut)
			assert.True(t, store.Rotation.Pinned())
			assert.False(t, store.PinHolds(time.Now()))
		})

	t.Run("should name the provider's CLI when a Codex account is picked over its exhaustion",
		func(t *testing.T) {
			t.Parallel()
			// given
			store := twoAccountStore()
			store.Provider = entities.ProviderCodex
			store.Rotation.MarkExhausted("b@example.com", time.Now().Add(recoveryIn))

			// when
			_, errOut := useOutput(t, store, "b@example.com")

			// then
			assert.Contains(t, errOut, "-- if codex refuses it, pick another account")
		})

	t.Run("should return an error when the account is not enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: twoAccountStore()}
		credentials := &doubles.StubCredentialsRepository{}
		command := commands.NewUseAccountCommand(accounts, credentials)

		// when
		err := command.Execute("missing@example.com")

		// then
		require.Error(t, err)
		assert.Zero(t, credentials.WriteCalls)
		assert.False(t, accounts.Store.Rotation.Pinned())
	})
}
