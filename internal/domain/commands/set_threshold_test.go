package commands_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

const (
	raisedThreshold = 100.0
	// borderlinePct sits above the default threshold but below a raised one, which
	// is the whole point of retuning: the account is spent under one and fine under
	// the other.
	borderlinePct = 95.0
)

// borderlineUsage returns usage whose only active limit sits at borderlinePct
// without the server calling it critical.
func borderlineUsage() *entities.Usage {
	return &entities.Usage{Limits: []entities.Limit{{
		Kind:     "weekly_all",
		Percent:  borderlinePct,
		Severity: entities.SeverityWarning,
		IsActive: true,
		ResetsAt: time.Now().Add(longRecovery),
	}}}
}

func TestSetThresholdCommandExecute(t *testing.T) {
	t.Parallel()

	newCommand := func(
		config *entities.Config,
		accounts *doubles.InMemoryAccountsRepository,
		credentials *doubles.StubCredentialsRepository,
		usage *doubles.StubUsageRepository,
		sessions *doubles.StubSessionsRepository,
	) *commands.SetThresholdCommand {
		return commands.NewSetThresholdCommand(config, accounts, credentials, usage, nil, sessions)
	}

	t.Run("should persist the threshold so a running daemon rereads it", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		command := newCommand(monitorConfig(), accounts, credentials, usage,
			&doubles.StubSessionsRepository{})

		// when
		err := command.Execute(raisedThreshold)

		// then
		require.NoError(t, err)
		require.NotNil(t, accounts.Store.Settings.Threshold)
		assert.InDelta(t, raisedThreshold, *accounts.Store.Settings.Threshold, 0)
	})

	t.Run("should return to the primary when raising the threshold clears it", func(t *testing.T) {
		t.Parallel()
		// given: the primary is over the default threshold and was rotated away from,
		// while the backup is idle
		store := livePairStore()
		store.Rotation.CurrentEmail = "b@example.com"
		store.Rotation.MarkExhausted("a@example.com", time.Now().Add(longRecovery))
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "b", RefreshToken: "rb", Scopes: loginScopes()},
			Identity: &entities.AccountIdentity{EmailAddress: "b@example.com"},
		}
		usage := perAccountUsage(map[string]*entities.Usage{
			"a": borderlineUsage(),
			"b": healthyUsage(),
		})
		command := newCommand(monitorConfig(), accounts, credentials, usage,
			&doubles.StubSessionsRepository{})

		// when
		err := command.Execute(raisedThreshold)

		// then: 95% is under 100%, so the primary is no longer spent and takes over
		require.NoError(t, err)
		assert.Equal(t, "a@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.False(t, accounts.Store.Rotation.IsExhausted("a@example.com", time.Now()))
		assert.Equal(t, "a", credentials.Written.AccessToken)
	})

	t.Run("should rotate away when lowering the threshold spends the primary", func(t *testing.T) {
		t.Parallel()
		// given: the primary sits under the default threshold and is active
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := perAccountUsage(map[string]*entities.Usage{
			"a": borderlineUsage(),
			"b": healthyUsage(),
		})
		command := newCommand(monitorConfig(), accounts, credentials, usage,
			&doubles.StubSessionsRepository{})

		// when
		err := command.Execute(borderlinePct - 1)

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Equal(t, "b", credentials.Written.AccessToken)
	})

	t.Run("should defer the switch while a claude session is running", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := perAccountUsage(map[string]*entities.Usage{
			"a": borderlineUsage(),
			"b": healthyUsage(),
		})
		command := newCommand(monitorConfig(), accounts, credentials, usage,
			&doubles.StubSessionsRepository{Running: true})

		// when
		err := command.Execute(borderlinePct - 1)

		// then
		require.NoError(t, err)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
		assert.Equal(t, 0, credentials.WriteCalls)
	})

	t.Run("should reject a threshold outside the 0-100 range", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		command := newCommand(monitorConfig(), accounts, &doubles.StubCredentialsRepository{},
			&doubles.StubUsageRepository{}, &doubles.StubSessionsRepository{})

		// when
		err := command.Execute(raisedThreshold + 1)

		// then
		require.Error(t, err)
		assert.Nil(t, accounts.Store.Settings.Threshold)
	})

	t.Run("should reject a threshold of zero", func(t *testing.T) {
		t.Parallel()
		// given: at 0 every account is spent the instant it is polled
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		command := newCommand(monitorConfig(), accounts, &doubles.StubCredentialsRepository{},
			&doubles.StubUsageRepository{}, &doubles.StubSessionsRepository{})

		// when
		err := command.Execute(0)

		// then
		require.Error(t, err)
		assert.Nil(t, accounts.Store.Settings.Threshold)
	})
}

func TestSetThresholdCommandReset(t *testing.T) {
	t.Parallel()

	t.Run("should drop the stored threshold and re-apply the default", func(t *testing.T) {
		t.Parallel()
		// given: a stored threshold that keeps a spent primary in play
		store := livePairStore()
		store.Settings.SetThreshold(fullPct)
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{
				AccessToken: "a", RefreshToken: "ra", ExpiresAt: farFuture, Scopes: loginScopes(),
			},
			Identity: &entities.AccountIdentity{EmailAddress: "a@example.com"},
		}
		usage := perAccountUsage(map[string]*entities.Usage{
			"a": borderlineUsage(),
			"b": healthyUsage(),
		})
		command := commands.NewSetThresholdCommand(
			monitorConfig(), accounts, credentials, usage, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Reset()

		// then: back to the configured 90%, under which 95% is spent
		require.NoError(t, err)
		assert.Nil(t, accounts.Store.Settings.Threshold)
		assert.Equal(t, "b@example.com", accounts.Store.Rotation.CurrentEmail)
	})

	t.Run("should be a no-op when no threshold was stored", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		command := commands.NewSetThresholdCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{},
			&doubles.StubUsageRepository{}, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Reset()

		// then
		require.NoError(t, err)
		assert.Nil(t, accounts.Store.Settings.Threshold)
		assert.Equal(t, 0, accounts.SaveCalls)
	})
}

func TestSetThresholdCommandRejectsNonNumericThresholds(t *testing.T) {
	t.Parallel()

	t.Run("should reject NaN", func(t *testing.T) {
		t.Parallel()
		// given: `strconv.ParseFloat` accepts the literal "NaN", and every comparison
		// against it is false, so a bare range check admits it. Stored, it would make
		// `Percent >= threshold` false for every limit and disable rotation silently.
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		command := commands.NewSetThresholdCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{},
			&doubles.StubUsageRepository{}, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Execute(math.NaN())

		// then
		require.Error(t, err)
		assert.Nil(t, accounts.Store.Settings.Threshold)
	})

	t.Run("should reject positive infinity", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		command := commands.NewSetThresholdCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{},
			&doubles.StubUsageRepository{}, nil, &doubles.StubSessionsRepository{})

		// when
		err := command.Execute(math.Inf(1))

		// then
		require.Error(t, err)
		assert.Nil(t, accounts.Store.Settings.Threshold)
	})
}
