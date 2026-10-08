package controllers

import (
	"fmt"
	"os"
	"slices"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// colorFlag names the flag that says when the output may carry colors.
const colorFlag = "color"

// The environment variables that turn colors on or off. NO_COLOR (no-color.org)
// and FORCE_COLOR (force-color.org) count once set to anything but the empty
// string, while CLICOLOR and CLICOLOR_FORCE (bixense.com/clicolors) are read for
// their value.
const (
	envNoColor       = "NO_COLOR"
	envForceColor    = "FORCE_COLOR"
	envCliColor      = "CLICOLOR"
	envCliColorForce = "CLICOLOR_FORCE"
	envTerm          = "TERM"
)

// colorMode is when the output may carry colors, as --color names it.
type colorMode string

const (
	// colorAuto colors a terminal, unless the environment asks otherwise.
	colorAuto colorMode = "auto"
	// colorAlways colors the output wherever it goes, a pipe included, for a pager
	// that renders the escape codes, such as `less -R`.
	colorAlways colorMode = "always"
	// colorNever colors nothing. A terminal still gets the meters and columns.
	colorNever colorMode = "never"
)

// String reports the mode, as pflag.Value requires.
func (m *colorMode) String() string {
	return string(*m)
}

// Set takes the mode from the command line, rejecting one it does not know, which
// cobra reports while it parses the flags.
func (m *colorMode) Set(value string) error {
	mode := colorMode(value)
	if !slices.Contains([]colorMode{colorAuto, colorAlways, colorNever}, mode) {
		return fmt.Errorf("must be one of %s, %s or %s", colorAuto, colorAlways, colorNever)
	}
	*m = mode
	return nil
}

// Type names the value --color takes, for the help text.
func (m *colorMode) Type() string {
	return "when"
}

// outputStyle resolves how richly the usage listing dresses up standard output,
// from --color, the environment, and whether the output is a terminal. A console
// that cannot be made to render colors keeps the rest (see consoleStyle).
func outputStyle(mode colorMode, stdout *os.File) entities.OutputStyle {
	return consoleStyle(stdout, resolveOutputStyle(mode, os.Getenv, isTerminal(stdout)))
}

// resolveOutputStyle decides how richly to dress up output, given the color mode,
// a reader of the environment, and whether the output is a terminal. Only a
// terminal gets meters and columns, unless colors are forced: a pipe or a file
// gets the plain text a script can read, the same on every run.
func resolveOutputStyle(mode colorMode, getenv func(string) string, terminal bool) entities.OutputStyle {
	switch {
	case wantsColors(mode, getenv, terminal):
		return entities.OutputColor
	case terminal:
		return entities.OutputMonochrome
	default:
		return entities.OutputPlain
	}
}

// wantsColors decides whether to color the output. --color always or never
// settles it. Otherwise the first of these that applies does: FORCE_COLOR, which
// forces colors unless it is "0" or "false", which turn them off; NO_COLOR, which
// turns them off; CLICOLOR_FORCE other than "0", which forces them; CLICOLOR=0 or
// TERM=dumb, which turn them off; and last, whether the output is a terminal.
//
// Each variable ranks the way its own specification has it. FORCE_COLOR outranks
// NO_COLOR, as in the reference code of force-color.org and in Node.js: it is
// usually set for the one command it comes with, such as a pipe into a pager,
// while NO_COLOR is a standing preference. CLICOLOR_FORCE yields to NO_COLOR, as
// bixense.com/clicolors specifies.
func wantsColors(mode colorMode, getenv func(string) string, terminal bool) bool {
	if mode != colorAuto {
		return mode == colorAlways
	}
	if force := getenv(envForceColor); force != "" {
		return force != "0" && force != "false"
	}
	if getenv(envNoColor) != "" {
		return false
	}
	if force := getenv(envCliColorForce); force != "" && force != "0" {
		return true
	}
	if getenv(envCliColor) == "0" || getenv(envTerm) == "dumb" {
		return false
	}
	return terminal
}

// isTerminal reports whether the file is a terminal, which is a character device.
// The devices that discard what is written to them, /dev/null and NUL, are
// character devices as well, so output sent there is dressed up for nobody.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
