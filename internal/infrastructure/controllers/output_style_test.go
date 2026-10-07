package controllers_test

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
	"github.com/rios0rios0/ccswitch/internal/infrastructure/controllers"
	"github.com/rios0rios0/ccswitch/test/doubles"
)

// outputStyleCase is one way of asking for colors, and the style it resolves to.
type outputStyleCase struct {
	name     string
	mode     string
	env      map[string]string
	terminal bool
	want     entities.OutputStyle
}

func TestResolveOutputStyle(t *testing.T) {
	t.Parallel()

	for _, scenario := range []outputStyleCase{
		{"should color a terminal when nothing says otherwise", "auto", nil, true, entities.OutputColor},
		{"should print plain text when output is not a terminal", "auto", nil, false, entities.OutputPlain},
		{
			"should keep the meters but drop the colors when NO_COLOR is set",
			"auto", map[string]string{"NO_COLOR": "1"}, true, entities.OutputMonochrome,
		},
		{
			"should keep the meters but drop the colors when CLICOLOR is 0",
			"auto", map[string]string{"CLICOLOR": "0"}, true, entities.OutputMonochrome,
		},
		{
			"should keep the meters but drop the colors when TERM is dumb",
			"auto", map[string]string{"TERM": "dumb"}, true, entities.OutputMonochrome,
		},
		{
			"should color a pipe when FORCE_COLOR is set",
			"auto", map[string]string{"FORCE_COLOR": "1"}, false, entities.OutputColor,
		},
		{
			"should color a pipe when CLICOLOR_FORCE is set",
			"auto", map[string]string{"CLICOLOR_FORCE": "1"}, false, entities.OutputColor,
		},
		{
			"should not color a pipe when CLICOLOR_FORCE is 0",
			"auto", map[string]string{"CLICOLOR_FORCE": "0"}, false, entities.OutputPlain,
		},
		{
			"should drop the colors when FORCE_COLOR is 0",
			"auto", map[string]string{"FORCE_COLOR": "0"}, true, entities.OutputMonochrome,
		},
		{
			"should color when FORCE_COLOR and NO_COLOR are both set",
			"auto", map[string]string{"FORCE_COLOR": "1", "NO_COLOR": "1"}, true, entities.OutputColor,
		},
		{
			"should keep the meters but drop the colors when NO_COLOR is set even though CLICOLOR_FORCE is",
			"auto", map[string]string{"NO_COLOR": "1", "CLICOLOR_FORCE": "1"}, true, entities.OutputMonochrome,
		},
		{
			"should color despite TERM=dumb when CLICOLOR_FORCE is set",
			"auto", map[string]string{"CLICOLOR_FORCE": "1", "TERM": "dumb"}, true, entities.OutputColor,
		},
		{
			"should color a pipe despite NO_COLOR when --color is always",
			"always", map[string]string{"NO_COLOR": "1"}, false, entities.OutputColor,
		},
		{
			"should keep the meters but drop the colors despite FORCE_COLOR when --color is never",
			"never", map[string]string{"FORCE_COLOR": "1"}, true, entities.OutputMonochrome,
		},
		{"should print plain text to a pipe when --color is never", "never", nil, false, entities.OutputPlain},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			// given
			mode, env, terminal := scenario.mode, scenario.env, scenario.terminal

			// when
			style := controllers.ResolveOutputStyle(mode, env, terminal)

			// then
			assert.Equal(t, scenario.want, style)
		})
	}
}

func TestColorFlag(t *testing.T) {
	t.Parallel()

	t.Run("should reject a color mode it does not know", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")

		// when
		err := executeCLIWith(path, &doubles.StubSelfUpdateRepository{}, io.Discard, io.Discard,
			"list", "--color", "sometimes")

		// then
		require.Error(t, err)
		assert.ErrorContains(t, err, "must be one of auto, always or never")
	})

	t.Run("should accept each color mode it knows", func(t *testing.T) {
		t.Parallel()
		// given
		path := filepath.Join(t.TempDir(), "store.json")

		// when
		errs := []error{
			executeCLI(path, "list", "--color", "auto"),
			executeCLI(path, "list", "--color", "always"),
			executeCLI(path, "list", "--color=never"),
		}

		// then
		for _, err := range errs {
			require.NoError(t, err)
		}
	})
}
