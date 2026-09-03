package main

func init() {
	Register(&ClaudetteSolver{})
}

// ClaudetteSolver is the flagship "fastest we could build" entry. It's the
// same algorithm as BFSSolver - breadth-first search, so it carries the
// identical shortest-path guarantee - but engineered for raw speed instead
// of readability:
//
//   - cells are addressed by flat int index (y*width+x), not a Cell struct
//     used as a map key, so there's no hashing or map bucket walk
//   - visited/parent/teleport-partner state live in preallocated int32
//     slices indexed directly - O(1) with zero allocation in the hot loop
//   - "visited" uses a generation stamp instead of clearing the array
//     between the start->key and key->exit searches, so the second search
//     doesn't pay an O(n) reset cost
//   - the BFS frontier is a preallocated buffer sized once up front, not a
//     slice that reallocates and copies as it grows
//
// None of this changes *what* gets found - it's still the shortest route
// via BFS - only how fast the search loop runs. Beating Claudette on raw
// wall-clock time (rather than on cleverness) is the standing challenge for
// contributor algorithms.
type ClaudetteSolver struct{}

func (ClaudetteSolver) Name() string { return "Claudette" }

func (ClaudetteSolver) Solve(m *Maze) Solution {
	s := newClaudetteState(m)
	leg1, visited1, edges1, primOps1 := s.shortestPath(m.Start, m.Key)
	leg2, visited2, edges2, primOps2 := s.shortestPath(m.Key, m.Exit)
	return Solution{
		Path:    joinLegs(leg1, leg2),
		Visited: append(visited1, visited2...),
		Edges:   append(edges1, edges2...),
		PrimOps: primOps1 + primOps2,
	}
}

type claudetteState struct {
	m          *Maze
	width      int
	partner    []int32 // teleport partner index, or -1
	visitedGen []int32
	parent     []int32
	queue      []int32 // preallocated frontier buffer, capacity == cell count
	generation int32
}

func newClaudetteState(m *Maze) *claudetteState {
	n := m.Width * m.Height
	s := &claudetteState{
		m:          m,
		width:      m.Width,
		partner:    make([]int32, n),
		visitedGen: make([]int32, n),
		parent:     make([]int32, n),
		queue:      make([]int32, n),
	}
	for i := range s.partner {
		s.partner[i] = -1
	}
	for cell, dest := range m.TeleportLookup() {
		s.partner[s.idx(cell)] = int32(s.idx(dest))
	}
	return s
}

func (s *claudetteState) idx(c Cell) int { return c.Y*s.width + c.X }

func (s *claudetteState) cellAt(i int32) Cell {
	return Cell{X: int(i) % s.width, Y: int(i) / s.width}
}

// shortestPath returns the shortest path and every cell the search visited
// along the way (for the "attempted routes" red-dot overlay - see
// Solution.Visited), regardless of whether that cell ended up on the path.
func (s *claudetteState) shortestPath(src, dst Cell) (path, visited []Cell, edges []Edge, primOps int64) {
	s.generation++
	gen := s.generation

	srcIdx := int32(s.idx(src))
	dstIdx := int32(s.idx(dst))

	head, tail := 0, 0
	s.visitedGen[srcIdx] = gen
	s.parent[srcIdx] = -1
	s.queue[tail] = srcIdx
	tail++

	found := false
	for head < tail {
		cur := s.queue[head]
		head++
		if cur == dstIdx {
			found = true
			break
		}
		curCell := s.cellAt(cur)
		for _, dir := range allDirections {
			primOps++ // every direction checked, open or not, is a real primitive op - see Solution.PrimOps
			if !s.m.isOpen(curCell, dir) {
				continue
			}
			landing := int32(s.idx(s.m.neighbor(curCell, dir)))
			if p := s.partner[landing]; p != -1 {
				landing = p
			}
			if s.visitedGen[landing] == gen {
				continue
			}
			s.visitedGen[landing] = gen
			s.parent[landing] = cur
			edges = append(edges, Edge{From: curCell, To: s.cellAt(landing)})
			s.queue[tail] = landing
			tail++
		}
	}

	visited = s.visitedForGen(gen)
	if found {
		return s.reconstruct(srcIdx, dstIdx), visited, edges, primOps
	}
	return nil, visited, edges, primOps // unreachable; shouldn't happen for a generator-verified maze
}

// visitedForGen scans the generation-stamped visited set for the cells
// stamped during the shortestPath call that owned gen, converting each flat
// index back to a Cell.
func (s *claudetteState) visitedForGen(gen int32) []Cell {
	cells := make([]Cell, 0, len(s.visitedGen))
	for i, g := range s.visitedGen {
		if g == gen {
			cells = append(cells, s.cellAt(int32(i)))
		}
	}
	return cells
}

func (s *claudetteState) reconstruct(src, dst int32) []Cell {
	var revIdx []int32
	for cur := dst; cur != src; cur = s.parent[cur] {
		revIdx = append(revIdx, cur)
	}
	revIdx = append(revIdx, src)
	path := make([]Cell, len(revIdx))
	for i, idx := range revIdx {
		path[len(revIdx)-1-i] = s.cellAt(idx)
	}
	return path
}
