package repositories_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	domain "github.com/rios0rios0/ccswitch/internal/domain/repositories"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
	"github.com/rios0rios0/ccswitch/test/builders"
)

const (
	// freePlanUsage is the shape the endpoint answers for a free plan that has spent
	// its single, 30-day window, as observed from Codex's own usage endpoint.
	freePlanUsage = `{
  "plan_type": "free",
  "rate_limit": {
    "allowed": false,
    "limit_reached": true,
    "primary_window": {"used_percent": 100, "limit_window_seconds": 2592000,
      "reset_after_seconds": 2570621, "reset_at": %d},
    "secondary_window": null
  },
  "code_review_rate_limit": null,
  "additional_rate_limits": null,
  "credits": {"has_credits": false, "unlimited": false, "balance": null},
  "rate_limit_reached_type": {"type": "rate_limit_reached"}
}`
	// paidPlanUsage is a paid plan with a 5-hour and a 7-day window, a limit on code
	// reviews, and a limit on one model.
	paidPlanUsage = `{
  "plan_type": "plus",
  "rate_limit": {
    "allowed": true,
    "limit_reached": false,
    "primary_window": {"used_percent": 12, "limit_window_seconds": 18000,
      "reset_after_seconds": 7980, "reset_at": %[1]d},
    "secondary_window": {"used_percent": 40, "limit_window_seconds": 604800,
      "reset_after_seconds": 352800, "reset_at": %[2]d}
  },
  "code_review_rate_limit": {
    "allowed": true,
    "limit_reached": false,
    "primary_window": {"used_percent": 0, "limit_window_seconds": 604800,
      "reset_after_seconds": 352800, "reset_at": %[2]d}
  },
  "additional_rate_limits": [{
    "limit_name": "GPT-5.3-Codex-Spark",
    "metered_feature": "codex_bengalfox",
    "rate_limit": {
      "allowed": false,
      "limit_reached": true,
      "primary_window": {"used_percent": 100, "limit_window_seconds": 18000,
        "reset_after_seconds": 7980, "reset_at": %[1]d}
    }
  }]
}`
	freePlanUsedPct    = 100.0
	paidSessionUsedPct = 12.0
	paidWeeklyUsedPct  = 40.0
	codexThreshold     = 99.0
	// windowResetIn is how far out a test response puts a window's reset.
	windowResetIn = 2 * time.Hour
)

// serveCodexUsage starts a server that answers the Codex usage endpoint with the body,
// and hands every request it receives to inspect.
func serveCodexUsage(t *testing.T, body string, inspect func(*http.Request)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inspect(r)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestOpenAIUsageRepositoryFetch(t *testing.T) {
	t.Parallel()

	t.Run("should read a free plan's single 30-day window and the refusal that comes with spending it",
		func(t *testing.T) {
			t.Parallel()
			// given
			reset := time.Now().Add(windowResetIn).Truncate(time.Second)
			server := serveCodexUsage(t, fmt.Sprintf(freePlanUsage, reset.Unix()), func(*http.Request) {})
			repo := repositories.NewOpenAIUsageRepository(server.URL, server.Client())

			// when
			usage, err := repo.Fetch("fixture-access-placeholder")

			// then
			require.NoError(t, err)
			assert.True(t, usage.LimitsOnly, "a free plan has no 5-hour or 7-day window to report")
			assert.True(t, usage.Blocked)
			require.Len(t, usage.Limits, 1)
			assert.Equal(t, entities.Limit{
				Kind: "30-day", Percent: freePlanUsedPct, IsActive: true, ResetsAt: reset,
			}, usage.Limits[0])
			assert.True(t, usage.Exhausted(codexThreshold))
			assert.Equal(t, reset, usage.RecoversAt(codexThreshold))
		})

	t.Run("should read a paid plan's windows and list the limits that cannot rotate the account",
		func(t *testing.T) {
			t.Parallel()
			// given
			sessionReset := time.Now().Add(windowResetIn).Truncate(time.Second)
			weeklyReset := sessionReset.Add(4 * 24 * time.Hour)
			server := serveCodexUsage(t, fmt.Sprintf(paidPlanUsage, sessionReset.Unix(), weeklyReset.Unix()),
				func(*http.Request) {})
			repo := repositories.NewOpenAIUsageRepository(server.URL, server.Client())

			// when
			usage, err := repo.Fetch("fixture-access-placeholder")

			// then
			require.NoError(t, err)
			assert.False(t, usage.Blocked)
			kinds := make([]string, 0, len(usage.Limits))
			for _, limit := range usage.Limits {
				kinds = append(kinds, limit.Kind)
			}
			assert.Equal(t, []string{"5-hour", "7-day", "7-day review", "5-hour GPT-5.3-Codex-Spark"}, kinds)
			assert.True(t, usage.Limits[0].IsActive)
			assert.True(t, usage.Limits[1].IsActive)
			assert.False(t, usage.Limits[2].IsActive, "a code review limit leaves every other request served")
			assert.False(t, usage.Limits[3].IsActive, "a limit on one model leaves the others served")
			assert.InDelta(t, paidSessionUsedPct, usage.Limits[0].Percent, 0)
			assert.InDelta(t, paidWeeklyUsedPct, usage.Limits[1].Percent, 0)
			assert.Equal(t, weeklyReset, usage.Limits[1].ResetsAt)
			assert.False(t, usage.Exhausted(codexThreshold),
				"the spent model limit must not rotate an account whose main limit has room")
		})

	t.Run("should call the usage endpoint as the ChatGPT account the token acts for", func(t *testing.T) {
		t.Parallel()
		// given
		token := builders.NewOpenAITokenBuilder().WithAccountID(codexAccountID).Build()
		seen := make(chan *http.Request, 1)
		server := serveCodexUsage(t, `{"plan_type":"plus"}`, func(r *http.Request) { seen <- r })
		repo := repositories.NewOpenAIUsageRepository(server.URL, server.Client())

		// when
		_, err := repo.Fetch(token)

		// then
		require.NoError(t, err)
		request := <-seen
		assert.Equal(t, "/wham/usage", request.URL.Path)
		assert.Equal(t, "Bearer "+token, request.Header.Get("Authorization"))
		assert.Equal(t, codexAccountID, request.Header.Get("Chatgpt-Account-Id"))
	})

	t.Run("should not read a refusal into a response that leaves it out", func(t *testing.T) {
		t.Parallel()
		// given
		server := serveCodexUsage(t, `{"plan_type":"plus","rate_limit":{"limit_reached":false}}`,
			func(*http.Request) {})
		repo := repositories.NewOpenAIUsageRepository(server.URL, server.Client())

		// when
		usage, err := repo.Fetch("fixture-access-placeholder")

		// then
		require.NoError(t, err)
		assert.False(t, usage.Blocked)
	})

	t.Run("should count a reset out from now when the endpoint names no moment for it", func(t *testing.T) {
		t.Parallel()
		// given
		before := time.Now()
		server := serveCodexUsage(t, `{"plan_type":"plus","rate_limit":{"allowed":true,
			"primary_window":{"used_percent":5,"limit_window_seconds":18000,"reset_after_seconds":3600}}}`,
			func(*http.Request) {})
		repo := repositories.NewOpenAIUsageRepository(server.URL, server.Client())

		// when
		usage, err := repo.Fetch("fixture-access-placeholder")

		// then
		require.NoError(t, err)
		require.Len(t, usage.Limits, 1)
		assert.WithinRange(t, usage.Limits[0].ResetsAt, before.Add(time.Hour), time.Now().Add(time.Hour))
	})

	t.Run("should name a window by its largest whole unit, and one of unknown length plainly", func(t *testing.T) {
		t.Parallel()
		// given: a 90-minute window, a 61-second one that rounds up to the minute as Codex
		// rounds it, and one the endpoint gave no length
		server := serveCodexUsage(t, `{"plan_type":"plus",
			"rate_limit":{"primary_window":{"used_percent":1,"limit_window_seconds":5400},
				"secondary_window":{"used_percent":2,"limit_window_seconds":61}},
			"code_review_rate_limit":{"primary_window":{"used_percent":3,"limit_window_seconds":0}}}`,
			func(*http.Request) {})
		repo := repositories.NewOpenAIUsageRepository(server.URL, server.Client())

		// when
		usage, err := repo.Fetch("fixture-access-placeholder")

		// then
		require.NoError(t, err)
		require.Len(t, usage.Limits, 3)
		assert.Equal(t, "90-minute", usage.Limits[0].Kind)
		assert.Equal(t, "2-minute", usage.Limits[1].Kind)
		assert.Equal(t, "window review", usage.Limits[2].Kind)
		assert.True(t, usage.Limits[0].ResetsAt.IsZero(), "a window that names no reset has none")
	})

	t.Run("should report a rejected token as ErrUnauthorized", func(t *testing.T) {
		t.Parallel()
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()
		repo := repositories.NewOpenAIUsageRepository(server.URL, server.Client())

		// when
		_, err := repo.Fetch("fixture-access-placeholder")

		// then
		require.ErrorIs(t, err, domain.ErrUnauthorized,
			"callers refresh and retry on this error; a plain error strands the account")
	})

	t.Run("should not report other failures as ErrUnauthorized", func(t *testing.T) {
		t.Parallel()
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		repo := repositories.NewOpenAIUsageRepository(server.URL, server.Client())

		// when
		_, err := repo.Fetch("fixture-access-placeholder")

		// then
		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrUnauthorized,
			"refreshing on a server-side failure burns the refresh token for nothing")
	})
}
