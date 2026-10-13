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
	"github.com/rios0rios0/ccswitch/test/doubles"
)

const (
	codexWindowKind = "30-day"
	codexModelLimit = "7-day GPT-5.3-Codex-Spark"
	codexSessionPct = 12.0
	codexModelPct   = 3.0
	monthlyResetIn  = 29*24*time.Hour + 18*time.Hour + 30*time.Minute
)

// errStoreUnreadable is a store that cannot be loaded, for a pool that fails its tick.
var errStoreUnreadable = errors.New("store unreadable")

// codexCreds returns a live ChatGPT sign-in as Codex holds one: an ID token beside the
// pair, and no scopes, since a Codex sign-in names none of Claude's.
func codexCreds(access, refresh string) entities.OAuthCredentials {
	return entities.OAuthCredentials{
		AccessToken:  access,
		RefreshToken: refresh,
		IDToken:      "id-" + access,
		ExpiresAt:    farFuture,
	}
}

// codexStore returns the store of two live Codex accounts, "x" (current) and "y".
func codexStore() *entities.Store {
	return &entities.Store{
		Provider: entities.ProviderCodex,
		Accounts: []entities.Account{
			{Email: "x@example.com", Order: 0, Credentials: codexCreds("x", "rx")},
			{Email: "y@example.com", Order: 1, Credentials: codexCreds("y", "ry")},
		},
		Rotation: entities.RotationState{CurrentEmail: "x@example.com"},
	}
}

// codexUsage returns the usage of a free Codex plan, reported as limits only, whose
// single 30-day window is used to the percent and resets at the given time.
func codexUsage(percent float64, reset time.Time) *entities.Usage {
	return &entities.Usage{LimitsOnly: true, Limits: []entities.Limit{
		{Kind: codexWindowKind, Percent: percent, IsActive: true, ResetsAt: reset},
	}}
}

// codexPool returns a pool over the given Codex store whose CLI is signed in to "x".
func codexPool(store *entities.Store, usage *doubles.StubUsageRepository) commands.Pool {
	current := codexCreds("x", "rx")
	return commands.Pool{
		Accounts: &doubles.InMemoryAccountsRepository{Store: store},
		Credentials: &doubles.StubCredentialsRepository{
			Creds:    &current,
			Identity: &entities.AccountIdentity{EmailAddress: "x@example.com"},
		},
		Usage:    usage,
		Tokens:   &doubles.StubTokensRepository{},
		Sessions: &doubles.StubSessionsRepository{},
	}
}

// exhaustedCodexPool returns a pool over codexStore whose current account "x" has spent
// its monthly window while "y" has capacity.
func exhaustedCodexPool() commands.Pool {
	reset := time.Now().Add(monthlyResetIn)
	return codexPool(codexStore(), perAccountUsage(map[string]*entities.Usage{
		"x": codexUsage(fullPct, reset),
		"y": codexUsage(lowPct, reset),
	}))
}

// listBoth runs `list` over the Claude store and the Codex pool, and returns what it
// printed.
func listBoth(config *entities.Config, claude *entities.Store, codex commands.Pool) (string, error) {
	var out bytes.Buffer
	err := commands.NewListAccountsCommand(
		config, &doubles.InMemoryAccountsRepository{Store: claude}, &doubles.StubCredentialsRepository{},
		&doubles.StubUsageRepository{Usage: healthyUsage()}, &doubles.StubTokensRepository{},
	).WithPool(codex).WithOutput(&out).Execute()
	return out.String(), err
}

func TestListAccountsNamesEachProvider(t *testing.T) {
	t.Parallel()

	t.Run("should name each provider above its accounts once Codex accounts are enrolled", func(t *testing.T) {
		t.Parallel()
		// given
		reset := time.Now().Add(monthlyResetIn)
		codex := codexPool(codexStore(), &doubles.StubUsageRepository{
			Usage: codexUsage(fullPct, reset),
		})

		// when
		out, err := listBoth(monitorConfig(), livePairStore(), codex)

		// then
		require.NoError(t, err)
		assert.Contains(t, out, "rotation threshold: 90%\nClaude\n* 1. a@example.com [ok]\n")
		assert.Contains(t, out, "\nCodex\n* 1. x@example.com [exhausted, available again in 29d 18h (")
		assert.Regexp(t, `\n     30-day\s+100%  resets in 29d 18h \(`+regexp.QuoteMeta(momentIn(reset))+`\)\n`, out)
		assert.Contains(t, out, "  2. y@example.com [exhausted",
			"every Codex account polls the same canned usage here")
	})

	t.Run("should list a store of Claude accounts alone exactly as it did before Codex", func(t *testing.T) {
		t.Parallel()
		// given: scripts read the plain listing, so it must not change for anyone who
		// enrolls no Codex account
		empty := codexPool(&entities.Store{Provider: entities.ProviderCodex}, &doubles.StubUsageRepository{})
		var single bytes.Buffer
		require.NoError(t, commands.NewListAccountsCommand(
			monitorConfig(), &doubles.InMemoryAccountsRepository{Store: livePairStore()},
			&doubles.StubCredentialsRepository{}, &doubles.StubUsageRepository{Usage: healthyUsage()},
			&doubles.StubTokensRepository{},
		).WithOutput(&single).Execute())

		// when
		out, err := listBoth(monitorConfig(), livePairStore(), empty)

		// then
		require.NoError(t, err)
		assert.Equal(t, single.String(), out)
		assert.NotContains(t, out, "Claude")
	})

	t.Run("should name Codex above its accounts when it is the only provider with any", func(t *testing.T) {
		t.Parallel()
		// given
		codex := codexPool(codexStore(), &doubles.StubUsageRepository{
			Usage: codexUsage(lowPct, time.Now().Add(monthlyResetIn)),
		})

		// when
		out, err := listBoth(monitorConfig(), &entities.Store{}, codex)

		// then
		require.NoError(t, err)
		assert.Contains(t, out, "rotation threshold: 90%\nCodex\n* 1. x@example.com [ok]\n")
		assert.NotContains(t, out, "Claude")
	})

	t.Run("should mark each provider with its logo in its brand color when output has colors", func(t *testing.T) {
		t.Parallel()
		// given
		codex := codexPool(codexStore(), &doubles.StubUsageRepository{
			Usage: codexUsage(lowPct, time.Now().Add(monthlyResetIn)),
		})

		// when
		out, err := listBoth(styledConfig(entities.OutputColor), livePairStore(), codex)

		// then
		require.NoError(t, err)
		assert.Contains(t, out, "\n\n"+sgr("91", "✻ Claude")+"\n",
			"bright red is how Claude Code paints its own logo on a sixteen-color terminal")
		assert.Contains(t, out, "\n\n"+sgr("35", ">_ Codex")+"\n",
			"magenta is the color the Codex CLI's style guide reserves for Codex")
	})

	t.Run("should widen the label column to fit a limit named for a model", func(t *testing.T) {
		t.Parallel()
		// given: a paid Codex plan, with a limit on one model besides its main window
		reset := time.Now().Add(sessionResetIn)
		usage := &entities.Usage{LimitsOnly: true, Limits: []entities.Limit{
			{Kind: "5-hour", Percent: codexSessionPct, IsActive: true, ResetsAt: reset},
			{Kind: codexModelLimit, Percent: codexModelPct, ResetsAt: reset},
		}}
		codex := codexPool(codexStore(), &doubles.StubUsageRepository{Usage: usage})
		width := len(codexModelLimit)

		// when
		out, err := listBoth(monitorConfig(), &entities.Store{}, codex)

		// then
		require.NoError(t, err)
		assert.Contains(t, out, fmt.Sprintf("     %-*s %4.0f%%  resets in 2h 13m", width, "5-hour", codexSessionPct))
		assert.Contains(
			t,
			out,
			fmt.Sprintf("     %-*s %4.0f%%  resets in 2h 13m", width, codexModelLimit, codexModelPct),
		)
	})

	t.Run("should save a provider's refreshed credentials to that provider's store alone", func(t *testing.T) {
		t.Parallel()
		// given: the backup Codex account's access token is spent
		store := codexStore()
		store.Accounts[1].Credentials.ExpiresAt = 1
		codex := codexPool(store, &doubles.StubUsageRepository{
			Usage: codexUsage(lowPct, time.Now().Add(monthlyResetIn)),
		})
		refreshed := codexCreds("y2", "ry2")
		codex.Tokens = &doubles.StubTokensRepository{Refreshed: &refreshed}
		claude := &doubles.InMemoryAccountsRepository{Store: livePairStore()}

		// when
		err := commands.NewListAccountsCommand(
			monitorConfig(), claude, &doubles.StubCredentialsRepository{},
			&doubles.StubUsageRepository{Usage: healthyUsage()}, &doubles.StubTokensRepository{},
		).WithPool(codex).WithOutput(&bytes.Buffer{}).Execute()

		// then
		require.NoError(t, err)
		codexAccounts, ok := codex.Accounts.(*doubles.InMemoryAccountsRepository)
		require.True(t, ok)
		assert.Positive(t, codexAccounts.SaveCalls, "the rotated refresh token is lost unless its store is saved")
		assert.Equal(t, "ry2", codexAccounts.Store.FindAccount("y@example.com").Credentials.RefreshToken)
		assert.Zero(t, claude.SaveCalls, "nothing of Claude's was refreshed")
	})

	t.Run("should not refresh a live Codex token for naming no Claude scope", func(t *testing.T) {
		t.Parallel()
		// given: Claude's credentials must name user:inference and are repaired when they
		// do not; a Codex sign-in names none, and repairing it would spend a refresh
		// token on every poll
		codex := codexPool(codexStore(), &doubles.StubUsageRepository{
			Usage: codexUsage(lowPct, time.Now().Add(monthlyResetIn)),
		})
		tokens := &doubles.StubTokensRepository{}
		codex.Tokens = tokens

		// when
		_, err := listBoth(monitorConfig(), &entities.Store{}, codex)

		// then
		require.NoError(t, err)
		assert.Zero(t, tokens.RefreshCalls)
	})
}

func TestMonitorRotatesEachProviderOnItsOwn(t *testing.T) {
	t.Parallel()

	t.Run("should switch a provider's account without touching another provider's credentials", func(t *testing.T) {
		t.Parallel()
		// given
		claudeCredentials := &doubles.StubCredentialsRepository{}
		codex := exhaustedCodexPool()
		monitor := commands.NewMonitorCommand(
			monitorConfig(), &doubles.InMemoryAccountsRepository{Store: livePairStore()}, claudeCredentials,
			perAccountUsage(map[string]*entities.Usage{"a": healthyUsage(), "b": healthyUsage()}),
			&doubles.StubTokensRepository{}, &doubles.StubSessionsRepository{},
		).WithPool(codex)

		// when
		err := monitor.Tick(time.Now())

		// then
		require.NoError(t, err)
		codexCredentials, ok := codex.Credentials.(*doubles.StubCredentialsRepository)
		require.True(t, ok)
		require.NotNil(t, codexCredentials.Written)
		assert.Equal(t, "y", codexCredentials.Written.AccessToken)
		codexAccounts, ok := codex.Accounts.(*doubles.InMemoryAccountsRepository)
		require.True(t, ok)
		assert.Equal(t, "y@example.com", codexAccounts.Store.Rotation.CurrentEmail)
		assert.Zero(t, claudeCredentials.WriteCalls, "Claude Code's credentials have nothing to do with Codex")
	})

	t.Run("should still tick the other providers when one provider's store cannot be read", func(t *testing.T) {
		t.Parallel()
		// given
		codex := exhaustedCodexPool()
		monitor := commands.NewMonitorCommand(
			monitorConfig(), &doubles.InMemoryAccountsRepository{LoadErr: errStoreUnreadable},
			&doubles.StubCredentialsRepository{}, &doubles.StubUsageRepository{},
			&doubles.StubTokensRepository{}, &doubles.StubSessionsRepository{},
		).WithPool(codex)

		// when
		err := monitor.Tick(time.Now())

		// then
		require.ErrorIs(t, err, errStoreUnreadable)
		codexAccounts, ok := codex.Accounts.(*doubles.InMemoryAccountsRepository)
		require.True(t, ok)
		assert.Equal(t, "y@example.com", codexAccounts.Store.Rotation.CurrentEmail)
	})

	t.Run("should defer a switch while that provider's CLI runs, whatever the other's does", func(t *testing.T) {
		t.Parallel()
		// given: codex is running and claude is not
		codex := exhaustedCodexPool()
		codex.Sessions = &doubles.StubSessionsRepository{Active: true}
		monitor := commands.NewMonitorCommand(
			monitorConfig(), &doubles.InMemoryAccountsRepository{Store: &entities.Store{}},
			&doubles.StubCredentialsRepository{}, &doubles.StubUsageRepository{},
			&doubles.StubTokensRepository{}, &doubles.StubSessionsRepository{Active: false},
		).WithPool(codex)

		// when
		err := monitor.Tick(time.Now())

		// then
		require.NoError(t, err)
		codexCredentials, ok := codex.Credentials.(*doubles.StubCredentialsRepository)
		require.True(t, ok)
		assert.Zero(t, codexCredentials.WriteCalls, "a live codex session must not be swapped underneath")
		codexAccounts, ok := codex.Accounts.(*doubles.InMemoryAccountsRepository)
		require.True(t, ok)
		assert.Equal(t, "y@example.com", codexAccounts.Store.Rotation.CurrentEmail,
			"the switch is recorded for the next launch")
	})

	t.Run("should not refresh live Codex tokens on a tick", func(t *testing.T) {
		t.Parallel()
		// given: a tick attends at once to a backup whose credentials are degraded, which
		// a Codex sign-in -- naming no Claude scope -- must never count as
		codex := exhaustedCodexPool()
		tokens := &doubles.StubTokensRepository{}
		codex.Tokens = tokens
		monitor := commands.NewMonitorCommand(
			monitorConfig(), &doubles.InMemoryAccountsRepository{Store: &entities.Store{}},
			&doubles.StubCredentialsRepository{}, &doubles.StubUsageRepository{},
			&doubles.StubTokensRepository{}, &doubles.StubSessionsRepository{},
		).WithPool(codex)

		// when
		err := monitor.Tick(time.Now())

		// then
		require.NoError(t, err)
		assert.Zero(t, tokens.RefreshCalls)
	})
}

func TestStatusNamesEachProvider(t *testing.T) {
	t.Parallel()

	t.Run("should print each provider's active account under its name once Codex accounts are enrolled",
		func(t *testing.T) {
			t.Parallel()
			// given
			reset := time.Now().Add(monthlyResetIn)
			codex := codexPool(codexStore(), &doubles.StubUsageRepository{
				Usage: codexUsage(fullPct, reset),
			})
			var out bytes.Buffer

			// when
			err := commands.NewStatusCommand(
				monitorConfig(), &doubles.InMemoryAccountsRepository{Store: livePairStore()},
				&doubles.StubCredentialsRepository{}, &doubles.StubUsageRepository{Usage: healthyUsage()},
				&doubles.StubTokensRepository{},
			).WithPool(codex).WithOutput(&out).Execute()

			// then
			require.NoError(t, err)
			assert.Contains(t, out.String(), "Claude\ncurrent account: a@example.com\n  5-hour: ")
			assert.Contains(
				t,
				out.String(),
				"Codex\ncurrent account: x@example.com\n  30-day:  100%, resets in 29d 18h",
			)
			assert.Contains(t, out.String(), "  binding: 30-day 100%, resets in 29d 18h",
				"Codex reports no severity band, so none is printed in parentheses")
			_, codexBlock, found := strings.Cut(out.String(), "Codex\n")
			require.True(t, found)
			assert.NotContains(t, codexBlock, "5-hour:", "a free Codex plan has no 5-hour window to report")
			assert.NotContains(t, codexBlock, "7-day:", "a free Codex plan has no 7-day window to report")
		})

	t.Run("should print a store of Claude accounts alone exactly as it did before Codex", func(t *testing.T) {
		t.Parallel()
		// given
		empty := codexPool(&entities.Store{Provider: entities.ProviderCodex}, &doubles.StubUsageRepository{})
		var single, both bytes.Buffer
		require.NoError(t, commands.NewStatusCommand(
			monitorConfig(), &doubles.InMemoryAccountsRepository{Store: livePairStore()},
			&doubles.StubCredentialsRepository{}, &doubles.StubUsageRepository{Usage: healthyUsage()},
			&doubles.StubTokensRepository{},
		).WithOutput(&single).Execute())

		// when
		err := commands.NewStatusCommand(
			monitorConfig(), &doubles.InMemoryAccountsRepository{Store: livePairStore()},
			&doubles.StubCredentialsRepository{}, &doubles.StubUsageRepository{Usage: healthyUsage()},
			&doubles.StubTokensRepository{},
		).WithPool(empty).WithOutput(&both).Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, single.String(), both.String())
	})
}

func TestSetThresholdAppliesToEveryProvider(t *testing.T) {
	t.Parallel()

	t.Run("should re-apply the rotation policy to each provider's accounts", func(t *testing.T) {
		t.Parallel()
		// given
		codex := exhaustedCodexPool()
		accounts := &doubles.InMemoryAccountsRepository{Store: livePairStore()}
		command := commands.NewSetThresholdCommand(
			monitorConfig(), accounts, &doubles.StubCredentialsRepository{},
			perAccountUsage(map[string]*entities.Usage{"a": healthyUsage(), "b": healthyUsage()}),
			&doubles.StubTokensRepository{}, &doubles.StubSessionsRepository{},
		).WithPool(codex)

		// when
		err := command.Execute(thresholdPct)

		// then
		require.NoError(t, err)
		codexAccounts, ok := codex.Accounts.(*doubles.InMemoryAccountsRepository)
		require.True(t, ok)
		assert.Equal(t, "y@example.com", codexAccounts.Store.Rotation.CurrentEmail)
		require.NotNil(t, accounts.Store.Settings.Threshold, "the threshold is kept in the first pool's store")
		assert.InDelta(t, thresholdPct, *accounts.Store.Settings.Threshold, 0)
	})

	t.Run("should apply the threshold to Codex's accounts when only Codex has any", func(t *testing.T) {
		t.Parallel()
		// given
		codex := exhaustedCodexPool()
		command := commands.NewSetThresholdCommand(
			monitorConfig(), &doubles.InMemoryAccountsRepository{Store: &entities.Store{}},
			&doubles.StubCredentialsRepository{}, &doubles.StubUsageRepository{},
			&doubles.StubTokensRepository{}, &doubles.StubSessionsRepository{},
		).WithPool(codex)

		// when
		err := command.Execute(thresholdPct)

		// then
		require.NoError(t, err)
		usage, ok := codex.Usage.(*doubles.StubUsageRepository)
		require.True(t, ok)
		assert.Positive(t, usage.FetchCalls, "an empty first store must not keep the others from being repolled")
	})
}

func TestGetAccountNamesTheProvider(t *testing.T) {
	t.Parallel()

	t.Run("should name the provider of a Codex account", func(t *testing.T) {
		t.Parallel()
		// given
		store := codexStore()
		store.Accounts[0].Credentials.SubscriptionType = "plus"
		var out bytes.Buffer

		// when
		err := commands.NewGetAccountCommand(
			monitorConfig(), &doubles.InMemoryAccountsRepository{Store: store}, &doubles.StubCredentialsRepository{},
			&doubles.StubUsageRepository{Usage: codexUsage(lowPct, time.Now().Add(monthlyResetIn))},
			&doubles.StubTokensRepository{},
		).WithOutput(&out).Execute("x@example.com")

		// then
		require.NoError(t, err)
		assert.Contains(t, out.String(), "x@example.com (active)\n  provider:       Codex\n  priority:       1 of 2")
		assert.Contains(t, out.String(), "  plan:           plus\n")
		assert.NotContains(t, out.String(), "warning:", "a Codex sign-in names no Claude scope, and needs none")
	})

	t.Run("should leave the provider out of a Claude account's plain output", func(t *testing.T) {
		t.Parallel()
		// given / when
		out, err := showOutput(monitorConfig(), livePairStore(),
			&doubles.StubUsageRepository{Usage: healthyUsage()}, "a@example.com")

		// then
		require.NoError(t, err)
		assert.NotContains(t, out, "provider:")
	})

	t.Run("should mark the provider with its logo in its brand color when output has colors", func(t *testing.T) {
		t.Parallel()
		// given / when
		out, err := showOutput(styledConfig(entities.OutputColor), livePairStore(),
			&doubles.StubUsageRepository{Usage: healthyUsage()}, "a@example.com")

		// then
		require.NoError(t, err)
		assert.Contains(t, out, "  provider:       "+sgr("91", "✻ Claude")+"\n")
	})
}

func TestEnrollAccountForCodex(t *testing.T) {
	t.Parallel()

	t.Run("should enroll the ChatGPT account Codex is signed in to", func(t *testing.T) {
		t.Parallel()
		// given
		signIn := codexCreds("x", "rx")
		accounts := &doubles.InMemoryAccountsRepository{Store: &entities.Store{Provider: entities.ProviderCodex}}
		credentials := &doubles.StubCredentialsRepository{
			Creds:    &signIn,
			Identity: &entities.AccountIdentity{EmailAddress: "x@example.com", AccountUUID: "workspace"},
		}

		// when
		err := commands.NewEnrollAccountCommand(accounts, credentials).Execute("", "", nil)

		// then
		require.NoError(t, err)
		account := accounts.Store.FindAccount("x@example.com")
		require.NotNil(t, account)
		assert.Equal(t, "id-x", account.Credentials.IDToken, "Codex cannot load a sign-in without its ID token")
		assert.Equal(t, "workspace", account.AccountUUID)
		assert.False(t, account.LongLived)
	})

	t.Run("should refuse a long-lived token, which Codex does not mint", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: &entities.Store{Provider: entities.ProviderCodex}}

		// when
		err := commands.NewEnrollAccountCommand(accounts, &doubles.StubCredentialsRepository{}).
			Execute("fixture-token-placeholder", "x@example.com", nil)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "`codex login`")
		assert.Zero(t, accounts.SaveCalls)
	})

	t.Run("should point at codex login when Codex's sign-in cannot be read", func(t *testing.T) {
		t.Parallel()
		// given
		accounts := &doubles.InMemoryAccountsRepository{Store: &entities.Store{Provider: entities.ProviderCodex}}
		credentials := &doubles.StubCredentialsRepository{ReadErr: errStoreUnreadable}

		// when
		err := commands.NewEnrollAccountCommand(accounts, credentials).Execute("", "", nil)

		// then
		require.ErrorIs(t, err, errStoreUnreadable)
		assert.Contains(t, err.Error(), "failed to read current Codex credentials (run `codex login` first)")
	})
}
