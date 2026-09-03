package main

func init() {
	Register(&HangeonOptimizedSolver{})
}

// HangeonOptimizedSolver keeps Hangeon's model exactly as it is - a plain
// adjacency graph, walked by ordinary breadth-first search - but skips the
// 2x-scaled grid entirely: one graph vertex per real maze cell, wired
// directly from the maze's own wall data, with no gap vertices in between.
// There are roughly 4x fewer nodes to visit as a result, and the
// teleporter redirect rule (see buildHangeonGraph's doc comment) collapses
// to its simplest form here - a cell's edge either points at an ordinary
// neighbor or, if that neighbor is a teleporter's trigger tile, straight
// at its paired cell - since there's no gap in between to redirect through
// in the first place.
//
// Unlike plain Hangeon, the graph itself is a flat [][]int32 (one slice
// per cell, indexed by y*width+x) rather than a map[Coordinate][]Coordinate
// - plain Hangeon's 2x-scaled grid has real, useful sparsity (most raw
// grid positions are walls, never becoming graph nodes at all, so a map
// only paying for the vertices that exist is the right fit) but this
// variant's graph is already dense, exactly one node per logical cell -
// a flat array is a strictly better fit, with no hashing or bucket walk on
// every edge traversal. It's walked by bfsOverGraphFlat, the same flat BFS
// BrenThreadOptimized already uses over its own discovered graph.
type HangeonOptimizedSolver struct{}

func (HangeonOptimizedSolver) Name() string { return "HangeonOptimized" }

func (HangeonOptimizedSolver) Solve(m *Maze) Solution {
	idx := func(c Cell) int32 { return int32(c.Y*m.Width + c.X) }
	cellAt := func(i int32) Cell { return Cell{X: int(i) % m.Width, Y: int(i) / m.Width} }

	graph := buildHangeonGraphFlat(m, idx)

	leg1, edges1 := bfsOverGraphFlat(graph, cellAt, idx(m.Start), idx(m.Key))
	leg2, edges2 := bfsOverGraphFlat(graph, cellAt, idx(m.Key), idx(m.Exit))
	edges := append(edges1, edges2...)

	// bfsOverGraphFlat's edges already amount to a full record of every
	// cell either leg visited (one edge per cell reached, from whichever
	// cell discovered it) - Start itself is the only cell that needs
	// adding by hand, since it's the one cell no edge ever points at.
	visited := make([]Cell, 0, len(edges)+1)
	visited = append(visited, m.Start)
	for _, e := range edges {
		visited = append(visited, e.To)
	}

	if leg1 == nil || leg2 == nil {
		return Solution{Visited: visited, Edges: edges} // unreachable; shouldn't happen for a generator-verified maze
	}

	return Solution{
		Path:    joinLegs(leg1, leg2),
		Visited: visited,
		Edges:   edges,
	}
}

// buildHangeonGraphFlat is buildHangeonGraph (solver_hangeon.go) without
// the 2x-scaled grid and without the map-based hangeonGraph type: one flat
// slice per logical maze cell, one edge per open direction, redirected
// straight to a teleporter's paired cell where applicable.
//
// Every cell's neighbor list is a *view* into one shared, preallocated
// []int32 (flatStore, len(allDirections) slots reserved per cell - the
// worst case of every direction being open) rather than its own
// independently append-grown slice - the same fix buildFlatNeighbors
// (solver_brenthread_optimized.go) already applies for the identical
// reason: a cell with 3-4 open directions would otherwise force append to
// reallocate its backing array 2-3 times as it grows past capacity 1, then
// 2, then 4, and across every cell in the maze that's most of this
// solver's allocation cost for no benefit - len(allDirections) is already
// known here, fixed and small. One allocation for the whole adjacency
// list instead of up to one-per-cell (times up to three regrowths each).
func buildHangeonGraphFlat(m *Maze, idx func(Cell) int32) [][]int32 {
	n := m.Width * m.Height
	partner := make([]int32, n)
	for i := range partner {
		partner[i] = -1
	}
	for from, to := range m.TeleportLookup() {
		partner[idx(from)] = idx(to)
	}
	resolve := func(i int32) int32 {
		if p := partner[i]; p != -1 {
			return p
		}
		return i
	}

	flatStore := make([]int32, n*len(allDirections))
	graph := make([][]int32, n)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			c := Cell{X: x, Y: y}
			i := idx(c)
			base := int(i) * len(allDirections)
			count := 0
			for _, dir := range allDirections {
				if m.isOpen(c, dir) {
					flatStore[base+count] = resolve(idx(m.neighbor(c, dir)))
					count++
				}
			}
			graph[i] = flatStore[base : base+count : base+count]
		}
	}
	return graph
}
