package main

import (
	"os"
	"regexp"
	"strings"
)

const (
	colorReset   = "\x1b[0m"
	colorWall    = "\x1b[90m"   // dim gray - recedes into the background
	colorPath    = "\x1b[92m"   // bright green
	colorStart   = "\x1b[1;92m" // bold bright green
	colorKey     = "\x1b[1;93m" // bold bright yellow (gold, for the treasure)
	colorExit    = "\x1b[1;91m" // bold bright red
	colorVisited = "\x1b[31m"   // plain (not bold/bright) red - deliberately duller than Exit's red
)

// teleporterPalette cycles distinct colors per teleporter pair, so both
// ends of the same pair are visually linked to each other (not just by
// matching letter case) and distinguishable from every other pair.
var teleporterPalette = []string{
	"\x1b[1;96m", // bright cyan
	"\x1b[1;95m", // bright magenta
	"\x1b[1;94m", // bright blue
	"\x1b[1;33m", // yellow
	"\x1b[1;36m", // cyan
	"\x1b[1;35m", // magenta
}

// colorsEnabled decides once at startup whether to emit ANSI codes at all:
// only when stdout is an actual terminal and the user hasn't opted out.
// Piping/redirecting to a file (as suggested for -bench output) naturally
// disables color, so redirected output never fills up with escape-code
// clutter.
var colorsEnabled = computeColorsEnabled()

func computeColorsEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTerminal(os.Stdout)
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func colorize(s, code string) string {
	if !colorsEnabled || code == "" {
		return s
	}
	return code + s + colorReset
}

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// visibleWidth measures the on-screen width of a string, ignoring any ANSI
// color codes it carries.
func visibleWidth(s string) int {
	return len([]rune(stripANSI(s)))
}

// padVisible right-pads s with spaces until its visible (non-ANSI) width
// reaches width. Safe to use on colored strings since padding is added
// after any trailing reset code.
func padVisible(s string, width int) string {
	w := visibleWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
