package main

func init() {
	Register(&HangeonOptimisedV2Solver{})
}

// HangeonOptimisedV2Solver is the CSR-graph experiment. It keeps an
// explicit, immutable directed graph but uses one contiguous neighbor store
// and an offset per vertex instead of a slice per vertex.
type HangeonOptimisedV2Solver struct{}

func (HangeonOptimisedV2Solver) Name() string { return "HangeonOptimisedV2" }

func (HangeonOptimisedV2Solver) Solve(m *Maze) Solution {
	s := newHangeonOptimisedV2State(m)
	edges := make([]Edge, 0, 2*(len(s.queue)-1))
	var primOps int64
	leg1, edges, primOps := s.shortestPath(m.Start, m.Key, edges, primOps)
	leg2, edges, primOps := s.shortestPath(m.Key, m.Exit, edges, primOps)

	visited := make([]Cell, 0, len(edges)+1)
	visited = append(visited, m.Start)
	for _, e := range edges {
		visited = append(visited, e.To)
	}
	if leg1 == nil || leg2 == nil {
		return Solution{Visited: visited, Edges: edges, PrimOps: primOps}
	}
	return Solution{Path: joinLegs(leg1, leg2), Visited: visited, Edges: edges, PrimOps: primOps}
}

type hangeonOptimisedV2State struct {
	width      int
	offsets    []uint32 // neighbors of i are offsets[i]:offsets[i+1]
	neighbors  []int32  // directed, teleport-resolved graph edges
	visitedGen []int32
	parent     []int32
	queue      []int32
	generation int32
}

func newHangeonOptimisedV2State(m *Maze) *hangeonOptimisedV2State {
	n := m.Width * m.Height
	s := &hangeonOptimisedV2State{
		width:      m.Width,
		visitedGen: make([]int32, n),
		parent:     make([]int32, n),
		queue:      make([]int32, n),
	}

	partner := make([]int32, n)
	for i := range partner {
		partner[i] = -1
	}
	for _, t := range m.Teleporters {
		from := int32(t.From.Y*m.Width + t.From.X)
		to := int32(t.To.Y*m.Width + t.To.X)
		partner[from] = to
		partner[to] = from
	}

	// First count the directed edges; then prefix-sum them into CSR offsets.
	// Keep this tiny fixed-degree operation local rather than depending on a
	// graph or bitset package: the four direction flags are the complete
	// graph-building vocabulary for this solver.
	s.offsets = make([]uint32, n+1)
	for i := 0; i < n; i++ {
		mask := m.open[i/m.Width][i%m.Width]
		s.offsets[i+1] = s.offsets[i] + hangeonOptimisedV2Degree(mask)
	}

	// Materialize the explicit graph in stable N,S,E,W edge order.
	s.neighbors = make([]int32, s.offsets[n])
	for i := 0; i < n; i++ {
		mask := m.open[i/m.Width][i%m.Width]
		at := s.offsets[i]
		add := func(landing int32) {
			if destination := partner[landing]; destination != -1 {
				landing = destination
			}
			s.neighbors[at] = landing
			at++
		}
		if mask&North != 0 {
			add(int32(i - m.Width))
		}
		if mask&South != 0 {
			add(int32(i + m.Width))
		}
		if mask&East != 0 {
			add(int32(i + 1))
		}
		if mask&West != 0 {
			add(int32(i - 1))
		}
	}
	return s
}

func hangeonOptimisedV2Degree(mask int) uint32 {
	var degree uint32
	if mask&North != 0 {
		degree++
	}
	if mask&South != 0 {
		degree++
	}
	if mask&East != 0 {
		degree++
	}
	if mask&West != 0 {
		degree++
	}
	return degree
}

func (s *hangeonOptimisedV2State) idx(c Cell) int32 {
	return int32(c.Y*s.width + c.X)
}

func (s *hangeonOptimisedV2State) cellAt(i int32) Cell {
	return Cell{X: int(i) % s.width, Y: int(i) / s.width}
}

func (s *hangeonOptimisedV2State) shortestPath(src, dst Cell, edges []Edge, primOps int64) ([]Cell, []Edge, int64) {
	s.generation++
	gen := s.generation
	srcIdx, dstIdx := s.idx(src), s.idx(dst)

	head, tail := 0, 0
	s.queue[tail] = srcIdx
	tail++
	s.visitedGen[srcIdx] = gen
	s.parent[srcIdx] = -1

	for head < tail {
		cur := s.queue[head]
		head++
		if cur == dstIdx {
			return s.reconstruct(srcIdx, dstIdx), edges, primOps
		}

		fromCell := s.cellAt(cur)
		for _, landing := range s.neighbors[s.offsets[cur]:s.offsets[cur+1]] {
			primOps++ // every neighbor examined is a real primitive op - see Solution.PrimOps
			if s.visitedGen[landing] == gen {
				continue
			}
			s.visitedGen[landing] = gen
			s.parent[landing] = cur
			s.queue[tail] = landing
			tail++
			edges = append(edges, Edge{From: fromCell, To: s.cellAt(landing)})
		}
	}
	return nil, edges, primOps
}

func (s *hangeonOptimisedV2State) reconstruct(src, dst int32) []Cell {
	length := 1
	for cur := dst; cur != src; cur = s.parent[cur] {
		length++
	}
	path := make([]Cell, length)
	for cur, i := dst, length-1; ; cur, i = s.parent[cur], i-1 {
		path[i] = s.cellAt(cur)
		if cur == src {
			return path
		}
	}
}
