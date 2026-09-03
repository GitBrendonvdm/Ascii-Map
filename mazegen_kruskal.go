package main

func init() {
	RegisterMazeStyle(MazeStyle{Name: "Kruskal", Generate: generateKruskal})
}

// kruskalEdge is a candidate wall between a cell and its neighbor in
// direction dir, used to build the shuffled edge list for randomized
// Kruskal's algorithm below.
type kruskalEdge struct {
	c   Cell
	dir int
}

// kruskalBraidProbability is deliberately lower than Braided style's 0.15 so
// Kruskal's own uniform, scattered texture still shows through after the
// light extra looping pass.
const kruskalBraidProbability = 0.1

// generateKruskal builds a maze with randomized Kruskal's algorithm: every
// wall in the grid is shuffled into a random global order, then walked in
// that order, opening a wall whenever it joins two cells that aren't
// already connected (tracked via union-find). This grows the spanning tree
// from many places across the grid at once rather than from a single
// point, giving a visibly more uniformly scattered texture than DFS
// (Perfect) or Prim's.
func generateKruskal(width, height int, seed int64, teleporters int) *Maze {
	m := NewMaze(width, height, seed)

	var edges []kruskalEdge
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := Cell{x, y}
			for _, dir := range []int{East, South} {
				if m.inBounds(m.neighbor(c, dir)) {
					edges = append(edges, kruskalEdge{c, dir})
				}
			}
		}
	}

	m.rng.Shuffle(len(edges), func(i, j int) { edges[i], edges[j] = edges[j], edges[i] })

	parent := make([]int, width*height)
	for i := range parent {
		parent[i] = i
	}

	var find func(i int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	union := func(a, b int) {
		rootA, rootB := find(a), find(b)
		if rootA != rootB {
			parent[rootA] = rootB
		}
	}

	for _, e := range edges {
		n := m.neighbor(e.c, e.dir)
		a, b := m.idx(e.c), m.idx(n)
		if find(a) != find(b) {
			m.carve(e.c, e.dir)
			union(a, b)
		}
	}

	m.Braid(kruskalBraidProbability)
	m.PlacePoints()
	m.PlaceTeleporters(teleporters)
	return m
}
