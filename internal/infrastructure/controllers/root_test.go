package controllers_test

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/controllers"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

func TestRootCommandUpdateCheck(t *testing.T) {
	t.Parallel()

	t.Run("should check for a newer release when an interactive command runs", func(t *testing.T) {
		t.Parallel()
		// given
		updates := &doubles.StubSelfUpdateRepository{}
		path := filepath.Join(t.TempDir(), "store.json")

		// when
		err := executeCLIWith(path, updates, io.Discard, io.Discard, "list")

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, updates.CheckCalls)
	})

	// Each of these is about the version itself, belongs to cobra, or is run
	// unattended by the shell integration, so a notice from it would go unread.
	for _, args := range [][]string{
		{"version"},
		{"self-update", "--dry-run"},
		{"ensure", "--quiet"},
		{"help"},
		{"completion", "bash"},
		{cobra.ShellCompRequestCmd, "li"},
	} {
		t.Run("should not check for a newer release when running "+strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			// given
			updates := &doubles.StubSelfUpdateRepository{}
			path := filepath.Join(t.TempDir(), "store.json")

			// when
			err := executeCLIWith(path, updates, io.Discard, io.Discard, args...)

			// then
			require.NoError(t, err)
			assert.Zero(t, updates.CheckCalls)
		})
	}

	t.Run("should not check for a newer release when the monitor runs", func(t *testing.T) {
		t.Parallel()
		// given -- running the monitor would start its loop, so the hook every command
		// runs ahead of its own body is invoked on it directly
		updates := &doubles.StubSelfUpdateRepository{}
		root := controllers.NewRootCommandWithUpdates("test", updates)
		monitor, _, err := root.Find([]string{"monitor"})
		require.NoError(t, err)

		// when
		root.PersistentPreRun(monitor, nil)

		// then
		assert.Zero(t, updates.CheckCalls)
	})
}

func TestRootCommandVersion(t *testing.T) {
	t.Parallel()

	t.Run("should print the labelled version for the --version flag", func(t *testing.T) {
		t.Parallel()
		// given
		var out bytes.Buffer
		path := filepath.Join(t.TempDir(), "store.json")

		// when
		err := executeCLIWith(path, &doubles.StubSelfUpdateRepository{}, &out, io.Discard, "--version")

		// then
		require.NoError(t, err)
		assert.Equal(t, "ccswitch version test\n", out.String())
	})

	t.Run("should print the bare version for the version command", func(t *testing.T) {
		t.Parallel()
		// given
		var out bytes.Buffer
		path := filepath.Join(t.TempDir(), "store.json")

		// when
		err := executeCLIWith(path, &doubles.StubSelfUpdateRepository{}, &out, io.Discard, "version")

		// then
		require.NoError(t, err)
		assert.Equal(t, "test\n", out.String())
	})

	t.Run("should print the version through the production release channel", func(t *testing.T) {
		t.Parallel()
		// given -- version never checks for a newer release, so nothing reaches GitHub
		var out bytes.Buffer
		root := controllers.NewRootCommand("test")
		root.SetOut(&out)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"version"})

		// when
		err := root.Execute()

		// then
		require.NoError(t, err)
		assert.Equal(t, "test\n", out.String())
	})
}

// TestRootCommandVerbose is deliberately not parallel: the log level it asserts on
// is process-wide, so it is restored afterwards and must not change underneath
// other tests while they run.
func TestRootCommandVerbose(t *testing.T) {
	t.Run("should turn on debug logging for -v", func(t *testing.T) {
		// given
		previous := logger.GetLevel()
		t.Cleanup(func() { logger.SetLevel(previous) })
		logger.SetLevel(logger.InfoLevel)
		path := filepath.Join(t.TempDir(), "store.json")

		// when
		err := executeCLIWith(path, &doubles.StubSelfUpdateRepository{}, io.Discard, io.Discard, "-v", "version")

		// then
		require.NoError(t, err)
		assert.Equal(t, logger.DebugLevel, logger.GetLevel())
	})
}

func TestDaemonArgs(t *testing.T) {
	t.Parallel()

	t.Run("should pass --verbose on to the daemon when the invocation asked for it", func(t *testing.T) {
		t.Parallel()
		// given
		cfg := &entities.Config{Interval: time.Minute, Verbose: true}

		// when
		args := controllers.DaemonArgs(cfg)

		// then
		assert.Equal(t, "monitor", args[0])
		assert.Contains(t, args, "--verbose")
	})

	t.Run("should leave --verbose out when the invocation did not ask for it", func(t *testing.T) {
		t.Parallel()
		// given
		cfg := &entities.Config{Interval: time.Minute}

		// when
		args := controllers.DaemonArgs(cfg)

		// then
		assert.NotContains(t, args, "--verbose")
	})
}
