package main

import "container/heap"

func init() {
	Register(&AStarSolver{})
}

// AStarSolver is goal-directed best-first search using the Manhattan
// (taxicab) distance to the destination as its heuristic. On an open,
// sparsely-walled maze it typically expands far fewer cells than plain BFS
// because it always prefers moves that look like they're heading toward the
// goal.
//
// Caveat: a teleporter can make the true remaining distance shorter than
// the maze's straight-line Manhattan distance would suggest (a jump can
// cut clean across the grid), which makes the heuristic technically
// inadmissible here. In practice this means A* is usually still very fast
// and usually still optimal, but - unlike BFS and Claudette - it isn't
// *guaranteed* to find the single shortest route on every teleporter
// layout. That trade-off (raw speed vs. a formal optimality guarantee) is
// exactly the kind of thing worth competing over.
type AStarSolver struct{}

func (AStarSolver) Name() string { return "A-Star" }

func (AStarSolver) Solve(m *Maze) Solution {
	leg1, visited1, edges1, primOps1 := aStarPath(m, m.Start, m.Key)
	leg2, visited2, edges2, primOps2 := aStarPath(m, m.Key, m.Exit)
	return Solution{
		Path:    joinLegs(leg1, leg2),
		Visited: append(visited1, visited2...),
		Edges:   append(edges1, edges2...),
		PrimOps: primOps1 + primOps2,
	}
}

func manhattan(a, b Cell) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

type aStarItem struct {
	cell     Cell
	priority int // gScore + heuristic
	index    int
}

type aStarQueue []*aStarItem

func (q aStarQueue) Len() int           { return len(q) }
func (q aStarQueue) Less(i, j int) bool { return q[i].priority < q[j].priority }
func (q aStarQueue) Swap(i, j int)      { q[i], q[j] = q[j], q[i]; q[i].index = i; q[j].index = j }
func (q *aStarQueue) Push(x interface{}) {
	item := x.(*aStarItem)
	item.index = len(*q)
	*q = append(*q, item)
}
func (q *aStarQueue) Pop() interface{} {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[:n-1]
	return item
}

func aStarPath(m *Maze, src, dst Cell) (path, visited []Cell, edges []Edge, primOps int64) {
	lookup := m.TeleportLookup()

	gScore := map[Cell]int{src: 0}
	parent := map[Cell]Cell{}
	visitedSet := map[Cell]bool{}

	pq := &aStarQueue{}
	heap.Init(pq)
	primOps += heapOpCost(pq.Len()) // see Solution.PrimOps: a heap push/pop costs more than a plain queue's
	heap.Push(pq, &aStarItem{cell: src, priority: manhattan(src, dst)})

	for pq.Len() > 0 {
		primOps += heapOpCost(pq.Len())
		cur := heap.Pop(pq).(*aStarItem).cell
		if visitedSet[cur] {
			continue
		}
		visitedSet[cur] = true
		// A cell's parent can be relaxed (re-pointed at a cheaper predecessor)
		// several times before it's ever popped; the edge is only real once
		// the cell is finalized here, using whichever parent won by then - so
		// the tree records one edge per cell, not one per relaxation.
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
			tentative := gScore[cur] + 1
			if best, ok := gScore[landing]; !ok || tentative < best {
				gScore[landing] = tentative
				parent[landing] = cur
				primOps += heapOpCost(pq.Len())
				heap.Push(pq, &aStarItem{cell: landing, priority: tentative + manhattan(landing, dst)})
			}
		}
	}
	return nil, visitedSlice(visitedSet), edges, primOps // unreachable; shouldn't happen for a generator-verified maze
}
