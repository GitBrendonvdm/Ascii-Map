package main

import (
	"sync"
	"sync/atomic"
)

func init() {
	Register(&BrenThreadOptimizedV3Solver{})
}

// BrenThreadOptimizedV3Solver is BrenThreadOptimizedV2 with the same
// worker pool, the same CAS-claimed cells, the same Manhattan-bucketed
// frontier, the same currentMin fix, the same disclosed no-relaxation
// suboptimality trade (see V1 and V2's own doc comments, both fully
// inherited here) - rewritten around the PrimOps cost V2 still had left
// once its own dominant cost (the from-0 bucket scan) was fixed.
//
// V2 already amortizes the *scan*, but every single frontier item still
// pays for two full mutex round trips of its own: one lock to fetch it
// off the bucket queue, a second lock moments later to commit whatever
// new cells its neighbors produced. On this project's own benchmark,
// that fixed per-item cost - independent of maze size, independent of
// how many neighbors a cell actually has - was measured as roughly 40%
// of V2's remaining PrimOps, and looked like an obvious target: batch
// several frontier cells per lock instead of one.
//
// Two batching attempts were tried first and both were measured, not
// just reasoned about, to make the combined score *worse*: batching the
// commit side (accumulate a whole batch's discoveries, push them all in
// one lock) delayed near-goal discoveries from reaching other workers;
// batching only the fetch side (grab several cells before processing
// any of them) still let a worker commit to farther-bucket cells that a
// strictly one-at-a-time fetch would have skipped once nearer ones
// arrived from its own prior commits. Both widened the search - total
// discoveries (`ops`) rose 60%+ - and that extra PrimOps swamped
// whatever the batched locks saved. Grabbing more than one cell ahead of
// actually processing it, in either direction, trades away exactly the
// tight "always work on the truest current best next" discipline that
// keeps this search narrow, and that discipline matters more to the
// score than the lock savings do.
//
// What ships here doesn't batch multiple cells at all - it merges two
// lock acquisitions that were always going to happen back to back into
// one: committing cell N's discoveries and fetching cell N+1 now share a
// single mutex hold, since nothing meaningful can happen between them
// anyway (no other worker's commit needs to land in that specific gap
// for correctness - see below). One combined lock instead of two per
// cell, with the exact same one-at-a-time processing order, the exact
// same immediate visibility of every discovery to every other worker,
// and the exact same CAS deciding the exact same winner for a given
// cell that V2's own claim would have. This is bookkeeping, not a new
// claim rule or a new ordering: fetching cell N+1 one statement earlier
// than V2 did - immediately after committing N, instead of after a
// second separate lock acquisition - can only ever see the *same or
// fresher* bucket state a separate fetch would have, never staler,
// since no other worker can commit anything in the gap this closes
// (that gap no longer exists to slip into).
//
// Allocs/Mem are unchanged from V2 - same arrays, same sizes, nothing
// new allocated to make the merge work; fetchNextLocked below is a
// closure over already-existing per-goroutine locals, not a new heap
// object of its own (it never escapes the goroutine that creates it).
type BrenThreadOptimizedV3Solver struct{}

func (BrenThreadOptimizedV3Solver) Name() string { return "BrenThreadOptimizedV3" }

func (BrenThreadOptimizedV3Solver) Solve(m *Maze) Solution {
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

	leg1Path, leg1Edges, leg1Visited, leg1Span, leg1PrimOps := runBrenThreadLegV3(neighbors, cellOf, n, maxDist, idx(m.Start), idx(m.Key))
	leg2Path, leg2Edges, leg2Visited, leg2Span, leg2PrimOps := runBrenThreadLegV3(neighbors, cellOf, n, maxDist, idx(m.Key), idx(m.Exit))

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

// runBrenThreadLegV3 is runBrenThreadLegV2 (solver_brenthread_optimized_v2.go)
// with a cell's commit and the next cell's fetch merged into one lock -
// see this file's own top-level doc comment for why. CAS claiming,
// one-at-a-time processing order, depth tracking, and per-worker PrimOps
// accumulation are otherwise unchanged from V2.
func runBrenThreadLegV3(neighbors [][]flatEdge, cellOf []Cell, n, maxDist int, src, dst int32) (path []Cell, edges []Edge, visited []Cell, span int, primOps int64) {
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
			var newItems, newDist [4]int32

			// fetchNextLocked must be called with mu already held. It
			// returns the next frontier cell (popping it, same as V2),
			// or (0, false) once pending reaches 0 with nothing left to
			// find anywhere. Callers merge this into whatever lock they
			// were already taking for other bookkeeping (a commit, or a
			// stand-down) instead of taking a second one - see this
			// file's doc comment.
			fetchNextLocked := func() (int32, bool) {
				for {
					localPrimOps += 3 // one mutex round trip per attempt - see this file's doc comment
					for currentMin < numBuckets && bucketHead[currentMin] == -1 {
						localPrimOps++ // each bucket advanced past is a real primitive op, same as V2
						currentMin++
					}
					if currentMin < numBuckets {
						posIdx := bucketHead[currentMin]
						bucketHead[currentMin] = next[posIdx]
						return posIdx, true
					}
					if pending == 0 {
						return 0, false
					}
					cond.Wait()
				}
			}

			mu.Lock()
			posIdx, ok := fetchNextLocked()
			mu.Unlock()
			if !ok {
				return // nothing to do at all for this leg (shouldn't happen - src always seeds one bucket)
			}

			for {
				localPrimOps++ // atomic load of `found`
				if atomic.LoadInt32(&found) == 1 {
					mu.Lock()
					pending--
					cond.Broadcast()
					posIdx, ok = fetchNextLocked() // merged with the stand-down's own lock - see this file's doc comment
					mu.Unlock()
					if !ok {
						return
					}
					continue
				}

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

				mu.Lock()
				for j := 0; j < newCount; j++ {
					d, c := newDist[j], newItems[j]
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
				posIdx, ok = fetchNextLocked() // merged with this commit's own lock - see this file's doc comment
				mu.Unlock()
				if !ok {
					return
				}
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
