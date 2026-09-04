package main

func init() {
	Register(&DFSSolver{})
}

// DFSSolver is a naive depth-first search: it walks the first unvisited
// direction it finds and backtracks on dead ends, using an explicit stack
// (not recursion, so it can't blow the call stack on a large maze). It is
// NOT guaranteed to find the shortest route - only *a* route - which makes
// it a useful baseline: it usually runs fast, but its path is typically
// much longer than BFS/A*/Claudette. A good illustration that "fast" and
// "shortest route" are different axes to compete on.
type DFSSolver struct{}

func (DFSSolver) Name() string { return "DFS" }

func (DFSSolver) Solve(m *Maze) Solution {
	leg1, visited1, edges1, primOps1 := dfsPath(m, m.Start, m.Key)
	leg2, visited2, edges2, primOps2 := dfsPath(m, m.Key, m.Exit)
	return Solution{
		Path:    joinLegs(leg1, leg2),
		Visited: append(visited1, visited2...),
		Edges:   append(edges1, edges2...),
		PrimOps: primOps1 + primOps2,
	}
}

func dfsPath(m *Maze, src, dst Cell) (path, visited []Cell, edges []Edge, primOps int64) {
	lookup := m.TeleportLookup()

	type frame struct {
		cell   Cell
		dirIdx int
	}

	visitedSet := map[Cell]bool{src: true}
	parent := map[Cell]Cell{}
	stack := []frame{{cell: src}}

	if src == dst {
		return []Cell{src}, visitedSlice(visitedSet), edges, primOps
	}

	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if top.dirIdx >= len(allDirections) {
			stack = stack[:len(stack)-1]
			continue
		}
		dir := allDirections[top.dirIdx]
		top.dirIdx++
		primOps++ // every direction checked, open or not, is a real primitive op - see Solution.PrimOps

		if !m.isOpen(top.cell, dir) {
			continue
		}
		landing := m.neighbor(top.cell, dir)
		if dest, ok := lookup[landing]; ok {
			landing = dest
		}
		if visitedSet[landing] {
			continue
		}
		visitedSet[landing] = true
		parent[landing] = top.cell
		edges = append(edges, Edge{From: top.cell, To: landing})
		if landing == dst {
			return reconstructPath(parent, src, dst), visitedSlice(visitedSet), edges, primOps
		}
		stack = append(stack, frame{cell: landing})
	}
	return nil, visitedSlice(visitedSet), edges, primOps // unreachable; shouldn't happen for a generator-verified maze
}
