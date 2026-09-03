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

	leg1, visited1, edges1 := hangeonShortestPath(g, toCoord(m.Start), toCoord(m.Key), isReal, toCell)
	if leg1 == nil {
		return Solution{Visited: visited1, Edges: edges1}
	}
	leg2, visited2, edges2 := hangeonShortestPath(g, toCoord(m.Key), toCoord(m.Exit), isReal, toCell)
	if leg2 == nil {
		return Solution{Visited: append(visited1, visited2...), Edges: append(edges1, edges2...)}
	}

	return Solution{
		Path:    joinLegs(leg1, leg2),
		Visited: append(visited1, visited2...),
		Edges:   append(edges1, edges2...),
	}
}

// buildHangeonGraph renders the maze as the 2x-scaled grid described on
// HangeonSolver and derives its adjacency graph one wall crossing at a
// time. A forced teleporter is folded in right here, at construction time:
// any edge whose destination is a teleporter's trigger tile is rewired to
// its paired tile instead - "stepping toward a teleporter forces you to
// its partner, no choice in the matter."
//
// Both ends of a pair are triggers (teleporters are bidirectional), which
// makes the gap immediately next to *either* end ambiguous if handled
// naively: that one gap node would need to mean "someone is approaching
// this trigger from outside" (redirect) when reached from its far
// neighbor, but "I just left this trigger, keep going" (don't re-trigger)
// when reached from the trigger's own side - and a graph edge can't carry
// "which direction did you arrive from" information. The fix is to never
// let a trigger's own outgoing move touch its adjacent gap at all: it
// jumps straight to whatever's on the far side of that gap (redirected
// again if that's also a trigger), collapsing two raw hops into one. Every
// gap next to a trigger is then only ever reached, and only ever means,
// "approaching from outside" - there's no second interpretation left for a
// search to accidentally pick.
func buildHangeonGraph(m *Maze) hangeonGraph {
	toCoord := func(c Cell) hangeonCoordinate { return hangeonCoordinate{Row: 2*c.Y + 1, Col: 2*c.X + 1} }

	redirect := make(map[hangeonCoordinate]hangeonCoordinate)
	for from, to := range m.TeleportLookup() {
		redirect[toCoord(from)] = toCoord(to)
	}
	resolve := func(c hangeonCoordinate) hangeonCoordinate {
		if dest, ok := redirect[c]; ok {
			return dest
		}
		return c
	}

	g := make(hangeonGraph)

	// addCrossing wires one open wall between real cells cell and n
	// (gap sitting between them) into the graph, handling every
	// combination of either side being an ordinary cell or a trigger.
	addCrossing := func(cell, gap, n hangeonCoordinate) {
		if _, trigger := redirect[cell]; trigger {
			g[cell] = append(g[cell], resolve(n))
		} else {
			g[cell] = append(g[cell], gap)
			g[gap] = append(g[gap], resolve(n))
		}
		if _, trigger := redirect[n]; trigger {
			g[n] = append(g[n], resolve(cell))
		} else {
			g[n] = append(g[n], gap)
			g[gap] = append(g[gap], resolve(cell))
		}
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
func hangeonShortestPath(g hangeonGraph, src, dst hangeonCoordinate, isReal func(hangeonCoordinate) bool, toCell func(hangeonCoordinate) Cell) (path, visited []Cell, edges []Edge) {
	if _, ok := g[src]; !ok {
		return nil, nil, nil
	}
	if _, ok := g[dst]; !ok {
		return nil, nil, nil
	}
	if src == dst {
		return []Cell{toCell(src)}, []Cell{toCell(src)}, nil
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
		for _, next := range g[current] {
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
				return reconstructHangeonPath(parent, src, dst, isReal, toCell), visited, edges
			}
			queue = append(queue, next)
		}
	}
	return nil, visited, edges // unreachable; shouldn't happen for a generator-verified maze
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
