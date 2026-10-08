package entities

// OutputStyle is how richly a command may dress up what it prints to standard
// output. The controllers resolve it from where that output goes and from the
// color settings the user gave, and it reaches the commands through Config.
type OutputStyle int

const (
	// OutputPlain prints the text alone, the same on every run. It is what a pipe or
	// a file gets, since whatever reads it there may be a script.
	OutputPlain OutputStyle = iota
	// OutputMonochrome draws a meter beside every utilization figure and lines the
	// reset times up in columns, without colors: a terminal whose user asked for
	// none.
	OutputMonochrome
	// OutputColor draws the meters and columns of OutputMonochrome and colors
	// states, figures and reset times by what they say.
	OutputColor
)

// Decorated reports whether the style draws meters and columns, which every style
// but the plain one does.
func (s OutputStyle) Decorated() bool {
	return s != OutputPlain
}

// Colored reports whether the style marks text up with ANSI colors.
func (s OutputStyle) Colored() bool {
	return s == OutputColor
}
