package main

import (
	"fmt"
	"strings"
)

type panel struct {
	title string
	lines []string
}

// buildPanel turns one algorithm's result into a printable panel: the
// solved maze (or, if disqualified, its rejection reason) with a header
// line naming the algorithm and its steps/time.
func buildPanel(r runResult) panel {
	if !r.valid {
		return panel{
			title: fmt.Sprintf("%s - DISQUALIFIED", r.name),
			lines: []string{colorize(r.invalidReason, colorExit)},
		}
	}
	return panel{
		title: fmt.Sprintf("%s - %d steps, %s", r.name, r.steps, r.elapsed),
		lines: strings.Split(strings.TrimRight(r.rendered, "\n"), "\n"),
	}
}

const panelGap = 3

// printPanelsGrid prints a set of panels packed left-to-right into rows
// that fit within maxWidth, wrapping to additional rows of panels as
// needed - maximizing how much of the console's width gets used instead of
// dumping every maze one after another down a single column.
func printPanelsGrid(panels []panel, maxWidth int) {
	if len(panels) == 0 {
		return
	}

	panelWidth := 0
	for _, p := range panels {
		if w := visibleWidth(p.title); w > panelWidth {
			panelWidth = w
		}
		for _, l := range p.lines {
			if w := visibleWidth(l); w > panelWidth {
				panelWidth = w
			}
		}
	}

	perRow := (maxWidth + panelGap) / (panelWidth + panelGap)
	if perRow < 1 {
		perRow = 1
	}

	for start := 0; start < len(panels); start += perRow {
		end := start + perRow
		if end > len(panels) {
			end = len(panels)
		}
		row := panels[start:end]

		for _, p := range row {
			fmt.Print(padVisible(p.title, panelWidth), strings.Repeat(" ", panelGap))
		}
		fmt.Println()

		maxLines := 0
		for _, p := range row {
			if len(p.lines) > maxLines {
				maxLines = len(p.lines)
			}
		}
		for i := 0; i < maxLines; i++ {
			for _, p := range row {
				line := ""
				if i < len(p.lines) {
					line = p.lines[i]
				}
				fmt.Print(padVisible(line, panelWidth), strings.Repeat(" ", panelGap))
			}
			fmt.Println()
		}
		fmt.Println()
	}
}

// resolveConsoleWidth picks the width to pack panels into: an explicit
// override, else the detected console width, else a sane default for
// piped/redirected output.
func resolveConsoleWidth(override int) int {
	if override > 0 {
		return override
	}
	if w := consoleWidth(); w > 0 {
		return w
	}
	return 120
}
