package main

import "strings"

// buildGrid renders the maze into a 2D grid of terminal cells (each one a
// single rune, optionally wrapped in ANSI color codes). Every maze cell
// becomes a 2x2 block in the output so that the wall/passage between two
// cells is its own character position:
//
//	████████
//	█S  █   █
//	██ ██████
//	█    K  █
//	████████
//
// path is nil for the plain unsolved maze, or the found route to overlay.
// visited (optional) marks cells the algorithm's search touched but that
// aren't on path - drawn as a red dot, always beneath the path's own
// marking.
func (m *Maze) buildGrid(path, visited []Cell) [][]string {
	outW := 2*m.Width + 1
	outH := 2*m.Height + 1

	grid := make([][]string, outH)
	for y := range grid {
		grid[y] = make([]string, outW)
		for x := range grid[y] {
			grid[y][x] = colorize("█", colorWall)
		}
	}

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			cx, cy := 2*x+1, 2*y+1
			grid[cy][cx] = " "
			if m.isOpen(Cell{x, y}, East) {
				grid[cy][cx+1] = " "
			}
			if m.isOpen(Cell{x, y}, South) {
				grid[cy+1][cx] = " "
			}
		}
	}

	place := func(c Cell, s string) {
		grid[2*c.Y+1][2*c.X+1] = s
	}

	for _, c := range visited {
		if grid[2*c.Y+1][2*c.X+1] == " " {
			place(c, colorize("•", colorVisited))
		}
	}

	if path != nil {
		// Mark the passage cell between two path steps too (the gap that
		// sits between two 2x2 cell blocks), so a route reads as a
		// continuous corridor rather than dots that merely happen to line
		// up. Unconditional (not "if still blank"): the path always wins
		// over a visited-only marker that might already be sitting there.
		markPassage := func(a, b Cell) {
			ax, ay := 2*a.X+1, 2*a.Y+1
			bx, by := 2*b.X+1, 2*b.Y+1
			mx, my := (ax+bx)/2, (ay+by)/2
			grid[my][mx] = colorize("·", colorPath)
		}

		for i, c := range path {
			place(c, colorize("·", colorPath))
			if i+1 < len(path) {
				a, b := path[i], path[i+1]
				// Only mark the between-cell passage for an ordinary
				// grid-adjacent step; a forced teleport jump has no
				// physical corridor to draw between its two endpoints.
				if abs(a.X-b.X)+abs(a.Y-b.Y) == 1 {
					markPassage(a, b)
				}
			}
		}
	}

	for i, t := range m.Teleporters {
		col := teleporterPalette[i%len(teleporterPalette)]
		place(t.From, colorize(string(t.Label), col))
		place(t.To, colorize(string(t.Label+32), col)) // lowercase pair
	}

	// Start/Key/Exit drawn last so they always win visually even if a
	// teleporter ever lands on the same spot (shouldn't happen, PlacePoints
	// runs before PlaceTeleporters and those cells are reserved, but this
	// keeps rendering honest regardless).
	place(m.Start, colorize("S", colorStart))
	place(m.Key, colorize("K", colorKey))
	place(m.Exit, colorize("E", colorExit))

	return grid
}

func (m *Maze) render(path, visited []Cell) string {
	grid := m.buildGrid(path, visited)
	var b strings.Builder
	for _, row := range grid {
		for _, cell := range row {
			b.WriteString(cell)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Render draws the plain, unsolved maze.
func (m *Maze) Render() string { return m.render(nil, nil) }

// RenderPath draws the maze with a found route overlaid, and - if the
// solver reported any - a red dot on every other cell its search visited
// along the way.
func (m *Maze) RenderPath(path, visited []Cell) string { return m.render(path, visited) }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Legend describes the symbols used in Render's output.
func (m *Maze) Legend() string {
	var b strings.Builder
	b.WriteString(colorize("S", colorStart) + " = start   ")
	b.WriteString(colorize("K", colorKey) + " = key (treasure box)   ")
	b.WriteString(colorize("E", colorExit) + " = exit   ")
	b.WriteString(colorize("•", colorVisited) + " = visited but not used (some algorithms don't report this)\n")
	if len(m.Teleporters) > 0 {
		b.WriteString("Teleporters (uppercase <-> matching lowercase, bidirectional, forced - no choice):\n")
		for i, t := range m.Teleporters {
			col := teleporterPalette[i%len(teleporterPalette)]
			b.WriteString("  ")
			b.WriteString(colorize(string(t.Label), col))
			b.WriteString(" <-> ")
			b.WriteString(colorize(string(t.Label+32), col))
			b.WriteByte('\n')
		}
	}
	return b.String()
}
