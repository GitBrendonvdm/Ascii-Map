package main

import (
	"math"
	"sort"
)

// scoredResult attaches a combined score to a run: the geometric mean of
// five ratios - steps, ops (total discovery work), primOps (a
// deterministic CPU-cost proxy), allocation count (allocs), and memory
// allocated (mem) - each normalized against whichever algorithm did best
// in that ONE dimension on THIS maze. 1.0 means "matched the best in
// every dimension on this maze"; higher is worse.
//
// Deliberately not wall-clock time or CPU time. Both are still measured
// (see runResult) and shown in the viewer as "does this feel fast"
// context, but they depend on how busy the machine running the benchmark
// happens to be at that exact moment - the same algorithm on the same
// maze can and does report a different number every run, purely from OS
// scheduling noise unrelated to the algorithm itself. Steps, ops, primOps,
// allocs, and mem are all a pure function of the code path
// taken: the same maze, solved by the same algorithm, produces the exact
// same numbers every time, on any machine, whether it's idle or swamped
// with other work - which is what makes them fair to actually rank on.
//
// primOps exists specifically to close a real gap the other four
// dimensions leave open: none of steps/ops/allocs/mem charge anything
// for raw looping, branching, or synchronization work that doesn't
// allocate or doesn't change what gets discovered. A goroutine-pool
// solver's lost CAS attempts, mutex round trips, and bucket-queue scans
// are exactly this kind of cost - measured directly on this project's own
// solvers, BrenThreadOptimized's real CPU time ran roughly an order of
// magnitude higher than a comparably-scoring single-threaded solver's,
// entirely invisible to allocs/mem/ops alone (see Solution.PrimOps for
// the exact, deterministic accounting).
//
// Span is deliberately not scored. It is an ideal dependency-depth model,
// not the total work actually performed; it omits worker startup and real
// synchronization overhead. Ops (len(Edges)) charges every discovery,
// regardless of whether it ran serially or concurrently. Span remains
// exported solely to make concurrent discovery generations visible in the
// animation.
//
// Normalizing against the best-per-maze, rather than using raw absolute
// numbers, is what makes a route that's short-but-wasteful and a route
// that's long-but-lean get penalized the same way, and - critically -
// makes averaging the score across ten structurally very different mazes
// fair: a maze that naturally takes 300 steps to solve shouldn't dominate
// a maze that naturally takes 14 just because its raw numbers are bigger.
// Every maze's best performer(s) always score 1.0 on that maze regardless
// of the maze's absolute difficulty, so the average across all ten is a
// genuine average of "how far from best," not an average of raw
// magnitudes.
type scoredResult struct {
	runResult
	stepsRatio   float64
	opsRatio     float64
	primOpsRatio float64
	allocsRatio  float64
	memRatio     float64
	score        float64
}

// scoreResults scores every valid (non-disqualified) result, sorted
// smallest score (best) first, ties broken by raw ops (itself
// deterministic, unlike wall-clock time).
func scoreResults(results []runResult) []scoredResult {
	var valid []runResult
	for _, r := range results {
		if r.valid {
			valid = append(valid, r)
		}
	}
	if len(valid) == 0 {
		return nil
	}

	bestSteps, bestOps, bestPrimOps, bestAllocs, bestMem := valid[0].steps, valid[0].ops, valid[0].primOps, valid[0].allocs, valid[0].memBytes
	for _, r := range valid[1:] {
		if r.steps < bestSteps {
			bestSteps = r.steps
		}
		if r.ops < bestOps {
			bestOps = r.ops
		}
		if r.primOps < bestPrimOps {
			bestPrimOps = r.primOps
		}
		if r.allocs < bestAllocs {
			bestAllocs = r.allocs
		}
		if r.memBytes < bestMem {
			bestMem = r.memBytes
		}
	}
	// Floor every "best" at one unit: a dimension that measured as exactly
	// zero (a trivial maze needing no allocations at all) would otherwise
	// divide by zero, or let every other entry's ratio collapse toward
	// zero for a dimension that was never meaningfully measured.
	if bestSteps < 1 {
		bestSteps = 1
	}
	if bestOps < 1 {
		bestOps = 1
	}
	if bestPrimOps < 1 {
		bestPrimOps = 1
	}
	if bestAllocs < 1 {
		bestAllocs = 1
	}
	if bestMem < 1 {
		bestMem = 1
	}

	scored := make([]scoredResult, len(valid))
	for i, r := range valid {
		steps, ops, primOps, allocs, mem := r.steps, r.ops, r.primOps, r.allocs, r.memBytes
		if steps < 1 {
			steps = 1
		}
		if ops < 1 {
			ops = 1
		}
		if primOps < 1 {
			primOps = 1
		}
		if allocs < 1 {
			allocs = 1
		}
		if mem < 1 {
			mem = 1
		}

		stepsRatio := float64(steps) / float64(bestSteps)
		opsRatio := float64(ops) / float64(bestOps)
		primOpsRatio := float64(primOps) / float64(bestPrimOps)
		allocsRatio := float64(allocs) / float64(bestAllocs)
		memRatio := float64(mem) / float64(bestMem)

		scored[i] = scoredResult{
			runResult:    r,
			stepsRatio:   stepsRatio,
			opsRatio:     opsRatio,
			primOpsRatio: primOpsRatio,
			allocsRatio:  allocsRatio,
			memRatio:     memRatio,
			// Geometric mean of the five ratios: equal weight per
			// dimension, and - unlike a raw product - stays on the same
			// human-readable scale as the ratios themselves (2.0 means
			// "twice as bad overall", not "thirty-two times as bad").
			score: math.Pow(stepsRatio*opsRatio*primOpsRatio*allocsRatio*memRatio, 1.0/5.0),
		}
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score < scored[j].score
		}
		return scored[i].span < scored[j].span
	})
	return scored
}
