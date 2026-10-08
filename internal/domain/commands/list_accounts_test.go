package commands_test

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/commands"
	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

// errUsageUnreachable is a plain failure used where the specific cause is
// irrelevant — anything that is not an authentication problem.
var errUsageUnreachable = errors.New("usage endpoint unreachable")

func TestListAccountsPersistsRefreshedCredentials(t *testing.T) {
	t.Parallel()

	t.Run("should save credentials minted while polling a backup account", func(t *testing.T) {
		t.Parallel()

		// given
		store := livePairStore()
		// The backup's access token is spent, so polling it has to refresh first.
		store.Accounts[1].Credentials.ExpiresAt = 1
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "b2", RefreshToken: "rb2", Scopes: loginScopes()},
		}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, tokens.RefreshCalls)
		assert.Positive(t, accounts.SaveCalls,
			"the rotated refresh token is lost unless the store is saved")

		backup := accounts.Store.FindAccount("b@example.com")
		require.NotNil(t, backup)
		assert.Equal(t, "b2", backup.Credentials.AccessToken)
		assert.Equal(t, "rb2", backup.Credentials.RefreshToken,
			"the server invalidated 'rb' when it minted 'rb2'")
	})

	t.Run("should not save when no poll refreshed anything", func(t *testing.T) {
		t.Parallel()

		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		tokens := &doubles.StubTokensRepository{}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		assert.Zero(t, tokens.RefreshCalls)
		assert.Zero(t, accounts.SaveCalls)
	})

	t.Run("should keep credentials refreshed before a failing usage call", func(t *testing.T) {
		t.Parallel()

		// given
		store := livePairStore()
		store.Accounts[1].Credentials.ExpiresAt = 1
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		// The refresh succeeds and rotates 'rb' server-side; the usage call after it
		// still fails, and the new pair must survive that.
		usage := &doubles.StubUsageRepository{
			ByToken:    map[string]*entities.Usage{"a": healthyUsage()},
			ErrByToken: map[string]error{"b2": errUsageUnreachable},
		}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "b2", RefreshToken: "rb2", Scopes: loginScopes()},
		}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		backup := accounts.Store.FindAccount("b@example.com")
		require.NotNil(t, backup)
		assert.Equal(t, "rb2", backup.Credentials.RefreshToken)
	})

	t.Run("should publish the active account's refreshed pair to the credentials store", func(t *testing.T) {
		t.Parallel()

		// given
		store := livePairStore()
		store.Accounts[0].Credentials.ExpiresAt = 1
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		// Claude Code still holds exactly the pair the refresh consumed.
		credentials := &doubles.StubCredentialsRepository{
			Creds: &entities.OAuthCredentials{AccessToken: "a", RefreshToken: "ra", Scopes: loginScopes()},
		}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "a2", RefreshToken: "ra2", Scopes: loginScopes()},
		}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, credentials, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		require.NotNil(t, credentials.Written,
			"leaving Claude Code on the rotated-away token logs the session out")
		assert.Equal(t, "ra2", credentials.Written.RefreshToken)
	})
}

func TestListAccountsSkipsLongLivedAccounts(t *testing.T) {
	t.Parallel()

	t.Run("should never refresh or save an account enrolled from a long-lived token", func(t *testing.T) {
		t.Parallel()

		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: longLivedOnlyStore()}
		usage := &doubles.StubUsageRepository{Usage: healthyUsage()}
		tokens := &doubles.StubTokensRepository{}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		assert.Zero(t, usage.FetchCalls)
		assert.Zero(t, tokens.RefreshCalls)
		assert.Zero(t, accounts.SaveCalls)
	})
}

func TestListAccountsRefreshesRejectedToken(t *testing.T) {
	t.Parallel()

	t.Run("should recover an account whose token was invalidated before its expiry", func(t *testing.T) {
		t.Parallel()

		// given
		store := &entities.Store{
			Accounts: []entities.Account{{
				Email: "a@example.com",
				Order: 0,
				// Far in the future: nothing about the timestamp suggests a refresh.
				Credentials: entities.OAuthCredentials{
					AccessToken:  "stale",
					RefreshToken: "ra",
					ExpiresAt:    farFuture,
					Scopes:       loginScopes(),
				},
			}},
			Rotation: entities.RotationState{CurrentEmail: "a@example.com"},
		}
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		usage := &doubles.StubUsageRepository{
			ErrByToken: map[string]error{
				"stale": fmt.Errorf("wrapped: %w", repositories.ErrUnauthorized),
			},
			ByToken: map[string]*entities.Usage{"fresh": healthyUsage()},
		}
		tokens := &doubles.StubTokensRepository{
			Refreshed: &entities.OAuthCredentials{AccessToken: "fresh", RefreshToken: "ra2", Scopes: loginScopes()},
		}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, tokens,
		).Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, tokens.RefreshCalls)
		assert.Equal(t, []string{"stale", "fresh"}, usage.Tokens)

		account := accounts.Store.FindAccount("a@example.com")
		require.NotNil(t, account)
		assert.Equal(t, "fresh", account.Credentials.AccessToken)
	})
}

func TestListAccountsPrintsWhenLimitsReset(t *testing.T) {
	t.Parallel()

	t.Run("should number accounts from 1 and print when each window resets", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		sessionReset := now.Add(sessionResetIn)
		weeklyReset := now.Add(weeklyResetIn)
		usage := &doubles.StubUsageRepository{Usage: &entities.Usage{
			FiveHour: entities.Window{Utilization: sessionPct, ResetsAt: sessionReset},
			SevenDay: entities.Window{Utilization: weeklyPct, ResetsAt: weeklyReset},
		}}
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		var out bytes.Buffer

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, &doubles.StubTokensRepository{},
		).WithOutput(&out).Execute()

		// then
		require.NoError(t, err)
		assert.Contains(t, out.String(), "* 1. a@example.com [ok]\n")
		assert.Contains(t, out.String(), "  2. b@example.com [ok]\n")
		assert.Regexp(t, `5-hour\s+56%\s+resets in 2h 13m \(`+regexp.QuoteMeta(momentIn(sessionReset))+`\)`,
			out.String())
		assert.Regexp(t, `7-day\s+93%\s+resets in 4d 2h \(`+regexp.QuoteMeta(momentIn(weeklyReset))+`\)`,
			out.String())
	})

	t.Run("should print when an exhausted account is available again", func(t *testing.T) {
		t.Parallel()
		// given: a scoped weekly limit, which neither window shows, exhausts "a"
		recovers := time.Now().Add(recoveryIn)
		usage := perAccountUsage(map[string]*entities.Usage{
			"a": {Limits: []entities.Limit{{
				Kind:     entities.LimitKindWeeklyScoped,
				Percent:  fullPct,
				IsActive: true,
				ResetsAt: recovers,
			}}},
			"b": healthyUsage(),
		})
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		var out bytes.Buffer

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, &doubles.StubTokensRepository{},
		).WithOutput(&out).Execute()

		// then
		require.NoError(t, err)
		assert.Contains(t, out.String(),
			"* 1. a@example.com [exhausted, available again in 31m ("+momentIn(recovers)+")]\n")
		assert.Regexp(t, `7-day scoped\s+100%\s+resets in 31m`, out.String())
	})

	t.Run("should fall back to the last known reading when the poll fails", func(t *testing.T) {
		t.Parallel()
		// given: the endpoint refuses "b", whose last reading the monitor recorded
		weeklyReset := time.Now().Add(weeklyResetIn)
		store := livePairStore()
		store.Accounts[1].LastUsage = &entities.Usage{
			SevenDay: entities.Window{Utilization: weeklyPct, ResetsAt: weeklyReset},
		}
		usage := &doubles.StubUsageRepository{
			ByToken:    map[string]*entities.Usage{"a": healthyUsage()},
			ErrByToken: map[string]error{"b": errUsageUnreachable},
		}
		accounts := &doubles.InMemoryAccountsRepository{Store: store}
		var out bytes.Buffer

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, &doubles.StubTokensRepository{},
		).WithOutput(&out).Execute()

		// then
		require.NoError(t, err)
		assert.Contains(t, out.String(), "usage unavailable; last known reading:")
		assert.Regexp(t, `7-day\s+93%\s+resets in 4d 2h`, out.String())
	})

	t.Run("should leave a zero unit out of a countdown and floor it at under a minute", func(t *testing.T) {
		t.Parallel()
		// given
		now := time.Now()
		usage := &doubles.StubUsageRepository{Usage: &entities.Usage{
			FiveHour: entities.Window{Utilization: sessionPct, ResetsAt: now.Add(wholeHoursResetIn)},
			SevenDay: entities.Window{Utilization: weeklyPct, ResetsAt: now.Add(imminentResetIn)},
		}}
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		var out bytes.Buffer

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{}, usage, &doubles.StubTokensRepository{},
		).WithOutput(&out).Execute()

		// then
		require.NoError(t, err)
		assert.Regexp(t, `5-hour\s+56%\s+resets in 4h \(`, out.String())
		assert.Regexp(t, `7-day\s+93%\s+resets in <1m \(`, out.String())
	})

	t.Run("should mark an account enrolled from a long-lived token as manual only", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: longLivedOnlyStore()}
		var out bytes.Buffer

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{},
			&doubles.StubUsageRepository{}, &doubles.StubTokensRepository{},
		).WithOutput(&out).Execute()

		// then
		require.NoError(t, err)
		assert.Contains(t, out.String(), "* 1. long@example.com [manual only]")
	})
}

func TestListAccountsDressesUpATerminal(t *testing.T) {
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
			out, err := listOutput(styledConfig(entities.OutputMonochrome), livePairStore(), usage)

			// then
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(out, "rotation threshold: 90%\n\n* 1. a@example.com [ok]\n"),
				"a decorated listing sets every account off with a blank line")
			assert.Contains(t, out,
				"     5-hour       ███████████▏░░░░░░░░  56%  resets in 2h 13m   "+momentIn(sessionReset)+"\n")
			assert.Contains(t, out,
				"     7-day        ██████████████████▌░  93%  resets in 4d 2h    "+momentIn(weeklyReset)+"\n")
			assert.NotContains(t, out, "\x1b[", "a terminal without colors gets no escape codes")
		})

	t.Run("should leave a meter empty at 0% and fill it no further than full when a figure passes 100%",
		func(t *testing.T) {
			t.Parallel()
			// given
			now := time.Now()
			usage := &doubles.StubUsageRepository{Usage: &entities.Usage{
				FiveHour: entities.Window{Utilization: 0, ResetsAt: now.Add(sessionResetIn)},
				SevenDay: entities.Window{Utilization: overflowPct, ResetsAt: now.Add(weeklyResetIn)},
			}}

			// when
			out, err := listOutput(styledConfig(entities.OutputMonochrome), livePairStore(), usage)

			// then
			require.NoError(t, err)
			assert.Contains(t, out, "     5-hour       ░░░░░░░░░░░░░░░░░░░░   0%  resets in 2h 13m")
			assert.Contains(t, out, "     7-day        ████████████████████ 120%  resets in 4d 2h")
		})

	t.Run("should paint an exhausted account and the figure that spent it red when output has colors",
		func(t *testing.T) {
			t.Parallel()
			// given: a scoped weekly limit at its ceiling exhausts "a"
			recovers := time.Now().Add(recoveryIn)
			usage := perAccountUsage(map[string]*entities.Usage{
				"a": {Limits: []entities.Limit{{
					Kind:     entities.LimitKindWeeklyScoped,
					Percent:  fullPct,
					IsActive: true,
					ResetsAt: recovers,
				}}},
				"b": healthyUsage(),
			})

			// when
			out, err := listOutput(styledConfig(entities.OutputColor), livePairStore(), usage)

			// then
			require.NoError(t, err)
			assert.Contains(t, out, "rotation threshold: "+sgr("1", "90%")+"\n")
			assert.Contains(t, out,
				sgr("32", "*")+" "+sgr("2", "1.")+" "+sgr("1;31", "a@example.com")+
					" ["+sgr("1;31", "exhausted")+", available again "+sgr("36", "in 31m")+
					" "+sgr("2", "("+momentIn(recovers)+")")+"]\n")
			assert.Contains(t, out,
				"7-day scoped "+sgr("31", "████████████████████")+" "+sgr("31", "100%")+
					"  "+sgr("2", "resets")+" "+sgr("36", "in 31m")+"      "+sgr("2", momentIn(recovers))+"\n")
			assert.Contains(t, out, "  "+sgr("2", "2.")+" "+sgr("1", "b@example.com")+" ["+sgr("32", "ok")+"]\n")
		})

	t.Run("should color a figure green, then yellow from 80% of the threshold, then red once it reaches it",
		func(t *testing.T) {
			t.Parallel()
			// given: with the threshold at 90%, figures turn yellow from 72%
			usage := &doubles.StubUsageRepository{Usage: &entities.Usage{
				FiveHour: entities.Window{Utilization: sessionPct},
				SevenDay: entities.Window{Utilization: weeklyPct},
				Limits:   []entities.Limit{{Kind: entities.LimitKindWeeklyScoped, Percent: nearingPct}},
			}}

			// when
			out, err := listOutput(styledConfig(entities.OutputColor), livePairStore(), usage)

			// then
			require.NoError(t, err)
			assert.Contains(t, out, sgr("32", "███████████▏")+sgr("2", "░░░░░░░░")+" "+sgr("32", " 56%"))
			assert.Contains(t, out, sgr("33", "███████████████")+sgr("2", "░░░░░")+" "+sgr("33", " 75%"))
			assert.Contains(t, out, sgr("31", "██████████████████▌")+sgr("2", "░")+" "+sgr("31", " 93%"))
		})

	t.Run("should round a figure down so that it never reads as the threshold before reaching it",
		func(t *testing.T) {
			t.Parallel()
			// given: the threshold is 90%
			usage := &doubles.StubUsageRepository{Usage: &entities.Usage{
				FiveHour: entities.Window{Utilization: justUnderThresholdPct},
				SevenDay: entities.Window{Utilization: almostFullPct},
			}}

			// when
			out, err := listOutput(styledConfig(entities.OutputColor), livePairStore(), usage)

			// then
			require.NoError(t, err)
			assert.Contains(t, out, sgr("33", "█████████████████▉")+sgr("2", "░░")+" "+sgr("33", " 89%"),
				"89.6% has not reached the threshold, so it must not read 90%")
			assert.Contains(t, out, sgr("31", "███████████████████▉")+" "+sgr("31", " 99%"),
				"only a figure at its limit fills the meter's last cell")
		})

	t.Run("should flag a stale reading and a reset already past in yellow when output has colors",
		func(t *testing.T) {
			t.Parallel()
			// given: the endpoint refuses "b", whose last reading predates a reset
			store := livePairStore()
			store.Accounts[1].LastUsage = &entities.Usage{
				FiveHour: entities.Window{Utilization: lowPct, ResetsAt: time.Now().Add(-staleFor)},
			}
			usage := &doubles.StubUsageRepository{
				ByToken:    map[string]*entities.Usage{"a": healthyUsage()},
				ErrByToken: map[string]error{"b": errUsageUnreachable},
			}

			// when
			out, err := listOutput(styledConfig(entities.OutputColor), store, usage)

			// then
			require.NoError(t, err)
			assert.Contains(t, out, "     "+sgr("33", "usage unavailable; last known reading:")+"\n")
			assert.Contains(t, out, sgr("2", "reset")+" "+sgr("33", "12m ago"))
		})

	t.Run("should paint an account red from its exhaustion marker when its usage cannot be read",
		func(t *testing.T) {
			t.Parallel()
			// given: the monitor marked "b" exhausted, and the endpoint now refuses it
			recovers := time.Now().Add(recoveryIn)
			store := livePairStore()
			store.Rotation.MarkExhausted("b@example.com", recovers)
			usage := &doubles.StubUsageRepository{
				ByToken:    map[string]*entities.Usage{"a": healthyUsage()},
				ErrByToken: map[string]error{"b": errUsageUnreachable},
			}

			// when
			out, err := listOutput(styledConfig(entities.OutputColor), store, usage)

			// then
			require.NoError(t, err)
			assert.Contains(t, out,
				"  "+sgr("2", "2.")+" "+sgr("1;31", "b@example.com")+
					" ["+sgr("1;31", "exhausted")+", available again "+sgr("36", "in 31m")+
					" "+sgr("2", "("+momentIn(recovers)+")")+"]\n")
		})

	t.Run("should say no more than exhausted when no limit over the threshold reports its reset",
		func(t *testing.T) {
			t.Parallel()
			// given
			usage := perAccountUsage(map[string]*entities.Usage{
				"a": {
					Limits: []entities.Limit{{Kind: entities.LimitKindWeeklyScoped, Percent: fullPct, IsActive: true}},
				},
				"b": healthyUsage(),
			})

			// when
			out, err := listOutput(styledConfig(entities.OutputColor), livePairStore(), usage)

			// then
			require.NoError(t, err)
			assert.Contains(t, out, sgr("1;31", "a@example.com")+" ["+sgr("1;31", "exhausted")+"]\n")
		})

	t.Run("should mark an account enrolled from a long-lived token as manual only in its own color",
		func(t *testing.T) {
			t.Parallel()
			// given
			usage := &doubles.StubUsageRepository{}

			// when
			out, err := listOutput(styledConfig(entities.OutputColor), longLivedOnlyStore(), usage)

			// then
			require.NoError(t, err)
			assert.Contains(t, out,
				sgr("32", "*")+" "+sgr("2", "1.")+" "+sgr("1", "long@example.com")+
					" ["+sgr("35", "manual only")+"] "+sgr("2", "long-lived token; its usage cannot be polled")+"\n")
		})

	t.Run("should print neither meters nor escape codes when output is not a terminal", func(t *testing.T) {
		t.Parallel()
		// given
		usage := &doubles.StubUsageRepository{Usage: &entities.Usage{
			FiveHour: entities.Window{Utilization: sessionPct, ResetsAt: time.Now().Add(sessionResetIn)},
		}}

		// when
		out, err := listOutput(monitorConfig(), livePairStore(), usage)

		// then
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(out, "rotation threshold: 90%\n* 1. a@example.com [ok]\n"))
		assert.NotContains(t, out, "\x1b[")
		assert.NotContains(t, out, "█")
		assert.NotContains(t, out, "░")
	})
}

// sgr wraps text in the escape codes a colored listing prints it with.
func sgr(params, text string) string {
	return "\x1b[" + params + "m" + text + "\x1b[0m"
}

// styledConfig returns monitorConfig with the given output style.
func styledConfig(style entities.OutputStyle) *entities.Config {
	config := monitorConfig()
	config.Output = style
	return config
}

// listOutput runs `list` over the store and the usage, and returns what it printed.
func listOutput(
	config *entities.Config,
	store *entities.Store,
	usage *doubles.StubUsageRepository,
) (string, error) {
	var out bytes.Buffer
	err := commands.NewListAccountsCommand(
		config, &doubles.InMemoryAccountsRepository{Store: store}, &doubles.StubCredentialsRepository{},
		usage, &doubles.StubTokensRepository{},
	).WithOutput(&out).Execute()
	return out.String(), err
}
