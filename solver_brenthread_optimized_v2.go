package main

import (
	"sync"
	"sync/atomic"
)

func init() {
	Register(&BrenThreadOptimizedV2Solver{})
}

// BrenThreadOptimizedV2Solver is BrenThreadOptimized with the same
// threading concept - a small worker pool racing to CAS-claim cells off a
// shared, Manhattan-distance-bucketed frontier, no relaxation, same
// disclosed suboptimality trade (see BrenThreadOptimizedSolver's own doc
// comment, which this shares in full) - rewritten around one specific,
// measured cost: PrimOps (see Solution.PrimOps), a deterministic proxy for
// real CPU cost that this trade never used to account for.
//
// V1's bucket queue finds its next item by scanning bucketHead from index
// 0 every single time any worker looks for work - a real, measured O(
// maxDist) cost paid on every pop attempt, not amortized away, regardless
// of how few of those buckets have had anything in them for a while. On
// this project's own benchmark, that scan was the dominant share of
// BrenThreadOptimized's PrimOps - its real CPU time ran roughly an order
// of magnitude higher than a comparably-scoring single-threaded solver's,
// almost entirely from repeatedly re-walking buckets already known empty.
//
// The fix: currentMin, a single mutex-protected index tracking the
// smallest bucket that might still hold something. Every bucket mutation -
// push or pop - already happens under this file's one shared mutex, so
// maintaining it exactly, right there in the same critical sections, adds
// no synchronization this solver wasn't already paying for and carries no
// correctness risk (this is bookkeeping alongside an already-atomic
// operation, not a second, independently-racing one):
//
//   - a push into bucket d can only ever reveal an *earlier* possible
//     minimum, never miss one already known: if d < currentMin, lower it.
//   - a pop scans forward from currentMin, not 0, advancing currentMin
//     past any bucket found empty along the way. Each such advance is
//     permanent progress - a bucket only gets scanned past once between
//     pushes that reopen it - so the amortized cost per pop collapses
//     from O(maxDist) to O(1) in the common case (a worker usually finds
//     something the instant it looks, since a push just lowered
//     currentMin to exactly where it should look).
//
// Every other allocation and concurrency choice (flat neighbor store,
// merged claim/parent array, intrusive-linked-list buckets, incremental
// edge/visited/depth recording, per-goroutine PrimOps accumulation merged
// with one atomic add each) is identical to V1 and documented there - see
// runBrenThreadLeg and its neighbors in solver_brenthread_optimized.go.
type BrenThreadOptimizedV2Solver struct{}

func (BrenThreadOptimizedV2Solver) Name() string { return "BrenThreadOptimizedV2" }

func (BrenThreadOptimizedV2Solver) Solve(m *Maze) Solution {
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

	leg1Path, leg1Edges, leg1Visited, leg1Span, leg1PrimOps := runBrenThreadLegV2(neighbors, cellOf, n, maxDist, idx(m.Start), idx(m.Key))
	leg2Path, leg2Edges, leg2Visited, leg2Span, leg2PrimOps := runBrenThreadLegV2(neighbors, cellOf, n, maxDist, idx(m.Key), idx(m.Exit))

	// leg2 runs after leg1 finishes, so its Depth numbering - which starts
	// back at 1 relative to Key - is offset by leg1Span here to keep every
	// edge's Depth meaningful as one continuous timeline across the whole
	// combined search.
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
		Span:    leg1Span + leg2Span,
		PrimOps: leg1PrimOps + leg2PrimOps,
	}
}

// runBrenThreadLegV2 is runBrenThreadLeg (solver_brenthread_optimized.go)
// with currentMin replacing the from-0 bucket scan - see this file's own
// top-level doc comment for why. Everything else - CAS claiming, depth
// tracking, incremental edge/visited recording, per-worker PrimOps
// accumulation - is unchanged from V1.
func runBrenThreadLegV2(neighbors [][]flatEdge, cellOf []Cell, n, maxDist int, src, dst int32) (path []Cell, edges []Edge, visited []Cell, span int, primOps int64) {
	dstCell := cellOf[dst]

	parent := make([]int32, n)
	for i := range parent {
		parent[i] = -1
	}
	parent[src] = src
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
	startBucket := int32(manhattan(cellOf[src], dstCell))
	bucketHead[startBucket] = src
	pending := 1

	// currentMin: see this file's top-level doc comment. Starts at the
	// only bucket that could possibly be non-empty yet.
	currentMin := startBucket
	numBuckets := int32(len(bucketHead))

	visited = make([]Cell, 1, n)
	visited[0] = cellOf[src]
	edges = make([]Edge, 0, n)

	var found int32

	numWorkers := 4
	if numWorkers > n {
		numWorkers = n
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	var totalPrimOps int64

	var workers sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			var localPrimOps int64
			defer func() { atomic.AddInt64(&totalPrimOps, localPrimOps) }()
			for {
				mu.Lock()
				var posIdx int32
				for {
					localPrimOps += 3 // mutex round trip to search the bucket queue - see Solution.PrimOps
					// Scan forward from currentMin, never from 0: every
					// bucket below currentMin is already known empty (see
					// the push side below, which only ever lowers it), so
					// nothing is ever missed - and in the common case
					// bucketHead[currentMin] already has something, so
					// this loop runs zero iterations.
					for currentMin < numBuckets && bucketHead[currentMin] == -1 {
						localPrimOps++ // each bucket advanced past is a real primitive op, but - unlike V1 - only ever once per bucket between pushes that reopen it
						currentMin++
					}
					if currentMin < numBuckets {
						posIdx = bucketHead[currentMin]
						bucketHead[currentMin] = next[posIdx]
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
					if d < currentMin {
						currentMin = d // a push can only ever reveal an earlier possible minimum
					}
					visited = append(visited, cellOf[c])
					depth[c] = depth[posIdx] + 1
					if depth[c] > maxDepth {
						maxDepth = depth[c]
					}
					edges = append(edges, Edge{From: cellOf[posIdx], To: cellOf[c], Depth: int(depth[c])})
				}
				pending += newCount - 1
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
