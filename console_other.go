//go:build !windows

package main

// enableANSI is a no-op outside Windows - every other common terminal
// already interprets ANSI escape codes natively.
func enableANSI() {}

// consoleWidth reports 0 (unknown) outside Windows; the caller falls back
// to a sane default column width.
func consoleWidth() int { return 0 }
