//go:build !windows

package controllers

import (
	"os"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// consoleStyle returns the style as it is: terminals outside Windows render ANSI
// escape sequences as they come.
func consoleStyle(_ *os.File, style entities.OutputStyle) entities.OutputStyle {
	return style
}
