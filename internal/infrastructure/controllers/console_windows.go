//go:build windows

package controllers

import (
	"os"

	"golang.org/x/sys/windows"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// consoleStyle makes the console that file writes to render the colors of the
// style. Windows Terminal turns virtual terminal processing on for every program,
// but the classic console window prints ANSI escape sequences as text until a
// program switches it on, and nothing does that for the listing: logrus switches
// the console only once it logs, which a listing may never do. A console that
// refuses, one older than Windows 10, keeps the meters and loses the colors it
// would otherwise print as raw codes. Output that is not a console, such as a pipe
// into a pager, is left as it is.
func consoleStyle(file *os.File, style entities.OutputStyle) entities.OutputStyle {
	if !style.Colored() {
		return style
	}
	handle := windows.Handle(file.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return style
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return style
	}
	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return entities.OutputMonochrome
	}
	return style
}
