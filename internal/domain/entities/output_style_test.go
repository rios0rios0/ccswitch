package entities_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

func TestOutputStyle(t *testing.T) {
	t.Parallel()

	t.Run("should neither decorate nor color when the style is plain", func(t *testing.T) {
		t.Parallel()
		// given
		var style entities.OutputStyle

		// when
		decorated, colored := style.Decorated(), style.Colored()

		// then
		assert.Equal(t, entities.OutputPlain, style, "the zero value has to be the plain style")
		assert.False(t, decorated)
		assert.False(t, colored)
	})

	t.Run("should decorate without colors when the style is monochrome", func(t *testing.T) {
		t.Parallel()
		// given
		style := entities.OutputMonochrome

		// when
		decorated, colored := style.Decorated(), style.Colored()

		// then
		assert.True(t, decorated)
		assert.False(t, colored)
	})

	t.Run("should decorate and color when the style is color", func(t *testing.T) {
		t.Parallel()
		// given
		style := entities.OutputColor

		// when
		decorated, colored := style.Decorated(), style.Colored()

		// then
		assert.True(t, decorated)
		assert.True(t, colored)
	})
}
