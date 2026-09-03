package main

func init() {
	RegisterMazeStyle(MazeStyle{Name: "Prim", Generate: generatePrim})
}

// defaultPrimBraidProbability is deliberately lower than
// defaultBraidProbability (Braided style's 0.15) so Prim's own
// dead-end-heavy, uniformly-branching texture still shows through after a
// light amount of looping.
const defaultPrimBraidProbability = 0.1

// primEdge is a candidate wall on the frontier: from is an already-visited
// cell, to is one of its in-bounds neighbors that was unvisited at the time
// this edge was added (it may since have been visited via another edge).
type primEdge struct {
	from, to Cell
	dir      int
}

// generatePrim builds a maze via randomized Prim's algorithm: growing a
// spanning tree outward from a single visited cell by repeatedly picking a
// uniformly random frontier edge (visited cell -> unvisited neighbor) rather
// than always extending the most recently carved cell (DFS/backtracker).
// This produces many short dead-ends and more uniform branching, a visibly
// different texture from the long winding corridors of the DFS-based
// Braided/Perfect styles.
func generatePrim(width, height int, seed int64, teleporters int) *Maze {
	m := NewMaze(width, height, seed)

	visited := make([][]bool, height)
	for y := range visited {
		visited[y] = make([]bool, width)
	}

	addFrontier := func(frontier []primEdge, c Cell) []primEdge {
		for _, dir := range allDirections {
			n := m.neighbor(c, dir)
			if !m.inBounds(n) || visited[n.Y][n.X] {
				continue
			}
			frontier = append(frontier, primEdge{from: c, to: n, dir: dir})
		}
		return frontier
	}

	start := Cell{0, 0}
	visited[start.Y][start.X] = true
	var frontier []primEdge
	frontier = addFrontier(frontier, start)

	for len(frontier) > 0 {
		i := m.rng.Intn(len(frontier))
		edge := frontier[i]
		frontier[i] = frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]

		if visited[edge.to.Y][edge.to.X] {
			continue
		}

		m.carve(edge.from, edge.dir)
		visited[edge.to.Y][edge.to.X] = true
		frontier = addFrontier(frontier, edge.to)
	}

	m.Braid(defaultPrimBraidProbability)
	m.PlacePoints()
	m.PlaceTeleporters(teleporters)
	return m
}
