package main

// edgeDisjointPaths reports how many edge-disjoint routes exist between src
// and dst, capped at cap (we only ever need to know "is it >= 2", so we stop
// searching for augmenting paths once we hit cap). This is a direct
// application of Menger's theorem: max-flow with unit edge capacities equals
// the maximum number of edge-disjoint paths, which is exactly what "multiple
// valid routes" needs to mean to be a real structural guarantee rather than
// a vibe.
func (m *Maze) edgeDisjointPaths(src, dst Cell, cap int) int {
	n := m.Width * m.Height
	adj := make([][]int, n)
	residual := make(map[[2]int]int)

	addNeighborEdge := func(a, b Cell) {
		ai, bi := m.idx(a), m.idx(b)
		if _, ok := residual[[2]int{ai, bi}]; !ok {
			adj[ai] = append(adj[ai], bi)
			adj[bi] = append(adj[bi], ai)
		}
		residual[[2]int{ai, bi}] = 1
		residual[[2]int{bi, ai}] = 1
	}

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			c := Cell{x, y}
			if m.isOpen(c, East) {
				addNeighborEdge(c, m.neighbor(c, East))
			}
			if m.isOpen(c, South) {
				addNeighborEdge(c, m.neighbor(c, South))
			}
		}
	}

	s, t := m.idx(src), m.idx(dst)
	flow := 0
	for flow < cap {
		parent := make([]int, n)
		for i := range parent {
			parent[i] = -1
		}
		parent[s] = s
		queue := []int{s}
		for len(queue) > 0 && parent[t] == -1 {
			u := queue[0]
			queue = queue[1:]
			for _, v := range adj[u] {
				if residual[[2]int{u, v}] > 0 && parent[v] == -1 {
					parent[v] = u
					queue = append(queue, v)
				}
			}
		}
		if parent[t] == -1 {
			break
		}
		for v := t; v != s; {
			u := parent[v]
			residual[[2]int{u, v}]--
			residual[[2]int{v, u}]++
			v = u
		}
		flow++
	}
	return flow
}
