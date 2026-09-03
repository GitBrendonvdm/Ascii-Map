package main

func init() {
	Register(&BFSSolver{})
}

// BFSSolver is the textbook reference implementation: plain breadth-first
// search with a Cell-keyed map for visited/parent tracking. It's the
// simplest correct algorithm here and always finds a shortest path, since
// every move costs exactly one step (including a forced teleport jump -
// entering a teleporter cell still only counts as the one step it took to
// walk onto it). Other algorithms are judged against this for correctness;
// Claudette (solver_claudette.go) is judged against this for speed.
type BFSSolver struct{}

func (BFSSolver) Name() string { return "BFS" }

func (BFSSolver) Solve(m *Maze) Solution {
	leg1, visited1, edges1 := bfsShortestPath(m, m.Start, m.Key)
	leg2, visited2, edges2 := bfsShortestPath(m, m.Key, m.Exit)
	return Solution{
		Path:    joinLegs(leg1, leg2),
		Visited: append(visited1, visited2...),
		Edges:   append(edges1, edges2...),
	}
}

// bfsShortestPath returns the shortest path, every cell the search visited
// along the way (for the "attempted routes" red-dot overlay - see
// Solution.Visited), and the discovery-order edges of its search tree (see
// Solution.Edges), regardless of whether a given cell ended up on the path.
func bfsShortestPath(m *Maze, src, dst Cell) (path, visited []Cell, edges []Edge) {
	lookup := m.TeleportLookup()

	visitedSet := map[Cell]bool{src: true}
	parent := map[Cell]Cell{}
	queue := []Cell{src}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == dst {
			return reconstructPath(parent, src, dst), visitedSlice(visitedSet), edges
		}
		for _, dir := range allDirections {
			if !m.isOpen(cur, dir) {
				continue
			}
			landing := m.neighbor(cur, dir)
			if dest, ok := lookup[landing]; ok {
				landing = dest
			}
			if !visitedSet[landing] {
				visitedSet[landing] = true
				parent[landing] = cur
				edges = append(edges, Edge{From: cur, To: landing})
				queue = append(queue, landing)
			}
		}
	}
	return nil, visitedSlice(visitedSet), edges // unreachable; shouldn't happen for a generator-verified maze
}

func reconstructPath(parent map[Cell]Cell, src, dst Cell) []Cell {
	path := []Cell{dst}
	for path[len(path)-1] != src {
		cur := path[len(path)-1]
		path = append(path, parent[cur])
	}
	// reverse into src -> dst order
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}
