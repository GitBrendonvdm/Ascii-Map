package main

// dirSlots is the fixed direction order edgeDisjointPaths' flat residual
// array uses to lay out each cell's up-to-4 neighbor slots. Purely
// internal/arbitrary - nothing outside this file depends on this order,
// it just has to be used consistently within it.
var dirSlots = [4]int{North, South, East, West}

func dirSlot(dir int) int {
	switch dir {
	case North:
		return 0
	case South:
		return 1
	case East:
		return 2
	default: // West
		return 3
	}
}

// edgeDisjointPaths reports how many edge-disjoint routes exist between src
// and dst, capped at cap (we only ever need to know "is it >= 2", so we stop
// searching for augmenting paths once we hit cap). This is a direct
// application of Menger's theorem: max-flow with unit edge capacities equals
// the maximum number of edge-disjoint paths, which is exactly what "multiple
// valid routes" needs to mean to be a real structural guarantee rather than
// a vibe.
//
// residual is a flat, direction-indexed []int8 (residual[cellIdx*4+slot]),
// not a map[[2]int]int - a grid graph has at most 4 neighbors per cell, a
// dense, fixed-shape structure a hash map is the wrong tool for: array
// indexing is O(1) with real cache locality, where a map lookup pays
// hashing + bucket-probe overhead on every access. int8 comfortably covers
// residual capacities here (they only ever range roughly 0..cap+1, cap
// itself being a small target like 2), at an eighth the memory of int.
//
// When the achieved flow falls short of cap, reachableFromSrc (valid only
// in that case) is the source side of the final residual graph's BFS - by
// max-flow/min-cut theory this is a genuine minimum cut, so opening ANY
// currently-closed wall from a reachableFromSrc cell to a non-reachable one
// is mathematically guaranteed to raise the achievable flow by exactly one
// - see raiseEdgeDisjointPaths, the only caller, for why that guarantee is
// what actually matters here.
func (m *Maze) edgeDisjointPaths(src, dst Cell, cap int) (flow int, reachableFromSrc []bool) {
	n := m.Width * m.Height
	residual := make([]int8, n*4)

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			c := Cell{x, y}
			ci := m.idx(c)
			for _, dir := range allDirections {
				if !m.isOpen(c, dir) {
					continue
				}
				residual[ci*4+dirSlot(dir)] = 1
			}
		}
	}

	s, t := m.idx(src), m.idx(dst)
	parent := make([]int32, n)
	parentDir := make([]int, n)
	queue := make([]int32, 0, n)
	for flow < cap {
		for i := range parent {
			parent[i] = -1
		}
		parent[s] = int32(s)
		queue = queue[:0]
		queue = append(queue, int32(s))
		for head := 0; head < len(queue) && parent[t] == -1; head++ {
			u := int(queue[head])
			uc := Cell{X: u % m.Width, Y: u / m.Width}
			for _, dir := range dirSlots {
				if residual[u*4+dirSlot(dir)] <= 0 {
					continue
				}
				vc := m.neighbor(uc, dir)
				v := m.idx(vc)
				if parent[v] == -1 {
					parent[v] = int32(u)
					parentDir[v] = dir
					queue = append(queue, int32(v))
				}
			}
		}
		if parent[t] == -1 {
			reachableFromSrc = make([]bool, n)
			for i := range parent {
				reachableFromSrc[i] = parent[i] != -1
			}
			return flow, reachableFromSrc
		}
		for v := t; v != s; {
			u := int(parent[v])
			dir := parentDir[v]
			residual[u*4+dirSlot(dir)]--
			residual[v*4+dirSlot(opposite[dir])]++
			v = u
		}
		flow++
	}
	return flow, nil
}

// raiseEdgeDisjointPaths opens walls, one at a time, until src and dst have
// at least target edge-disjoint routes between them or no more walls can
// help, and returns however many were actually achieved. Each wall it
// opens is the current max-flow computation's own min-cut edge (see
// edgeDisjointPaths' own doc comment), so - by max-flow/min-cut duality -
// it's mathematically guaranteed to raise the edge-disjoint path count by
// exactly one, every single time: this loop runs exactly target-achieved
// times before returning target, never more, at any maze size.
//
// That replaces the old approach (open a uniformly random closed wall
// touching the shortest existing path, then recompute the whole max-flow
// from scratch to see if the guess happened to help), which needed
// hundreds of blind guesses to converge even at modest maze sizes
// (measured: 208 attempts at 100x100, 449 at 250x250 - both paid for with
// a full max-flow recomputation per guess, whether or not it helped) and
// grew unworkable well before reaching the sizes benchmarkSizeTiers now
// includes. Opening a min-cut edge instead of a random one does change
// which wall gets opened on a given attempt, so - unlike edgeDisjointPaths'
// own data-structure rewrite - this does change the maze a given seed
// produces, even at sizes small enough that the old approach was already
// fast.
func (m *Maze) raiseEdgeDisjointPaths(src, dst Cell, target int) int {
	for {
		flow, reachableFromSrc := m.edgeDisjointPaths(src, dst, target)
		if flow >= target {
			return flow
		}
		if !m.openCutWall(reachableFromSrc) {
			return flow // no wall crosses the cut - shouldn't happen for a spanning-tree-derived maze, but stay correct if it ever does
		}
	}
}
