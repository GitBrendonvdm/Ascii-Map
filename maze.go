package main

import "math/rand"

// Direction bitmask for which walls are open on a cell.
const (
	North = 1 << iota
	South
	East
	West
)

var opposite = map[int]int{
	North: South,
	South: North,
	East:  West,
	West:  East,
}

var delta = map[int][2]int{
	North: {0, -1},
	South: {0, 1},
	East:  {1, 0},
	West:  {-1, 0},
}

var allDirections = []int{North, South, East, West}

type Cell struct {
	X, Y int
}

type Teleporter struct {
	Label    byte // 'A', 'B', 'C' ... paired with lowercase on rendering
	From, To Cell
}

type Maze struct {
	Width, Height    int
	open             [][]int // open[y][x] = bitmask of passable directions
	Start, Key, Exit Cell
	Teleporters      []Teleporter
	rng              *rand.Rand
}

func NewMaze(width, height int, seed int64) *Maze {
	m := &Maze{
		Width:  width,
		Height: height,
		rng:    rand.New(rand.NewSource(seed)),
	}
	m.open = make([][]int, height)
	for y := range m.open {
		m.open[y] = make([]int, width)
	}
	return m
}

func (m *Maze) inBounds(c Cell) bool {
	return c.X >= 0 && c.X < m.Width && c.Y >= 0 && c.Y < m.Height
}

func (m *Maze) idx(c Cell) int {
	return c.Y*m.Width + c.X
}

func (m *Maze) neighbor(c Cell, dir int) Cell {
	d := delta[dir]
	return Cell{c.X + d[0], c.Y + d[1]}
}

func (m *Maze) isOpen(c Cell, dir int) bool {
	return m.open[c.Y][c.X]&dir != 0
}

// CellOpenMask exposes a cell's raw open-direction bitmask (see the
// North/South/East/West constants above) for callers outside this file that
// need to describe the maze's wall layout, e.g. the JSON exporter.
func (m *Maze) CellOpenMask(c Cell) int {
	return m.open[c.Y][c.X]
}

// carve opens the wall between c and its neighbor in direction dir (both sides).
func (m *Maze) carve(c Cell, dir int) {
	n := m.neighbor(c, dir)
	m.open[c.Y][c.X] |= dir
	m.open[n.Y][n.X] |= opposite[dir]
}

// GeneratePerfectMaze builds a spanning-tree maze via randomized DFS
// (recursive backtracker), guaranteeing every cell is reachable.
func (m *Maze) GeneratePerfectMaze() {
	visited := make([][]bool, m.Height)
	for y := range visited {
		visited[y] = make([]bool, m.Width)
	}

	start := Cell{0, 0}
	visited[start.Y][start.X] = true
	stack := []Cell{start}

	dirs := []int{North, South, East, West}

	for len(stack) > 0 {
		cur := stack[len(stack)-1]

		// Shuffle direction order for randomness.
		m.rng.Shuffle(len(dirs), func(i, j int) { dirs[i], dirs[j] = dirs[j], dirs[i] })

		advanced := false
		for _, dir := range dirs {
			n := m.neighbor(cur, dir)
			if !m.inBounds(n) || visited[n.Y][n.X] {
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

// Braid randomly knocks down extra walls to introduce loops (cycles), so
// the maze stops being a strict tree and gains alternate routes. prob is
// the chance, per closed internal wall, that it gets opened.
func (m *Maze) Braid(prob float64) {
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			c := Cell{x, y}
			for _, dir := range []int{East, South} {
				n := m.neighbor(c, dir)
				if !m.inBounds(n) {
					continue
				}
				if m.isOpen(c, dir) {
					continue
				}
				if m.rng.Float64() < prob {
					m.carve(c, dir)
				}
			}
		}
	}
}

// openCutWall opens a uniformly random currently-closed wall connecting a
// reachableFromSrc cell to a non-reachableFromSrc one - see
// raiseEdgeDisjointPaths (flow.go) for why every such wall is guaranteed
// to raise the edge-disjoint path count it's called about. Returns false
// if no such wall exists (shouldn't happen for a spanning-tree-derived
// maze, but stay correct if it ever does).
func (m *Maze) openCutWall(reachableFromSrc []bool) bool {
	type edge struct {
		c   Cell
		dir int
	}
	var candidates []edge
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			c := Cell{x, y}
			ci := m.idx(c)
			if !reachableFromSrc[ci] {
				continue
			}
			for _, dir := range allDirections {
				n := m.neighbor(c, dir)
				if !m.inBounds(n) || m.isOpen(c, dir) {
					continue
				}
				if !reachableFromSrc[m.idx(n)] {
					candidates = append(candidates, edge{c, dir})
				}
			}
		}
	}
	if len(candidates) == 0 {
		return false
	}
	e := candidates[m.rng.Intn(len(candidates))]
	m.carve(e.c, e.dir)
	return true
}

// gridDegree returns how many grid neighbors a cell can ever have (2 in a
// corner, 3 on an edge, 4 in the interior). By Menger's theorem this is a
// hard ceiling on the number of edge-disjoint paths that can ever touch that
// cell, no matter how many walls get opened - a corner cell is structurally
// stuck at 2.
func (m *Maze) gridDegree(c Cell) int {
	n := 0
	for _, dir := range []int{North, South, East, West} {
		if m.inBounds(m.neighbor(c, dir)) {
			n++
		}
	}
	return n
}

// EnsureRedundantRoutes keeps opening extra walls until there are at least
// minPaths edge-disjoint routes between start-key and key-exit (verified via
// max-flow / Menger's theorem), so the maze always has genuine alternate
// paths rather than a single "one true solution" corridor. The target is
// clamped per-pair to whatever the endpoints' grid degrees can structurally
// support (e.g. a corner cell caps routes at 2), and the achieved/target
// clamp is returned so the caller can report honestly instead of pretending
// an impossible target was met.
//
// The actual wall-opening happens in raiseEdgeDisjointPaths (flow.go): each
// wall it opens is the current max-flow computation's own min-cut edge,
// mathematically guaranteed (max-flow/min-cut duality) to raise the
// achieved count by exactly one, so reaching target here costs exactly
// target-achieved wall opens, at any maze size - not the hundreds of
// random guesses an earlier version of this needed (see
// raiseEdgeDisjointPaths' own doc comment for the measured numbers).
func (m *Maze) EnsureRedundantRoutes(start, key, exit Cell, minPaths int) (achievedStartKey, achievedKeyExit, targetStartKey, targetKeyExit int) {
	targetStartKey = minPaths
	if d := m.gridDegree(start); d < targetStartKey {
		targetStartKey = d
	}
	if d := m.gridDegree(key); d < targetStartKey {
		targetStartKey = d
	}
	targetKeyExit = minPaths
	if d := m.gridDegree(key); d < targetKeyExit {
		targetKeyExit = d
	}
	if d := m.gridDegree(exit); d < targetKeyExit {
		targetKeyExit = d
	}

	achievedStartKey = m.raiseEdgeDisjointPaths(start, key, targetStartKey)
	achievedKeyExit = m.raiseEdgeDisjointPaths(key, exit, targetKeyExit)
	return
}

// BFS distance from a source cell to every other cell, ignoring teleporters
// (used only for picking well-separated start/key/exit points).
func (m *Maze) bfsDistances(src Cell) [][]int {
	dist := make([][]int, m.Height)
	for y := range dist {
		dist[y] = make([]int, m.Width)
		for x := range dist[y] {
			dist[y][x] = -1
		}
	}
	dist[src.Y][src.X] = 0
	queue := []Cell{src}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, dir := range allDirections {
			if !m.isOpen(cur, dir) {
				continue
			}
			n := m.neighbor(cur, dir)
			if dist[n.Y][n.X] == -1 {
				dist[n.Y][n.X] = dist[cur.Y][cur.X] + 1
				queue = append(queue, n)
			}
		}
	}
	return dist
}

func farthestCell(dist [][]int, exclude ...Cell) Cell {
	best := Cell{0, 0}
	bestDist := -1
	for y := range dist {
		for x := range dist[y] {
			c := Cell{x, y}
			skip := false
			for _, e := range exclude {
				if e == c {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			if dist[y][x] > bestDist {
				bestDist = dist[y][x]
				best = c
			}
		}
	}
	return best
}

// PlacePoints chooses Start, Key and Exit as three well-separated cells
// using a double-sweep heuristic: start is the grid corner, key is the cell
// farthest from start, exit is the cell farthest from key. This is the
// right choice for a fully-connected, whole-grid maze; a style that carves
// only part of the grid (rooms, split halves, ...) should use
// PlacePointsFrom instead so Start lands somewhere actually reachable.
func (m *Maze) PlacePoints() {
	m.PlacePointsFrom(Cell{0, 0})
}

// PlacePointsFrom is PlacePoints but with an explicit Start cell, for maze
// styles where Start can't just be the grid corner - e.g. because that
// corner was never carved into the maze at all (rooms-and-corridors, a
// maze confined to one half of the grid, ...). Key and Exit are still
// chosen via the same double-sweep heuristic (farthest cell from start,
// then farthest cell from key), which only ever considers cells actually
// reachable from start - so it degrades gracefully to whatever connected
// region start sits in, not the whole grid.
func (m *Maze) PlacePointsFrom(start Cell) {
	distFromStart := m.bfsDistances(start)
	key := farthestCell(distFromStart, start)

	distFromKey := m.bfsDistances(key)
	exit := farthestCell(distFromKey, start, key)

	m.Start, m.Key, m.Exit = start, key, exit
}

// PlaceTeleporters drops n forced one-way-no-choice teleporter pairs onto
// random empty cells. After each placement it re-checks full solvability
// (start->key and key->exit) using forced-teleport BFS; a placement that
// would softlock the maze is retried on different cells instead.
//
// Any teleporter a generator already placed on m.Teleporters before calling
// this (e.g. a structural, mandatory crossing a style's own layout depends
// on) has its cells and label reserved too, so the ones placed here can
// never collide with - or relabel over - one that already exists.
func (m *Maze) PlaceTeleporters(n int) {
	reserved := map[Cell]bool{m.Start: true, m.Key: true, m.Exit: true}

	// S, K and E are reserved for start/key/exit, so skip them when handing
	// out teleporter labels to avoid a teleporter rendering as an
	// indistinguishable duplicate of one of those markers.
	reservedLetters := map[byte]bool{'S': true, 'K': true, 'E': true}
	for _, t := range m.Teleporters {
		reserved[t.From] = true
		reserved[t.To] = true
		reservedLetters[t.Label] = true
	}
	var labels []byte
	for c := byte('A'); c <= 'Z'; c++ {
		if !reservedLetters[c] {
			labels = append(labels, c)
		}
	}
	if n > len(labels) {
		n = len(labels) // out of distinct letters; cap rather than collide
	}
	labelFor := func(i int) byte { return labels[i] }

	for i := 0; i < n; i++ {
		const maxTries = 500
		placed := false
		for try := 0; try < maxTries; try++ {
			a := Cell{m.rng.Intn(m.Width), m.rng.Intn(m.Height)}
			b := Cell{m.rng.Intn(m.Width), m.rng.Intn(m.Height)}
			if a == b || reserved[a] || reserved[b] {
				continue
			}

			candidate := Teleporter{Label: labelFor(i), From: a, To: b}
			trial := append(append([]Teleporter{}, m.Teleporters...), candidate)

			if m.solvableWithTeleporters(trial) {
				m.Teleporters = trial
				reserved[a] = true
				reserved[b] = true
				placed = true
				break
			}
		}
		if !placed {
			// Couldn't find a safe placement for this pair within the try
			// budget; skip it rather than risk an unsolvable maze.
			continue
		}
	}
}

// teleportLookup builds a map from a teleporter cell to where stepping on
// it forcibly sends the player (both directions of every pair).
func teleportLookup(tps []Teleporter) map[Cell]Cell {
	lookup := make(map[Cell]Cell, len(tps)*2)
	for _, t := range tps {
		lookup[t.From] = t.To
		lookup[t.To] = t.From
	}
	return lookup
}

// reachableWithTeleporters does a BFS over the maze where entering a
// teleporter cell always, unconditionally, redirects to its paired cell
// (mirrors "no choice in the matter" teleport semantics).
func (m *Maze) reachableWithTeleporters(src Cell, tps []Teleporter) map[Cell]bool {
	lookup := teleportLookup(tps)
	visited := map[Cell]bool{src: true}
	queue := []Cell{src}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, dir := range allDirections {
			if !m.isOpen(cur, dir) {
				continue
			}
			landing := m.neighbor(cur, dir)
			if dest, ok := lookup[landing]; ok {
				landing = dest
			}
			if !visited[landing] {
				visited[landing] = true
				queue = append(queue, landing)
			}
		}
	}
	return visited
}

func (m *Maze) solvableWithTeleporters(tps []Teleporter) bool {
	fromStart := m.reachableWithTeleporters(m.Start, tps)
	if !fromStart[m.Key] {
		return false
	}
	fromKey := m.reachableWithTeleporters(m.Key, tps)
	return fromKey[m.Exit]
}

// TeleportLookup exposes the cell->paired-cell map (both directions) so
// solver algorithms can resolve forced teleportation without duplicating
// the pairing logic.
func (m *Maze) TeleportLookup() map[Cell]Cell {
	return teleportLookup(m.Teleporters)
}

// Clone returns an independent deep copy of the maze, so every algorithm in
// a benchmark run gets its own untouched, unsolved copy - one algorithm's
// bugs or bookkeeping can never leak into another's run.
func (m *Maze) Clone() *Maze {
	c := &Maze{
		Width:       m.Width,
		Height:      m.Height,
		Start:       m.Start,
		Key:         m.Key,
		Exit:        m.Exit,
		Teleporters: append([]Teleporter{}, m.Teleporters...),
	}
	c.open = make([][]int, m.Height)
	for y := range c.open {
		c.open[y] = append([]int{}, m.open[y]...)
	}
	return c
}
