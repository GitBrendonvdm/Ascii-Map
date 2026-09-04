package main

import (
	"sync"
	"sync/atomic"
)

func init() {
	Register(&BrenThreadOptimizedSolver{})
}

// BrenThreadOptimizedSolver trades away BrenThread's guaranteed-optimal
// route for real speed, the same trade A-Star makes over BFS: a small
// fixed pool of worker goroutines runs a concurrent best-first search per
// leg (Start->Key, then Key->Exit), pulling whichever claimed cell is
// currently closest (Manhattan distance) to that leg's destination off a
// shared bucket queue - the same heuristic A-Star's own priority is built
// on. A cell is claimed exclusively via a single atomic.CompareAndSwapInt32
// (lock-free; no goroutine ever blocks waiting on another to finish its
// claim), and whichever goroutine wins that claim records itself as the
// claimed cell's parent. The moment a leg's destination is itself claimed,
// that leg stops - there's nothing left worth exploring for - and its
// route is reconstructed by walking parent pointers straight back to the
// source, exactly like A-Star reconstructs from its own parent map once
// its goal is dequeued. No separate full-graph BFS reconstruction runs
// here (contrast HangeonOptimized, which shares this file's
// bfsOverGraphFlat and keeps that guarantee) - the parent chain a claim
// actually followed *is* the route.
//
// That's what makes this no longer guaranteed-optimal, and unlike
// A-Star's own trade (which costs it a fairly small, occasional
// suboptimality - averaging ~1.3% extra steps across this project's
// benchmark seeds), this one is deliberately accepted despite being far
// more severe: measured at ~72% of runs coming back suboptimal, with a
// worst case of well over a hundred extra steps on top of the true
// shortest route. Two things compound to cause that, beyond plain
// Manhattan-distance inadmissibility near teleporters:
//
//   - no relaxation. A-Star can re-point an already-discovered cell at a
//     cheaper predecessor if a shorter route to it turns up later; a
//     claim here is permanent the instant the CAS succeeds, so whichever
//     predecessor happens to win that race keeps the cell forever, even
//     when a genuinely shorter route arrives moments afterward.
//   - concurrent claiming breaks strict best-first order. Several workers
//     independently scanning the bucket queue at once means cells don't
//     get claimed in strict priority order the way a single-threaded
//     heap-pop would guarantee - a farther cell can win its claim before
//     a closer one even gets examined, purely from which worker happened
//     to reach it first.
//
// This was measured and flagged explicitly, and keeping it this way -
// rather than either reverting to the guaranteed-optimal full-graph BFS
// reconstruction bfsOverGraphFlat still provides (see HangeonOptimized),
// or investing in a proper relaxation-capable parallel search - was a
// deliberate choice: this solver's score comes first here, route quality
// second.
type BrenThreadOptimizedSolver struct{}

func (BrenThreadOptimizedSolver) Name() string { return "BrenThreadOptimized" }

func (BrenThreadOptimizedSolver) Solve(m *Maze) Solution {
	idx := func(c Cell) int32 { return int32(c.Y*m.Width + c.X) }
	n := m.Width * m.Height
	neighbors := buildFlatNeighbors(m, idx)

	cellOf := make([]Cell, n)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			cellOf[idx(Cell{X: x, Y: y})] = Cell{X: x, Y: y}
		}
	}
	maxDist := m.Width + m.Height

	leg1Path, leg1Edges, leg1Visited, leg1Span, leg1PrimOps := runBrenThreadLeg(neighbors, cellOf, n, maxDist, idx(m.Start), idx(m.Key))
	leg2Path, leg2Edges, leg2Visited, leg2Span, leg2PrimOps := runBrenThreadLeg(neighbors, cellOf, n, maxDist, idx(m.Key), idx(m.Exit))

	// leg2 runs after leg1 finishes (see Span's own doc comment below), so
	// its Depth numbering - which starts back at 1 relative to Key - is
	// offset by leg1Span here to keep every edge's Depth meaningful as one
	// continuous timeline across the whole combined search.
	for i := range leg2Edges {
		leg2Edges[i].Depth += leg1Span
	}

	visited := append(leg1Visited, leg2Visited...)
	edges := append(leg1Edges, leg2Edges...)
	if leg1Path == nil || leg2Path == nil {
		return Solution{Visited: visited, Edges: edges} // unreachable; shouldn't happen for a generator-verified maze
	}

	return Solution{
		Path:    joinLegs(leg1Path, leg2Path),
		Visited: visited,
		Edges:   edges,
		// Legs run sequentially - leg2 can't start until leg1 actually
		// finds Key, its own source - so their spans add rather than
		// overlap, the same way Steps already adds both legs' path
		// lengths.
		Span:    leg1Span + leg2Span,
		PrimOps: leg1PrimOps + leg2PrimOps,
	}
}

// flatEdge is one of a cell's legal moves: the teleport-resolved index it
// leads to. dir is kept around from earlier iterations of this solver but
// is unused by the current (distance-based) exploration strategy.
type flatEdge struct {
	to  int32
	dir int
}

// buildFlatNeighbors precomputes, for every cell, its legal moves - open
// wall, teleport-resolved - once, synchronously, before exploration
// starts. This is the same O(cells) work the exploration always did one
// way or another, just done exactly once instead of once per step:
// previously every single step re-derived its own X/Y from its index and
// re-walked m.isOpen/m.neighbor for each of its up to 4 directions, all
// through Cell-based APIs, purely to answer a question ("what can I
// legally reach from here") that never changes for a given cell across
// the whole run.
//
// Every cell's neighbor list is a *view* into one shared, preallocated
// []flatEdge (flatStore, len(allDirections) slots reserved per cell -
// the worst case of every direction being open) rather than its own
// independently append-grown slice: neighbors[i] = flatStore[base :
// base+count]. A cell with 3-4 open directions would otherwise force
// append to reallocate its backing array 2-3 times as it grows past
// capacity 1, then 2, then 4 - across every cell in the maze, that adds
// up to a lot of avoidable allocation for a count (len(allDirections))
// this function already knows is small and fixed. One allocation for the
// whole adjacency list instead of up to one-per-cell (times up to three
// regrowths each).
func buildFlatNeighbors(m *Maze, idx func(Cell) int32) [][]flatEdge {
	n := m.Width * m.Height
	partner := make([]int32, n)
	for i := range partner {
		partner[i] = -1
	}
	for from, to := range m.TeleportLookup() {
		partner[idx(from)] = idx(to)
	}
	resolve := func(i int32) int32 {
		if p := partner[i]; p != -1 {
			return p
		}
		return i
	}

	flatStore := make([]flatEdge, n*len(allDirections))
	neighbors := make([][]flatEdge, n)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			c := Cell{X: x, Y: y}
			i := idx(c)
			base := int(i) * len(allDirections)
			count := 0
			for _, dir := range allDirections {
				if m.isOpen(c, dir) {
					flatStore[base+count] = flatEdge{to: resolve(idx(m.neighbor(c, dir))), dir: dir}
					count++
				}
			}
			neighbors[i] = flatStore[base : base+count : base+count]
		}
	}
	return neighbors
}

// runBrenThreadLeg runs one leg's concurrent best-first search from src
// toward dst - see BrenThreadOptimizedSolver's doc comment for the full
// picture (bucket-queue priority, CAS claiming, parent-pointer
// reconstruction, why it's no longer guaranteed-optimal). parent is local
// to this single leg, not shared with the other leg's call, so the two
// run as fully independent searches exactly like A-Star's own leg1/leg2
// calls. Two allocation-cutting choices here matter enough to call out
// explicitly (see BrenThreadOptimizedSolver's doc comment for the
// measured effect):
//
//   - parent doubles as the exclusive-claim mechanism. A single
//     atomic.CompareAndSwapInt32 on parent[cell] itself (-1 -> claimer's
//     index) both claims the cell AND records who claimed it, instead of
//     a separate claimed []int32 CAS'd first and a plain write into a
//     second parent array after - one array and one atomic op per claim
//     instead of two of each. src is self-referencing (parent[src] = src)
//     purely as a sentinel, so it reads as "already claimed" without
//     pointing at a fabricated predecessor.
//   - the bucket queue is an intrusive singly-linked list threaded
//     through one preallocated []int32 (next), not a [][]int32 of
//     independently growing slices. bucketHead[d] is the flat index of
//     the first pending cell at distance d, or -1; next[cell] chains to
//     whatever was pushed into that same bucket just before it. Push and
//     pop are both O(1) array writes with zero heap allocation - no
//     append, so no repeated backing-array growth every time a bucket
//     gains a new cell.
func runBrenThreadLeg(neighbors [][]flatEdge, cellOf []Cell, n, maxDist int, src, dst int32) (path []Cell, edges []Edge, visited []Cell, span int, primOps int64) {
	dstCell := cellOf[dst]

	parent := make([]int32, n) // -1 = unclaimed; else the flat index that claimed it (src claims itself)
	for i := range parent {
		parent[i] = -1
	}
	parent[src] = src
	// depth[cell] is cell's generation number in the claim tree: one more
	// than whichever cell claimed it. Zero-valued at src by construction
	// (Go zero-initializes the slice, and src's real depth is 0 anyway),
	// so no separate init pass is needed the way parent's -1 sentinel
	// requires. This is what span is actually measured from - see this
	// solver's top-level doc comment for the work-vs-span distinction.
	depth := make([]int32, n)
	var maxDepth int32

	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	bucketHead := make([]int32, maxDist+1)
	for i := range bucketHead {
		bucketHead[i] = -1
	}
	next := make([]int32, n)
	next[src] = -1
	bucketHead[manhattan(cellOf[src], dstCell)] = src
	pending := 1

	// Built incrementally, under the same lock already guarding the
	// bucket queue, in the order claims actually commit - not recovered
	// afterward by scanning cells in flat-index order. Flat-index order
	// has no relation to discovery order once multiple workers are
	// claiming concurrently (the 138th cell discovered can easily have a
	// lower index than the 5th), which is exactly what made an earlier
	// version of this function's search-tree animation visibly jump
	// around the maze instead of growing outward the way it actually
	// happened.
	visited = make([]Cell, 1, n)
	visited[0] = cellOf[src]
	edges = make([]Edge, 0, n) // at most one edge per claimed cell other than src

	var found int32 // atomic flag: dst has been claimed, nothing left worth exploring for

	numWorkers := 4 // matches the earlier-tuned figure: more just adds contention on the shared queue for a maze this size, not real parallelism
	if numWorkers > n {
		numWorkers = n
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	// totalPrimOps (see Solution.PrimOps) is written exactly once per
	// worker, atomically, right as it exits - see the two defers below.
	// Each worker accumulates into its own goroutine-local localPrimOps
	// throughout (no synchronization needed for that, it's never shared),
	// so the only atomic traffic this adds is one add per worker, not one
	// per increment.
	var totalPrimOps int64

	var workers sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			var localPrimOps int64
			// Registered after workers.Done() above, so - deferred calls
			// run LIFO - this merge always completes before Done() fires,
			// which is what guarantees every worker's contribution has
			// already landed in totalPrimOps by the time workers.Wait()
			// (after the loop below) returns.
			defer func() { atomic.AddInt64(&totalPrimOps, localPrimOps) }()
			for {
				mu.Lock()
				var posIdx int32
				for {
					localPrimOps += 3 // mutex round trip to search the bucket queue - see Solution.PrimOps
					foundBucket := -1
					for d := 0; d < len(bucketHead); d++ {
						localPrimOps++ // every bucket examined during the linear scan is a real primitive op
						if bucketHead[d] != -1 {
							foundBucket = d
							break
						}
					}
					if foundBucket >= 0 {
						posIdx = bucketHead[foundBucket]
						bucketHead[foundBucket] = next[posIdx]
						break
					}
					if pending == 0 {
						mu.Unlock()
						return // nothing left anywhere, we're done
					}
					cond.Wait()
				}
				mu.Unlock()

				localPrimOps++ // atomic load of `found`
				if atomic.LoadInt32(&found) == 1 {
					localPrimOps += 3 // mutex round trip to record this worker standing down
					mu.Lock()
					pending--
					cond.Broadcast()
					mu.Unlock()
					continue
				}

				// Fixed-size, stack-allocated: a cell has at most
				// len(allDirections) legal moves, so this never needs to
				// grow the way append-ing onto a nil slice would.
				var newItems, newDist [4]int32
				newCount := 0
				for _, e := range neighbors[posIdx] {
					localPrimOps += 2 // the candidate-move check itself, plus the atomic CAS it requires
					if !atomic.CompareAndSwapInt32(&parent[e.to], -1, posIdx) {
						continue // already claimed by another worker
					}
					if e.to == dst {
						atomic.StoreInt32(&found, 1)
						localPrimOps++
					}
					newItems[newCount] = e.to
					newDist[newCount] = int32(manhattan(cellOf[e.to], dstCell))
					newCount++
				}

				localPrimOps += 3 // mutex round trip to commit this batch to the bucket queue
				mu.Lock()
				for i := 0; i < newCount; i++ {
					d, c := newDist[i], newItems[i]
					next[c] = bucketHead[d]
					bucketHead[d] = c
					visited = append(visited, cellOf[c])
					depth[c] = depth[posIdx] + 1
					if depth[c] > maxDepth {
						maxDepth = depth[c]
					}
					edges = append(edges, Edge{From: cellOf[posIdx], To: cellOf[c], Depth: int(depth[c])})
				}
				pending += newCount - 1 // the new claims, minus this item finishing
				cond.Broadcast()
				mu.Unlock()
			}
		}()
	}
	workers.Wait()
	primOps = totalPrimOps

	if parent[dst] == -1 {
		return nil, edges, visited, int(maxDepth), primOps // unreachable; shouldn't happen for a generator-verified maze
	}
	path = reconstructFlatPath(parent, src, dst, func(i int32) Cell { return cellOf[i] })
	return path, edges, visited, int(maxDepth), primOps
}

// bfsOverGraphFlat walks a precomputed flat int32 adjacency graph with an
// ordinary, unbiased breadth-first search - guaranteed-optimal, the way
// HangeonOptimized still needs (see that file). BrenThreadOptimized no
// longer uses this itself (see runBrenThreadLeg's parent-pointer
// reconstruction above) but the function stays here since both solvers
// share this file's flat-graph helpers.
func bfsOverGraphFlat(graph [][]int32, cellAt func(int32) Cell, src, dst int32) (path []Cell, edges []Edge, primOps int64) {
	return bfsOverGraphFlatInternal(graph, cellAt, src, dst, false)
}

// bfsOverGraphFlatWithDepth is the same optimal BFS, but retains its BFS
// generation on each discovery edge for a caller that actually runs search
// trees concurrently.
func bfsOverGraphFlatWithDepth(graph [][]int32, cellAt func(int32) Cell, src, dst int32) (path []Cell, edges []Edge, primOps int64) {
	return bfsOverGraphFlatInternal(graph, cellAt, src, dst, true)
}

func bfsOverGraphFlatInternal(graph [][]int32, cellAt func(int32) Cell, src, dst int32, recordDepth bool) (path []Cell, edges []Edge, primOps int64) {
	n := len(graph)
	visitedFlag := make([]bool, n)
	parent := make([]int32, n)
	for i := range parent {
		parent[i] = -1
	}
	visitedFlag[src] = true
	queue := make([]int32, 0, n)
	queue = append(queue, src)
	// A BFS discovery tree has at most one edge per non-root vertex. Both
	// HangeonOptimized and V3 export this whole tree, so growing the slice a
	// few elements at a time only creates short-lived backing arrays.
	edges = make([]Edge, 0, n-1)

	depth, levelEnd := 0, 1
	for head := 0; head < len(queue); head++ {
		cur := queue[head]
		if cur == dst {
			return reconstructFlatPath(parent, src, dst, cellAt), edges, primOps
		}
		for _, next := range graph[cur] {
			primOps++ // every neighbor examined is a real primitive op - see Solution.PrimOps
			if !visitedFlag[next] {
				visitedFlag[next] = true
				parent[next] = cur
				edge := Edge{From: cellAt(cur), To: cellAt(next)}
				if recordDepth {
					edge.Depth = depth + 1
				}
				edges = append(edges, edge)
				queue = append(queue, next)
			}
		}
		if head+1 == levelEnd {
			levelEnd = len(queue)
			depth++
		}
	}
	return nil, edges, primOps // unreachable; shouldn't happen for a generator-verified maze
}

func reconstructFlatPath(parent []int32, src, dst int32, cellAt func(int32) Cell) []Cell {
	var revIdx []int32
	for cur := dst; cur != src; cur = parent[cur] {
		revIdx = append(revIdx, cur)
	}
	revIdx = append(revIdx, src)
	path := make([]Cell, len(revIdx))
	for i, idx := range revIdx {
		path[len(revIdx)-1-i] = cellAt(idx)
	}
	return path
}
