package main

// hangeonCoordinate is a (row, col) position in Hangeon's own graph model -
// deliberately Row/Col-addressed and grid-shaped, rather than reusing
// Cell's X/Y bitmask-neighbor style: this solver models the maze the way a
// simple dungeon-crawler graph library would (parse a grid of traversable
// positions, derive a plain adjacency graph from it, walk the graph with
// breadth-first search), not the way the rest of this codebase does, so
// its search tree looks genuinely different from the bitmask-based solvers
// on an identical maze even though it finds an equally optimal route.
type hangeonCoordinate struct {
	Row, Col int
}

// hangeonGraph is a plain adjacency list: every traversable position maps
// to the traversable positions reachable from it in one step.
type hangeonGraph map[hangeonCoordinate][]hangeonCoordinate

func init() {
	Register(&HangeonSolver{})
}

// HangeonSolver renders the maze as a 2x-scaled grid - one vertex for every
// real cell, plus one "gap" vertex on every open wall crossing between two
// adjacent real cells, matching how a simple ASCII dungeon-crawler graph
// library would treat every traversable character as its own vertex -
// derives a plain Coordinate-keyed adjacency graph from it, and finds the
// route with an ordinary breadth-first search. The gap vertices give the
// graph genuine geometric structure (two hops to cross an opening, not an
// abstract "is this direction open" bit), which is what makes this
// solver's search tree look meaningfully different from the bitmask-based
// solvers, even though the shortest path it finds is exactly as optimal as
// BFS's.
type HangeonSolver struct{}

func (HangeonSolver) Name() string { return "Hangeon" }

func (HangeonSolver) Solve(m *Maze) Solution {
	g := buildHangeonGraph(m)

	toCoord := func(c Cell) hangeonCoordinate { return hangeonCoordinate{Row: 2*c.Y + 1, Col: 2*c.X + 1} }
	toCell := func(c hangeonCoordinate) Cell { return Cell{X: (c.Col - 1) / 2, Y: (c.Row - 1) / 2} }
	isReal := func(c hangeonCoordinate) bool { return c.Row%2 == 1 && c.Col%2 == 1 }

	// lookup is TeleportLookup translated into this solver's own coordinate
	// space, applied at traversal time in hangeonShortestPath rather than
	// baked into the graph - see buildHangeonGraph's doc comment for why.
	lookup := make(map[hangeonCoordinate]hangeonCoordinate, len(m.Teleporters)*2)
	for from, to := range m.TeleportLookup() {
		lookup[toCoord(from)] = toCoord(to)
	}

	leg1, visited1, edges1, primOps1 := hangeonShortestPath(g, lookup, toCoord(m.Start), toCoord(m.Key), isReal, toCell)
	if leg1 == nil {
		return Solution{Visited: visited1, Edges: edges1, PrimOps: primOps1}
	}
	leg2, visited2, edges2, primOps2 := hangeonShortestPath(g, lookup, toCoord(m.Key), toCoord(m.Exit), isReal, toCell)
	if leg2 == nil {
		return Solution{Visited: append(visited1, visited2...), Edges: append(edges1, edges2...), PrimOps: primOps1 + primOps2}
	}

	return Solution{
		Path:    joinLegs(leg1, leg2),
		Visited: append(visited1, visited2...),
		Edges:   append(edges1, edges2...),
		PrimOps: primOps1 + primOps2,
	}
}

// hangeonGapOffset shifts a crossing's forward gap coordinate into a
// second, disjoint region of coordinate space to build its distinct
// backward gap - see buildHangeonGraph's doc comment. Real cells and
// ordinary gaps both stay within [0, 2*maxDimension], so this - comfortably
// beyond any maze this project generates - never collides with a real
// coordinate or with another crossing's own gaps.
const hangeonGapOffset = 1 << 20

// buildHangeonGraph renders the maze as the 2x-scaled grid described on
// HangeonSolver and derives its adjacency graph one wall crossing at a
// time - purely topological, with no teleport awareness at all: forced
// teleportation is resolved separately, at BFS traversal time (see
// hangeonShortestPath), the same "redirect the moment you're about to land
// on a registered endpoint" pattern every other solver in this codebase
// already uses, not baked into the graph itself.
//
// Each open wall between real cells cell and n gets *two* single-purpose
// gap nodes - one for cell->n, a different one for n->cell - rather than
// one shared, undirected gap serving both directions. Two earlier versions
// of this function tried sharing a single gap and ran into two different
// failure modes that both trace back to the same root cause: a shared
// gap's edge list can't record which direction it was entered from, so an
// edge meant for approaching a trigger from one side was just as reachable
// from the other. The first attempt resolved each of a gap's two edges
// independently at construction time - meaning the partner of a trigger
// endpoint sat right there in the shared list regardless of which side you
// came from, so stepping back the way you'd just come from (the same gap,
// same direction, no new ground covered) could hop to a completely
// unrelated, distant cell in one step (an illegal move a stress test
// caught via ValidatePath). Patching that by skipping a gap's edge back
// toward its own immediate predecessor fixed the illegal move but broke
// something subtler: a *shared* gap can only ever be marked visited once,
// so the instant either cell's approach claimed it, the other cell's
// entirely different, legitimate approach through the same physical
// crossing was permanently locked out too - even on mazes where the true
// shortest route (independently verified against BFS) requires bouncing
// off the same teleporter from two different neighboring cells. Giving
// each direction its own private node removes the sharing that both bugs
// trace back to, instead of special-casing around it: forward and
// backward can never block each other, because they were never the same
// graph node to begin with.
func buildHangeonGraph(m *Maze) hangeonGraph {
	toCoord := func(c Cell) hangeonCoordinate { return hangeonCoordinate{Row: 2*c.Y + 1, Col: 2*c.X + 1} }

	g := make(hangeonGraph)

	// addCrossing wires one open wall between real cells cell and n into
	// the graph as two independent, single-edge directed hops: cell->
	// gapForward->n, and (via a disjoint gapForward+offset coordinate)
	// n->gapBackward->cell.
	addCrossing := func(cell, gapForward, n hangeonCoordinate) {
		g[cell] = append(g[cell], gapForward)
		g[gapForward] = append(g[gapForward], n)

		gapBackward := hangeonCoordinate{Row: gapForward.Row + hangeonGapOffset, Col: gapForward.Col}
		g[n] = append(g[n], gapBackward)
		g[gapBackward] = append(g[gapBackward], cell)
	}

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			c := Cell{X: x, Y: y}
			cell := toCoord(c)
			if m.isOpen(c, East) {
				gap := hangeonCoordinate{Row: cell.Row, Col: cell.Col + 1}
				addCrossing(cell, gap, toCoord(m.neighbor(c, East)))
			}
			if m.isOpen(c, South) {
				gap := hangeonCoordinate{Row: cell.Row + 1, Col: cell.Col}
				addCrossing(cell, gap, toCoord(m.neighbor(c, South)))
			}
		}
	}
	return g
}

// hangeonShortestPath is one breadth-first search over g from src to dst,
// in real-cell terms throughout: it returns the shortest path (nil if
// unreachable), every real cell it visited, and its search tree's edges,
// all converted via toCell. isReal/toCell abstract over whether g still has
// "gap" nodes (plain Hangeon's 2x-scaled grid) or is already one node per
// real cell (HangeonOptimized, see solver_hangeon_optimized.go): a gap
// contributes no path/visited/edge entry of its own - hopping through one
// just extends its nearest real ancestor's reach one hop further, so two
// raw hops through a gap collapse transparently into one edge between the
// real cells on either side of it.
//
// Forced teleportation is resolved here, not in g itself (see
// buildHangeonGraph's doc comment for why): the moment a graph edge's raw
// destination is a real, registered teleporter cell, it's redirected to
// its paired cell before anything else - visited/parent/edge bookkeeping,
// the dst check - ever sees it, so a redirect is indistinguishable from an
// ordinary edge to everything downstream, exactly like every other solver
// in this codebase treats it.
func hangeonShortestPath(g hangeonGraph, lookup map[hangeonCoordinate]hangeonCoordinate, src, dst hangeonCoordinate, isReal func(hangeonCoordinate) bool, toCell func(hangeonCoordinate) Cell) (path, visited []Cell, edges []Edge, primOps int64) {
	if _, ok := g[src]; !ok {
		return nil, nil, nil, 0
	}
	if _, ok := g[dst]; !ok {
		return nil, nil, nil, 0
	}
	if src == dst {
		return []Cell{toCell(src)}, []Cell{toCell(src)}, nil, 0
	}

	visitedSet := map[hangeonCoordinate]bool{src: true}
	parent := map[hangeonCoordinate]hangeonCoordinate{}
	// realAncestor[c] is the nearest real cell on c's path back to src -
	// itself, if c is real. src is always real (both Hangeon variants only
	// ever call this with a real Start/Key/Exit coordinate), so this is
	// always resolvable by the time any node is dequeued.
	realAncestor := map[hangeonCoordinate]hangeonCoordinate{src: src}
	if isReal(src) {
		visited = append(visited, toCell(src))
	}

	queue := []hangeonCoordinate{src}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		ancestor := realAncestor[current]
		for _, raw := range g[current] {
			primOps++ // every graph edge examined - real cell or gap - is a real primitive op, see Solution.PrimOps
			next := raw
			if dest, ok := lookup[next]; ok {
				next = dest // forced redirect, resolved before this edge is treated as reaching anywhere
			}
			if visitedSet[next] {
				continue
			}
			visitedSet[next] = true
			parent[next] = current
			if isReal(next) {
				realAncestor[next] = next
				visited = append(visited, toCell(next))
				if next != ancestor {
					edges = append(edges, Edge{From: toCell(ancestor), To: toCell(next)})
				}
			} else {
				realAncestor[next] = ancestor
			}
			if next == dst {
				return reconstructHangeonPath(parent, src, dst, isReal, toCell), visited, edges, primOps
			}
			queue = append(queue, next)
		}
	}
	return nil, visited, edges, primOps // unreachable; shouldn't happen for a generator-verified maze
}

// reconstructHangeonPath walks parent pointers from dst back to src,
// keeping only the real-cell positions (gaps carry no cell of their own).
func reconstructHangeonPath(parent map[hangeonCoordinate]hangeonCoordinate, src, dst hangeonCoordinate, isReal func(hangeonCoordinate) bool, toCell func(hangeonCoordinate) Cell) []Cell {
	var raw []hangeonCoordinate
	for cur := dst; ; cur = parent[cur] {
		raw = append(raw, cur)
		if cur == src {
			break
		}
	}
	path := make([]Cell, 0, len(raw))
	for i := len(raw) - 1; i >= 0; i-- {
		if isReal(raw[i]) {
			path = append(path, toCell(raw[i]))
		}
	}
	return path
}
