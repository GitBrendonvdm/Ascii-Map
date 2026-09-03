package main

func init() {
	RegisterMazeStyle(MazeStyle{Name: "Winding", Generate: generateWinding})
}

// defaultWindingBraidProbability is deliberately much lower than the
// Braided style's default: just enough to avoid one perfectly rigid
// spanning tree, while keeping this style's whole visual point - long,
// straight, snake-like corridors - dominant.
const defaultWindingBraidProbability = 0.05

// generateWinding carves the maze as a series of long, mostly-straight
// strands: pick a random already-carved cell, pick a random direction, and
// carve a long run in that direction (occasionally drifting 90 degrees
// rather than turning sharply), stopping when the run's length is used up
// or it runs into a wall or already-carved territory. Repeat, picking a
// fresh random starting point and direction each time, until every cell is
// reachable.
//
// An earlier version of this style used a direction-biased DFS that
// strongly preferred continuing straight - but starting from a corner and
// always preferring "straight, then the nearest wall's turn" is exactly
// the recipe for hugging the grid's boundary inward, which made it
// converge on the same concentric-rectangle spiral shape as the Spiral
// style instead of looking like its own thing. Starting each strand from a
// random point already on the maze, in a random direction, has no such
// structural bias toward any particular shape - strands sprawl outward
// from scattered points across the grid instead of wall-following from one
// continuously growing tip.
func generateWinding(width, height int, seed int64, teleporters int) *Maze {
	m := NewMaze(width, height, seed)

	visited := make([][]bool, height)
	for y := range visited {
		visited[y] = make([]bool, width)
	}

	start := Cell{width / 2, height / 2}
	visited[start.Y][start.X] = true
	visitedCells := []Cell{start}
	total := width * height

	minRun := 4
	maxRun := (width + height) / 2
	if maxRun < minRun+1 {
		maxRun = minRun + 1
	}

	for len(visitedCells) < total {
		from, ok := findWindingFrontierCell(m, visited, visitedCells)
		if !ok {
			break // every visited cell is fully enclosed; shouldn't happen on a connected grid
		}

		dir := allDirections[m.rng.Intn(4)]
		runLen := minRun + m.rng.Intn(maxRun-minRun+1)
		cur := from

		for i := 0; i < runLen; i++ {
			// Occasionally drift 90 degrees rather than snapping to a
			// brand new random direction every step, so a strand still
			// reads as one long corridor with gentle bends, not a jittery
			// walk.
			if i > 0 && m.rng.Intn(3) == 0 {
				dir = windingDrift(m, dir)
			}

			n := m.neighbor(cur, dir)
			if !m.inBounds(n) || visited[n.Y][n.X] {
				// Blocked - give this strand one more direction to try
				// before giving up on it early; a fresh strand will pick
				// up elsewhere on the next outer iteration regardless.
				dir = allDirections[m.rng.Intn(4)]
				n = m.neighbor(cur, dir)
				if !m.inBounds(n) || visited[n.Y][n.X] {
					break
				}
			}

			m.carve(cur, dir)
			visited[n.Y][n.X] = true
			visitedCells = append(visitedCells, n)
			cur = n
		}
	}

	m.Braid(defaultWindingBraidProbability)
	m.PlacePoints()
	m.PlaceTeleporters(teleporters)
	return m
}

// findWindingFrontierCell returns a random already-visited cell that still
// has at least one unvisited neighbor (a valid place to start a new
// strand). A bounded number of random picks handles the common case
// cheaply; a full scan only kicks in once the frontier has shrunk to a
// small fraction of the grid, where random picks would mostly miss.
func findWindingFrontierCell(m *Maze, visited [][]bool, visitedCells []Cell) (Cell, bool) {
	for try := 0; try < 30; try++ {
		c := visitedCells[m.rng.Intn(len(visitedCells))]
		if windingHasUnvisitedNeighbor(m, visited, c) {
			return c, true
		}
	}
	for _, c := range visitedCells {
		if windingHasUnvisitedNeighbor(m, visited, c) {
			return c, true
		}
	}
	return Cell{}, false
}

func windingHasUnvisitedNeighbor(m *Maze, visited [][]bool, c Cell) bool {
	for _, dir := range allDirections {
		n := m.neighbor(c, dir)
		if m.inBounds(n) && !visited[n.Y][n.X] {
			return true
		}
	}
	return false
}

// windingDrift turns 90 degrees left or right from dir, picked randomly -
// never a sharp reversal, so a strand bends gently instead of doubling
// back on itself.
func windingDrift(m *Maze, dir int) int {
	var perpA, perpB int
	if dir == North || dir == South {
		perpA, perpB = East, West
	} else {
		perpA, perpB = North, South
	}
	if m.rng.Intn(2) == 0 {
		return perpA
	}
	return perpB
}
