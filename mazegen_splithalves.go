package main

func init() {
	RegisterMazeStyle(MazeStyle{Name: "SplitHalves", Generate: generateSplitHalves})
}

// splitHalvesMinRoutes matches the Braided style's own default -min-routes:
// each half should feel just as redundant internally as a normal Braided
// maze, not like the single "one true solution" corridor a bare
// spanning-tree carve produces on its own.
const splitHalvesMinRoutes = 2

// splitHalvesBraidProbability matches Braided's own default -braid: light,
// general-purpose looping applied to each half before Start/Key/Exit are
// even chosen, on top of the formal min-routes guarantee applied afterward
// once they are.
const splitHalvesBraidProbability = 0.15

// generateSplitHalves builds two independent mazes side by side - a left
// half confined to columns < midX and a right half confined to columns >=
// midX, each with its own redundant routes (braided, then formally
// guaranteed edge-disjoint alternates - see below) - with no ordinary wall
// ever connecting the two halves. The only way across is one mandatory
// forced teleporter, placed directly (not via PlaceTeleporters, and always
// present regardless of the -teleporters flag, the same way Perfect always
// skips teleporters entirely for its own structural reason): a plain wall
// gap would let a player *choose* to cross without ever touching a
// teleporter, defeating the entire point of a style whose key or exit sits
// behind one. Key is confined to the left half (Start's side) and Exit to
// the right half, so reaching Exit always means actually being forced
// across by that teleporter.
func generateSplitHalves(width, height int, seed int64, teleporters int) *Maze {
	m := NewMaze(width, height, seed)
	midX := width / 2

	carveConfinedPerfectMaze(m, Cell{0, 0}, 0, midX-1)
	carveConfinedPerfectMaze(m, Cell{width - 1, 0}, midX, width-1)

	// General-purpose looping within each half - every wall-opening helper
	// used here and below is confined to its own half's column range, so
	// this can never accidentally open the one connection that's supposed
	// to only ever be the mandatory teleporter.
	braidConfined(m, splitHalvesBraidProbability, 0, midX-1)
	braidConfined(m, splitHalvesBraidProbability, midX, width-1)

	start := Cell{0, 0}

	// Any cell in each half works here - unlike PlaceTeleporters' general
	// case, there's no alternate route this placement could accidentally
	// break, since the halves have no other connection at all yet.
	var left Cell
	for {
		left = Cell{m.rng.Intn(midX), m.rng.Intn(height)}
		if left != start {
			break
		}
	}
	right := Cell{midX + m.rng.Intn(width-midX), m.rng.Intn(height)}
	m.Teleporters = []Teleporter{{Label: 'A', From: left, To: right}}

	// Key is the farthest reachable cell from Start within the left half
	// (ordinary walls only - bfsDistances never crosses the teleporter).
	// left itself is excluded: stepping onto it always redirects to right,
	// so it can never be a valid resting cell for Key to occupy.
	distFromStart := m.bfsDistances(start)
	var key Cell
	bestKeyDist := -1
	for y := 0; y < height; y++ {
		for x := 0; x < midX; x++ {
			if (Cell{x, y}) == left {
				continue
			}
			if distFromStart[y][x] > bestKeyDist {
				bestKeyDist = distFromStart[y][x]
				key = Cell{x, y}
			}
		}
	}

	// Exit is the farthest reachable cell from right within the right half
	// - right is where a player actually lands after being forced across,
	// so that's the correct starting point for "farthest away", not Start
	// (which has no ordinary-wall path into the right half at all).
	distFromRight := m.bfsDistances(right)
	var exit Cell
	bestExitDist := -1
	for y := 0; y < height; y++ {
		for x := midX; x < width; x++ {
			if (Cell{x, y}) == right {
				continue
			}
			if distFromRight[y][x] > bestExitDist {
				bestExitDist = distFromRight[y][x]
				exit = Cell{x, y}
			}
		}
	}

	m.Start, m.Key, m.Exit = start, key, exit

	// Formally guarantee redundant routes for the specific pair each half
	// actually needs it for (Start<->Key on the left, right<->Exit on the
	// right) - the same targeted, max-flow-verified guarantee Braided
	// applies, on top of the general braiding already done above, in case
	// the light braid pass didn't happen to open a second route near
	// these particular two points.
	ensureRedundantRoutesConfined(m, start, key, splitHalvesMinRoutes, 0, midX-1)
	ensureRedundantRoutesConfined(m, right, exit, splitHalvesMinRoutes, midX, width-1)

	if teleporters > 1 {
		m.PlaceTeleporters(teleporters - 1) // extra flavor teleporters, on top of the mandatory crossing
	}
	return m
}

// carveConfinedPerfectMaze is GeneratePerfectMaze's randomized DFS
// (recursive backtracker), but confined to columns [minX, maxX] - a neighbor
// outside that column range is treated the same as an out-of-grid-bounds
// neighbor and skipped, so the carve never crosses into the other half.
func carveConfinedPerfectMaze(m *Maze, start Cell, minX, maxX int) {
	visited := make([][]bool, m.Height)
	for y := range visited {
		visited[y] = make([]bool, m.Width)
	}
	visited[start.Y][start.X] = true
	stack := []Cell{start}

	dirs := []int{North, South, East, West}

	for len(stack) > 0 {
		cur := stack[len(stack)-1]

		m.rng.Shuffle(len(dirs), func(i, j int) { dirs[i], dirs[j] = dirs[j], dirs[i] })

		advanced := false
		for _, dir := range dirs {
			n := m.neighbor(cur, dir)
			if !m.inBounds(n) || n.X < minX || n.X > maxX || visited[n.Y][n.X] {
				continue
			}
			m.carve(cur, dir)
			visited[n.Y][n.X] = true
			stack = append(stack, n)
			advanced = true
			break
		}

		if !advanced {
			stack = stack[:len(stack)-1]
		}
	}
}

// braidConfined is Maze.Braid (maze.go), scoped to columns [minX,maxX]:
// East walls are only ever considered when the neighbor is still inside
// that range, so a braid pass can never open the one connection between
// SplitHalves' two halves that's supposed to be the mandatory teleporter
// alone. South walls never cross halves in the first place (each half
// spans the maze's full height), so they need no such guard.
func braidConfined(m *Maze, prob float64, minX, maxX int) {
	for y := 0; y < m.Height; y++ {
		for x := minX; x <= maxX; x++ {
			c := Cell{x, y}
			if x+1 <= maxX && !m.isOpen(c, East) && m.rng.Float64() < prob {
				m.carve(c, East)
			}
			if y+1 < m.Height && !m.isOpen(c, South) && m.rng.Float64() < prob {
				m.carve(c, South)
			}
		}
	}
}

// openRandomWallConfined is Maze.openRandomWall (maze.go), scoped to
// columns [minX,maxX] for the same reason as braidConfined.
func openRandomWallConfined(m *Maze, minX, maxX int) bool {
	type edge struct {
		c   Cell
		dir int
	}
	var closed []edge
	for y := 0; y < m.Height; y++ {
		for x := minX; x <= maxX; x++ {
			c := Cell{x, y}
			if x+1 <= maxX && !m.isOpen(c, East) {
				closed = append(closed, edge{c, East})
			}
			if y+1 < m.Height && !m.isOpen(c, South) {
				closed = append(closed, edge{c, South})
			}
		}
	}
	if len(closed) == 0 {
		return false
	}
	e := closed[m.rng.Intn(len(closed))]
	m.carve(e.c, e.dir)
	return true
}

// openWallNearPathConfined is Maze.openWallNearPath (maze.go), scoped to
// columns [minX,maxX] for the same reason as braidConfined: it opens a
// wall touching the current shortest route between a and b, but only ever
// considers walls that stay within the half a and b already both live in.
func openWallNearPathConfined(m *Maze, a, b Cell, minX, maxX int) bool {
	dist := m.bfsDistances(a)
	if dist[b.Y][b.X] == -1 {
		return false
	}

	type edge struct {
		c   Cell
		dir int
	}
	var closed []edge
	seen := map[Cell]bool{}
	for cur := b; ; {
		if !seen[cur] {
			seen[cur] = true
			for _, dir := range allDirections {
				n := m.neighbor(cur, dir)
				if !m.inBounds(n) || n.X < minX || n.X > maxX {
					continue
				}
				if !m.isOpen(cur, dir) {
					closed = append(closed, edge{cur, dir})
				}
			}
		}
		if cur == a {
			break
		}
		next := cur
		for _, dir := range allDirections {
			if !m.isOpen(cur, dir) {
				continue
			}
			n := m.neighbor(cur, dir)
			if m.inBounds(n) && dist[n.Y][n.X] == dist[cur.Y][cur.X]-1 {
				next = n
				break
			}
		}
		if next == cur {
			break
		}
		cur = next
	}

	if len(closed) == 0 {
		return false
	}
	e := closed[m.rng.Intn(len(closed))]
	m.carve(e.c, e.dir)
	return true
}

// ensureRedundantRoutesConfined is Maze.EnsureRedundantRoutes (maze.go),
// scoped to columns [minX,maxX]: it opens walls (preferring ones near a
// and b's current shortest path, same as the unconfined version) until
// there are at least minPaths edge-disjoint routes between a and b,
// without ever touching a wall outside that half.
func ensureRedundantRoutesConfined(m *Maze, a, b Cell, minPaths, minX, maxX int) {
	target := minPaths
	if d := m.gridDegree(a); d < target {
		target = d
	}
	if d := m.gridDegree(b); d < target {
		target = d
	}

	const maxAttempts = 2000
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if m.edgeDisjointPaths(a, b, target) >= target {
			return
		}
		if !openWallNearPathConfined(m, a, b, minX, maxX) && !openRandomWallConfined(m, minX, maxX) {
			return
		}
	}
}
