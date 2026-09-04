package main

// HangeonOptimisedV3Solver builds its immutable graph before beginning either
// search. It then grows the Start->Key and Exit<-Key decision trees at the
// same time. The exit-side tree walks the transposed graph, so reversing its
// parent chain still produces legal forward moves even when teleporters make
// the graph directed.
type HangeonOptimisedV3Solver struct{}

func init() { Register(&HangeonOptimisedV3Solver{}) }

func (HangeonOptimisedV3Solver) Name() string { return "HangeonOptimisedV3" }

type hangeonOptimisedV3Graph struct {
	forward [][]int32
	reverse [][]int32
}

type hangeonOptimisedV3Leg struct {
	path    []Cell
	edges   []Edge
	primOps int64
}

func (HangeonOptimisedV3Solver) Solve(m *Maze) Solution {
	idx := func(c Cell) int32 { return int32(c.Y*m.Width + c.X) }
	cellAt := func(i int32) Cell { return Cell{X: int(i) % m.Width, Y: int(i) / m.Width} }

	// Graph construction is deliberately a separate phase. Neither decision
	// tree can observe a partially built adjacency list.
	graphReady := make(chan hangeonOptimisedV3Graph, 1)
	go func() { graphReady <- buildHangeonOptimisedV3Graph(m, idx) }()
	graph := <-graphReady

	startTree := make(chan hangeonOptimisedV3Leg, 1)
	exitTree := make(chan hangeonOptimisedV3Leg, 1)
	go func() {
		path, edges, primOps := bfsOverGraphFlatWithDepth(graph.forward, cellAt, idx(m.Start), idx(m.Key))
		startTree <- hangeonOptimisedV3Leg{path: path, edges: edges, primOps: primOps}
	}()
	go func() {
		path, edges, primOps := bfsOverReverseGraphFlat(graph.reverse, cellAt, idx(m.Exit), idx(m.Key))
		exitTree <- hangeonOptimisedV3Leg{path: path, edges: edges, primOps: primOps}
	}()

	fromStart, fromExit := <-startTree, <-exitTree
	startSpan := hangeonOptimisedV3EdgeSpan(fromStart.edges)
	exitSpan := hangeonOptimisedV3EdgeSpan(fromExit.edges)
	edges := append(fromStart.edges, fromExit.edges...)
	visited := make([]Cell, 0, len(edges)+2)
	visited = append(visited, m.Start)
	for _, edge := range fromStart.edges {
		visited = append(visited, edge.To)
	}
	// A reverse-tree edge is presented in its legal forward orientation,
	// making From (rather than To) the cell newly discovered by that tree.
	visited = append(visited, m.Exit)
	for _, edge := range fromExit.edges {
		visited = append(visited, edge.From)
	}
	if fromStart.path == nil || fromExit.path == nil {
		return Solution{Visited: visited, Edges: edges, PrimOps: fromStart.primOps + fromExit.primOps}
	}
	return Solution{
		Path:    joinLegs(fromStart.path, fromExit.path),
		Visited: visited,
		Edges:   edges,
		// The two post-graph-build decision trees are independent. Their
		// critical paths overlap, so the larger BFS generation depth is the
		// search span (not the combined number of discovered cells).
		Span:    max(startSpan, exitSpan),
		PrimOps: fromStart.primOps + fromExit.primOps,
	}
}

func buildHangeonOptimisedV3Graph(m *Maze, idx func(Cell) int32) hangeonOptimisedV3Graph {
	forward := buildHangeonGraphFlat(m, idx)
	reverse := make([][]int32, len(forward))
	degree := make([]int, len(forward))
	for _, neighbors := range forward {
		for _, to := range neighbors {
			degree[to]++
		}
	}
	store := make([]int32, len(forward)*len(allDirections))
	offset := 0
	for i, count := range degree {
		reverse[i] = store[offset : offset+count : offset+count]
		offset += count
	}
	next := make([]int, len(forward))
	for from, neighbors := range forward {
		for _, to := range neighbors {
			reverse[to][next[to]] = int32(from)
			next[to]++
		}
	}
	return hangeonOptimisedV3Graph{forward: forward, reverse: reverse}
}

// bfsOverReverseGraphFlat starts at Exit on the transposed graph. Its path
// and exported edges are reversed back into the maze's legal forward
// direction (Key->Exit) before being returned.
func bfsOverReverseGraphFlat(graph [][]int32, cellAt func(int32) Cell, src, dst int32) (path []Cell, edges []Edge, primOps int64) {
	path, backwardEdges, primOps := bfsOverGraphFlatWithDepth(graph, cellAt, src, dst)
	if path == nil {
		return nil, backwardEdges, primOps
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	edges = make([]Edge, len(backwardEdges))
	for i, edge := range backwardEdges {
		edges[i] = Edge{From: edge.To, To: edge.From, Depth: edge.Depth}
	}
	return path, edges, primOps
}

func hangeonOptimisedV3EdgeSpan(edges []Edge) int {
	span := 0
	for _, edge := range edges {
		if edge.Depth > span {
			span = edge.Depth
		}
	}
	return span
}
