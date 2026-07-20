package repositories_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/infrastructure/repositories"
)

const usageBody = `{
  "five_hour": {"utilization": 56.0, "resets_at": "2026-07-17T23:29:59.668737+00:00"},
  "seven_day": {"utilization": 93.0, "resets_at": "2026-07-22T01:59:59.668756+00:00"},
  "limits": [
    {"kind":"session","group":"session","percent":56,"severity":"normal","is_active":false,"resets_at":"2026-07-17T23:29:59.668737+00:00"},
    {"kind":"weekly_scoped","group":"weekly","percent":100,"severity":"critical","is_active":true,"resets_at":"2026-07-22T01:59:59.669034+00:00"}
  ],
  "extra_usage": {"is_enabled": true}
}`

const (
	exhaustThreshold = 90.0
	wantFiveHour     = 56.0
	wantSevenDay     = 93.0
	usageEpsilon     = 0.001
)

func TestAnthropicUsageRepositoryFetch(t *testing.T) {
	t.Parallel()

	t.Run("should parse the usage response and detect exhaustion", func(t *testing.T) {
		t.Parallel()
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/oauth/usage", r.URL.Path)
			assert.Equal(t, "Bearer token-123", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(usageBody))
		}))
		defer server.Close()
		repo := repositories.NewAnthropicUsageRepository(server.URL, server.Client())

		// when
		usage, err := repo.Fetch("token-123")

		// then
		require.NoError(t, err)
		assert.InDelta(t, wantFiveHour, usage.FiveHour.Utilization, usageEpsilon)
		assert.InDelta(t, wantSevenDay, usage.SevenDay.Utilization, usageEpsilon)
		assert.True(t, usage.ExtraUsage)
		assert.True(t, usage.Exhausted(exhaustThreshold))
	})

	t.Run("should return an error on a non-200 response", func(t *testing.T) {
		t.Parallel()
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()
		repo := repositories.NewAnthropicUsageRepository(server.URL, server.Client())

		// when
		_, err := repo.Fetch("token")

		// then
		require.Error(t, err)
	})
}
