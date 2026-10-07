package commands

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/rios0rios0/ccswitch/internal/domain/entities"
)

// tone is the SGR parameter list a stretch of colored text is printed with. Each
// is named for what the text says rather than for how it looks, and keeps to the
// sixteen colors a terminal's theme defines, so the listing stays readable on a
// light background as well as on a dark one. Only red is ever bold: Windows
// Terminal and xterm show bold text in the bright variant of its color by
// default, which washes green, yellow and cyan out on a light background but
// leaves red readable.
type tone string

const (
	// toneHealthy marks a figure well clear of the threshold, and an account that
	// has capacity: green.
	toneHealthy tone = "32"
	// toneNearing marks a figure closing in on the threshold, and a reading so old
	// that its limit has reset since: yellow.
	toneNearing tone = "33"
	// toneSpent marks a figure that has reached the threshold: red.
	toneSpent tone = "31"
	// toneExhausted marks an exhausted account, its email and its state: bold red.
	toneExhausted tone = "1;31"
	// toneTime marks how long until something resets: cyan.
	toneTime tone = "36"
	// toneManual marks an account that only a person can select: magenta.
	toneManual tone = "35"
	// toneActive marks the account Claude Code runs on: green, as git marks the
	// branch checked out.
	toneActive tone = "32"
	// toneStrong marks the threshold and the email of an account that is not
	// exhausted: bold.
	toneStrong tone = "1"
	// toneMuted marks what supports a figure rather than being one: dates,
	// positions, and the empty track of a meter. It is faint rather than "bright
	// black", which some themes, Solarized Dark among them, paint in the color of
	// the background.
	toneMuted tone = "2"
)

const (
	// sgrStart and sgrEnd enclose the parameters of the escape sequence that sets a
	// tone, and sgrReset closes every colored stretch so no tone leaks past it.
	sgrStart = "\x1b["
	sgrEnd   = "m"
	sgrReset = "\x1b[0m"

	// meterCells is how many terminal cells a meter spans, each standing for 5%.
	meterCells = 20
	// meterSteps is how finely a meter's last cell fills: block elements come in
	// eighths of a cell.
	meterSteps = 8
	// meterFill draws a used cell, meterPartials a cell filled from one to seven
	// eighths, and meterTrack an unused cell. A shaded track stays distinct from
	// the fill even on a terminal that does not render faint text.
	meterFill     = "█"
	meterPartials = "▏▎▍▌▋▊▉"
	meterTrack    = "░"
	// fullPercent is a figure at its limit, which fills a meter.
	fullPercent = 100.0
	// nearingShare is how much of the way to the threshold a figure has come once
	// it is shown as nearing it: from 80% of the threshold, a figure turns yellow
	// before it turns red.
	nearingShare = 0.8
	// resetColumnWidth fits the longest countdown a live reading shows, "resets in
	// 23h 59m", so that the dates after the countdowns line up in a column.
	resetColumnWidth = 17
	// columnGap separates a figure from its countdown, and a countdown from its
	// date.
	columnGap = "  "
)

// palette dresses up what a command prints in one output style. The color style
// paints text in tones, every decorated style draws meters and columns, and the
// plain style leaves each line exactly as it reads without a palette.
type palette struct {
	style entities.OutputStyle
}

// paint wraps text in the tone when the style has colors, and returns it as it is
// otherwise.
func (p palette) paint(color tone, text string) string {
	if !p.style.Colored() || text == "" {
		return text
	}
	return sgrStart + string(color) + sgrEnd + text + sgrReset
}

// heading starts an account's line: a marker on the account Claude Code runs on,
// the account's position in the rotation order, and its email in the given tone.
func (p palette) heading(position int, email string, active bool, color tone) string {
	marker := " "
	if active {
		marker = p.paint(toneActive, "*")
	}
	return marker + " " + p.paint(toneMuted, strconv.Itoa(position)+".") + " " + p.paint(color, email)
}

// state phrases whether an account can be used: "ok", "exhausted", or, when the
// moment it ends is known, "exhausted, available again in 31m (Tue Sep 29
// 16:05)".
func (p palette) state(account availability, now time.Time) string {
	if !account.exhausted {
		return p.paint(toneHealthy, stateOK)
	}
	exhausted := p.paint(toneExhausted, stateExhausted)
	if account.recovers.IsZero() {
		return exhausted
	}
	return exhausted + ", available again " + p.moment(now, account.recovers)
}

// readingLine renders one utilization figure: its label and percentage, then when
// it resets. A decorated style draws a meter between the label and the
// percentage, colors both by how close the figure stands to the threshold, and
// lines the countdown and the date up in columns of their own. It rounds the
// figure down, as Claude Code's own usage screen does, so that a figure never
// reads as the threshold before it has reached it and turned red. The plain style
// prints the line as it always has.
func (p palette) readingLine(reading entities.Reading, now time.Time, threshold float64) string {
	label := fmt.Sprintf("%-*s", readingLabelWidth, readingLabel(reading.Kind))
	if !p.style.Decorated() {
		line := fmt.Sprintf("%s%s %4.0f%%", readingIndent, label, reading.Percent)
		if !reading.ResetsAt.IsZero() {
			line += columnGap + describeReset(now, reading.ResetsAt)
		}
		return line
	}

	color := figureTone(reading, threshold)
	line := readingIndent + label + " " + p.meter(reading.Percent, color) + " " +
		p.paint(color, fmt.Sprintf("%3.0f%%", math.Floor(reading.Percent)))
	if !reading.ResetsAt.IsZero() {
		line += columnGap + p.reset(now, reading.ResetsAt)
	}
	return line
}

// meter draws a utilization percentage as a bar meterCells cells wide: filled in
// the given tone in whole eighths of a cell, rounded down like the figure beside
// it so that only a figure at its limit fills the bar, over a faint track for the
// rest. A figure past its limit fills the bar and no more.
func (p palette) meter(percent float64, color tone) string {
	clamped := min(max(percent, 0), fullPercent)
	// Multiplying before dividing keeps a whole-number figure exact: 60/100 lands a
	// hair under 0.6, and the floor would take an eighth off a 60% meter.
	eighths := int(math.Floor(clamped * meterCells * meterSteps / fullPercent))
	cells, remainder := eighths/meterSteps, eighths%meterSteps
	fill := strings.Repeat(meterFill, cells)
	if remainder > 0 {
		fill += string([]rune(meterPartials)[remainder-1])
		cells++
	}
	return p.paint(color, fill) + p.paint(toneMuted, strings.Repeat(meterTrack, meterCells-cells))
}

// reset phrases when a limit resets for a decorated line: "resets in 2h 13m",
// padded so that the date after it starts a column, or "reset 5m ago" for a
// moment already past, which only a stale reading shows.
func (p palette) reset(now, moment time.Time) string {
	verb, color := "resets", toneTime
	if !moment.After(now) {
		verb, color = "reset", toneNearing
	}
	phrase := countdown(now, moment)
	padding := strings.Repeat(" ", max(resetColumnWidth-len(verb+" "+phrase), 0))
	return p.paint(toneMuted, verb) + " " + p.paint(color, phrase) + padding + columnGap +
		p.paint(toneMuted, localMoment(moment))
}

// moment renders a moment relative to now and in local time: "in 2h 13m (Tue Sep
// 29 17:47)" ahead of now, "5m ago (Tue Sep 29 17:29)" behind it, or "unknown"
// for the zero time. The countdown is in the time tone and the date faint.
func (p palette) moment(now, moment time.Time) string {
	if moment.IsZero() {
		return "unknown"
	}
	return p.paint(toneTime, countdown(now, moment)) + " " + p.paint(toneMuted, "("+localMoment(moment)+")")
}

// figureTone colors a utilization figure by how close it stands to the rotation
// threshold: red once it has reached it, yellow from nearingShare of the way
// there, and green below that.
func figureTone(reading entities.Reading, threshold float64) tone {
	switch {
	case reading.Spent(threshold):
		return toneSpent
	case reading.Percent >= threshold*nearingShare:
		return toneNearing
	default:
		return toneHealthy
	}
}
