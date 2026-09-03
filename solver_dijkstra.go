package main

import "container/heap"

func init() {
	Register(&DijkstraSolver{})
}

// DijkstraSolver is Dijkstra's algorithm: a priority-queue-driven
// best-first search that always expands whichever known cell currently has
// the smallest cumulative distance from the start - with no heuristic
// steering it toward the goal at all, unlike A-Star (which is exactly
// Dijkstra plus one). Every move in this maze costs exactly one step
// (including a forced teleport jump), so Dijkstra is guaranteed optimal
// here for the same underlying reason BFS is (uniform edge weights) - it
// just gets there by maintaining an explicit min-heap keyed on
// distance-so-far, rather than relying on a plain FIFO queue that happens
// to already process cells in distance order only because every edge
// costs the same. On a maze with non-uniform move costs, Dijkstra is the
// algorithm that would still find the shortest route where plain BFS
// couldn't; here it stands as the general-purpose case next to BFS's
// special case.
type DijkstraSolver struct{}

func (DijkstraSolver) Name() string { return "Dijkstra" }

func (DijkstraSolver) Solve(m *Maze) Solution {
	leg1, visited1, edges1, primOps1 := dijkstraPath(m, m.Start, m.Key)
	leg2, visited2, edges2, primOps2 := dijkstraPath(m, m.Key, m.Exit)
	return Solution{
		Path:    joinLegs(leg1, leg2),
		Visited: append(visited1, visited2...),
		Edges:   append(edges1, edges2...),
		PrimOps: primOps1 + primOps2,
	}
}

type dijkstraItem struct {
	cell     Cell
	priority int // cumulative distance from src
	index    int
}

type dijkstraQueue []*dijkstraItem

func (q dijkstraQueue) Len() int           { return len(q) }
func (q dijkstraQueue) Less(i, j int) bool { return q[i].priority < q[j].priority }
func (q dijkstraQueue) Swap(i, j int)      { q[i], q[j] = q[j], q[i]; q[i].index = i; q[j].index = j }
func (q *dijkstraQueue) Push(x interface{}) {
	item := x.(*dijkstraItem)
	item.index = len(*q)
	*q = append(*q, item)
}
func (q *dijkstraQueue) Pop() interface{} {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[:n-1]
	return item
}

func dijkstraPath(m *Maze, src, dst Cell) (path, visited []Cell, edges []Edge, primOps int64) {
	lookup := m.TeleportLookup()

	dist := map[Cell]int{src: 0}
	parent := map[Cell]Cell{}
	visitedSet := map[Cell]bool{}

	pq := &dijkstraQueue{}
	heap.Init(pq)
	primOps += heapOpCost(pq.Len()) // see Solution.PrimOps: a heap push/pop costs more than a plain queue's
	heap.Push(pq, &dijkstraItem{cell: src, priority: 0})

	for pq.Len() > 0 {
		primOps += heapOpCost(pq.Len())
		cur := heap.Pop(pq).(*dijkstraItem).cell
		if visitedSet[cur] {
			continue
		}
		visitedSet[cur] = true
		// A cell's distance can be relaxed (re-pointed at a cheaper
		// predecessor) several times before it's ever popped; the edge is
		// only real once the cell is finalized here, mirroring A-Star's
		// same reasoning - one edge per cell, not one per relaxation.
		if cur != src {
			edges = append(edges, Edge{From: parent[cur], To: cur})
		}
		if cur == dst {
			return reconstructPath(parent, src, dst), visitedSlice(visitedSet), edges, primOps
		}

		for _, dir := range allDirections {
			primOps++ // every direction checked, open or not, is a real primitive op - see Solution.PrimOps
			if !m.isOpen(cur, dir) {
				continue
			}
			landing := m.neighbor(cur, dir)
			if dest, ok := lookup[landing]; ok {
				landing = dest
			}
			tentative := dist[cur] + 1
			if best, ok := dist[landing]; !ok || tentative < best {
				dist[landing] = tentative
				parent[landing] = cur
				primOps += heapOpCost(pq.Len())
				heap.Push(pq, &dijkstraItem{cell: landing, priority: tentative})
			}
		}
	}
	return nil, visitedSlice(visitedSet), edges, primOps // unreachable; shouldn't happen for a generator-verified maze
}
