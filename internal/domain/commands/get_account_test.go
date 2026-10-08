package commands_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

func TestGetAccountCommandExecute(t *testing.T) {
	t.Parallel()

	t.Run("should print the account's place, state and when each limit resets", func(t *testing.T) {
		t.Parallel()
		// given: "b" is exhausted until its 5-hour window resets
		recovers := time.Now().Add(recoveryIn)
		usage := perAccountUsage(map[string]*entities.Usage{
			"b": {
				FiveHour: entities.Window{Utilization: fullPct, ResetsAt: recovers},
				Limits: []entities.Limit{{
					Kind:     entities.LimitKindSession,
					Percent:  fullPct,
					IsActive: true,
					ResetsAt: recovers,
				}},
			},
		})
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		var out bytes.Buffer

		// when
		err := commands.NewGetAccountCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, &doubles.StubTokensRepository{},
		).WithOutput(&out).Execute("b@example.com")

		// then
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(out.String(), "b@example.com\n"), out.String())
		assert.Contains(t, out.String(), "priority:       2 of 2\n")
		assert.Contains(t, out.String(),
			"state:          exhausted, available again in 31m ("+momentIn(recovers)+")\n")
		assert.Regexp(t, `5-hour\s+100%\s+resets in 31m`, out.String())
	})

	t.Run("should mark the active account and name the primary", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		var out bytes.Buffer

		// when
		err := commands.NewGetAccountCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, &doubles.StubTokensRepository{},
		).WithOutput(&out).Execute("a@example.com")

		// then
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(out.String(), "a@example.com (active)\n"), out.String())
		assert.Contains(t, out.String(), "priority:       1 of 2 (primary)\n")
		assert.Contains(t, out.String(), "state:          ok\n")
	})

	t.Run("should save credentials the poll refreshed", func(t *testing.T) {
		t.Parallel()
		// given: the backup's access token is spent, so reading its usage refreshes it
		store := livePairStore()
		store.Accounts[1].Credentials.ExpiresAt = 1
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "b2", RefreshToken: "rb2", Scopes: loginScopes()},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}

		// when
		err := commands.NewGetAccountCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).WithOutput(&bytes.Buffer{}).Execute("b@example.com")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, accounts.SaveCalls, "the rotated refresh token is lost unless the store is saved")
		backup := accounts.Store.FindAccount("b@example.com")
		require.NotNil(t, backup)
		assert.Equal(t, "rb2", backup.Credentials.RefreshToken)
	})

	t.Run("should fall back to the last known reading when the poll fails", func(t *testing.T) {
		t.Parallel()
		// given
		weeklyReset := time.Now().Add(weeklyResetIn)
		store := livePairStore()
		store.Accounts[1].LastUsage = &entities.Usage{
			SevenDay: entities.Window{Utilization: weeklyPct, ResetsAt: weeklyReset},
		}
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		usage := &doubles.StubUsageRepository{Err: errUsageUnreachable}
		var out bytes.Buffer

		// when
		err := commands.NewGetAccountCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, &doubles.StubTokensRepository{},
		).WithOutput(&out).Execute("b@example.com")

		// then
		require.NoError(t, err)
		assert.Contains(t, out.String(), "usage:          unavailable; last known reading:\n")
		assert.Regexp(t, `7-day\s+93%\s+resets in 4d 2h`, out.String())
		assert.Zero(t, accounts.SaveCalls)
	})

	t.Run("should not poll an account enrolled from a long-lived token", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: longLivedFallbackStore()}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		var out bytes.Buffer

		// when
		err := commands.NewGetAccountCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, &doubles.StubTokensRepository{},
		).WithOutput(&out).Execute("long@example.com")

		// then: polling it would 403, so its usage is never asked for
		require.NoError(t, err)
		assert.Zero(t, usage.FetchCalls)
		assert.Contains(t, out.String(), "state:          manual only")
	})

	t.Run("should return an error when the account is not enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}

		// when
		err := commands.NewGetAccountCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, &doubles.StubTokensRepository{},
		).WithOutput(&bytes.Buffer{}).Execute("missing@example.com")

		// then
		require.ErrorIs(t, err, entities.ErrAccountNotEnrolled)
		assert.Zero(t, usage.FetchCalls)
	})
}

func TestGetAccountPreservesRefreshOnUsageFailure(t *testing.T) {
	t.Parallel()

	t.Run("should publish and save rotated tokens even when usage is unavailable", func(t *testing.T) {
		t.Parallel()
		// given: the refresh succeeds, invalidating the old token, but the usage endpoint fails
		store := expiredPrimaryStore()
		previous := store.Accounts[0].Credentials
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		credentials := &doubles.StubCredentialsRepository{Creds: &previous}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{
				AccessToken:  "replacement-access-placeholder",
				RefreshToken: "replacement-refresh-placeholder",
				Scopes:       loginScopes(),
				ExpiresAt:    farFuture,
			},
		}
		var out bytes.Buffer

		// when
		err := commands.NewGetAccountCommand(
			monitorConfig(), accounts, credentials,
			&doubles.StubUsageRepository{Err: errUsageUnreachable}, tokens,
		).WithOutput(&out).Execute("a@example.com")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, accounts.SaveCalls)
		require.NotNil(t, credentials.Written)
		assert.Equal(t, *tokens.Refreshed, *credentials.Written)
		assert.Equal(t, *tokens.Refreshed, accounts.Store.Accounts[0].Credentials)
		assert.Contains(t, out.String(), "usage:          unavailable")
		assert.NotContains(t, out.String(), tokens.Refreshed.AccessToken)
		assert.NotContains(t, out.String(), tokens.Refreshed.RefreshToken)
	})
}

func TestGetAccountDressesUpATerminal(t *testing.T) {
	t.Parallel()

	t.Run("should draw a meter beside each figure and put the reset dates in a column when output is a terminal",
		func(t *testing.T) {
			t.Parallel()
			// given
			now := time.Now()
			sessionReset := now.Add(sessionResetIn)
			weeklyReset := now.Add(weeklyResetIn)
			usage := &doubles.StubUsageRepository{Usage: &entities.Usage{
				FiveHour: entities.Window{Utilization: sessionPct, ResetsAt: sessionReset},
				SevenDay: entities.Window{Utilization: weeklyPct, ResetsAt: weeklyReset},
			}}

			// when
			out, err := showOutput(styledConfig(entities.OutputMonochrome), livePairStore(), usage, "a@example.com")

			// then
			require.NoError(t, err)
			assert.Contains(t, out, "  usage:\n"+
				"     5-hour       ███████████▏░░░░░░░░  56%  resets in 2h 13m   "+momentIn(sessionReset)+"\n"+
				"     7-day        ██████████████████▌░  93%  resets in 4d 2h    "+momentIn(weeklyReset)+"\n")
			assert.NotContains(t, out, "\x1b[", "a terminal without colors gets no escape codes")
		})

	t.Run("should paint an exhausted account and the figure that spent it red when output has colors",
		func(t *testing.T) {
			t.Parallel()
			// given: "b" is exhausted until its 5-hour limit resets
			recovers := time.Now().Add(recoveryIn)
			usage := perAccountUsage(map[string]*entities.Usage{
				"b": {Limits: []entities.Limit{{
					Kind:     entities.LimitKindSession,
					Percent:  fullPct,
					IsActive: true,
					ResetsAt: recovers,
				}}},
			})

			// when
			out, err := showOutput(styledConfig(entities.OutputColor), livePairStore(), usage, "b@example.com")

			// then
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(out, sgr("1;31", "b@example.com")+"\n"), out)
			assert.Contains(t, out,
				"  state:          "+sgr("1;31", "exhausted")+", available again "+sgr("36", "in 31m")+
					" "+sgr("2", "("+momentIn(recovers)+")")+"\n")
			assert.Contains(t, out,
				"     5-hour       "+sgr("31", "████████████████████")+" "+sgr("31", "100%")+
					"  "+sgr("2", "resets")+" "+sgr("36", "in 31m"))
		})

	t.Run("should mark the active account green and color when its tokens expire when output has colors",
		func(t *testing.T) {
			t.Parallel()
			// given
			now := time.Now()
			accessExpiry, refreshExpiry := now.Add(sessionResetIn), now.Add(weeklyResetIn)
			store := livePairStore()
			store.Accounts[0].Credentials.ExpiresAt = accessExpiry.UnixMilli()
			store.Accounts[0].Credentials.RefreshTokenExpiresAt = refreshExpiry.UnixMilli()
			usage := &doubles.StubUsageRepository{Usage: healthyUsage()}

			// when
			out, err := showOutput(styledConfig(entities.OutputColor), store, usage, "a@example.com")

			// then
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(out, sgr("1", "a@example.com")+" "+sgr("32", "(active)")+"\n"), out)
			assert.Contains(t, out, "  state:          "+sgr("32", "ok")+"\n")
			assert.Contains(t, out,
				"  access token:   expires "+sgr("36", "in 2h 13m")+" "+sgr("2", "("+momentIn(accessExpiry)+")")+"\n")
			assert.Contains(t, out,
				"  refresh token:  expires "+sgr("36", "in 4d 2h")+" "+sgr("2", "("+momentIn(refreshExpiry)+")")+"\n")
		})

	t.Run("should flag discarded credentials and unread usage in yellow when output has colors",
		func(t *testing.T) {
			t.Parallel()
			// given: "b" lost the inference scope, and the refresh that would repair it fails
			store := livePairStore()
			store.Accounts[1].Credentials.Scopes = []string{"user:profile"}
			store.Accounts[1].LastUsage = &entities.Usage{
				SevenDay: entities.Window{Utilization: weeklyPct, ResetsAt: time.Now().Add(weeklyResetIn)},
			}
			usage := &doubles.StubUsageRepository{Usage: healthyUsage()}

			// when
			out, err := showOutput(styledConfig(entities.OutputColor), store, usage, "b@example.com")

			// then
			require.NoError(t, err)
			assert.Contains(t, out, "  warning:        "+sgr("33", `the credentials lack the "user:inference" scope, `+
				"so Claude Code will discard them; log in again with `claude` and re-enroll")+"\n")
			assert.Contains(t, out, "  usage:          "+sgr("33", "unavailable; last known reading:")+"\n")
			assert.Contains(t, out, "     7-day        "+sgr("31", "██████████████████▌"))
		})

	t.Run("should paint an account red from its exhaustion marker when its usage cannot be read",
		func(t *testing.T) {
			t.Parallel()
			// given: the monitor marked "b" exhausted, and the endpoint now refuses it
			store := livePairStore()
			store.Rotation.MarkExhausted("b@example.com", time.Now().Add(recoveryIn))
			usage := &doubles.StubUsageRepository{Err: errUsageUnreachable}

			// when
			out, err := showOutput(styledConfig(entities.OutputColor), store, usage, "b@example.com")

			// then
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(out, sgr("1;31", "b@example.com")+"\n"), out)
			assert.Contains(t, out, "  usage:          "+sgr("33", "unavailable")+"\n")
		})

	t.Run("should mark an account enrolled from a long-lived token as manual only in its own color",
		func(t *testing.T) {
			t.Parallel()
			// given
			usage := &doubles.StubUsageRepository{}

			// when
			out, err := showOutput(styledConfig(entities.OutputColor), longLivedOnlyStore(), usage, "long@example.com")

			// then
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(out, sgr("1", "long@example.com")+" "+sgr("32", "(active)")+"\n"), out)
			assert.Contains(t, out, "  state:          "+sgr("35", "manual only")+" "+
				sgr("2", "(long-lived token; its usage cannot be polled, so select it with `ccswitch use`)")+"\n")
		})

	t.Run("should print neither meters nor escape codes when output is not a terminal", func(t *testing.T) {
		t.Parallel()
		// given
		usage := &doubles.StubUsageRepository{Usage: &entities.Usage{
			FiveHour: entities.Window{Utilization: sessionPct, ResetsAt: time.Now().Add(sessionResetIn)},
		}}

		// when
		out, err := showOutput(monitorConfig(), livePairStore(), usage, "a@example.com")

		// then
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(out, "a@example.com (active)\n"), out)
		assert.Regexp(t, `\n     5-hour\s+56%  resets in 2h 13m \(`, out)
		assert.NotContains(t, out, "\x1b[")
		assert.NotContains(t, out, "█")
		assert.NotContains(t, out, "░")
	})
}

// showOutput runs `show` for the email over the store and the usage, and returns
// what it printed.
func showOutput(
	config *entities.Config,
	store *entities.Store,
	usage *doubles.StubUsageRepository,
	email string,
) (string, error) {
	var out bytes.Buffer
	err := commands.NewGetAccountCommand(
		config, &doubles.InMemoryAccountsRepository{Store: store}, &doubles.StubCredentialsRepository{},
		usage, &doubles.StubTokensRepository{},
	).WithOutput(&out).Execute(email)
	return out.String(), err
}
