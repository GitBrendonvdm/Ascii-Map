package main

import (
	"math"
	"sort"
)

// scoredResult attaches a combined score to a run: the geometric mean of
// four ratios - steps, span (critical-path length), allocation count
// (allocs), and memory allocated (mem) - each normalized against
// whichever algorithm did best in that ONE dimension on THIS maze. 1.0
// means "matched the best in every dimension on this maze"; higher is
// worse.
//
// Deliberately not wall-clock time or CPU time. Both are still measured
// (see runResult) and shown in the viewer as "does this feel fast"
// context, but they depend on how busy the machine running the benchmark
// happens to be at that exact moment - the same algorithm on the same
// maze can and does report a different number every run, purely from OS
// scheduling noise unrelated to the algorithm itself. Steps, span,
// allocs, and mem are all a pure function of the code path taken: the
// same maze, solved by the same algorithm, produces the exact same
// numbers every time, on any machine, whether it's idle or swamped with
// other work - which is what makes them fair to actually rank on.
//
// Also deliberately not ops (len(Edges), total discoveries made): span is
// used instead precisely because ops conflates two different things for a
// concurrent solver - total work performed (which ops measures) and how
// long that work actually took given real parallelism (which span
// measures, as the longest chain of sequentially-dependent discoveries -
// what len(Edges) would take even with infinite parallel workers). A
// solver that splits the identical total work across four threads
// shouldn't score as if it did four times the work in serial: ops would
// treat it that way, span doesn't. For a single-threaded solver, ops and
// span are the same number (every discovery strictly follows the last),
// so this swap is a no-op there - only a genuinely concurrent solver's
// score changes at all. ops is still measured and shown for context
// (paired with the search-tree animation), just no longer part of Score.
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
	stepsRatio  float64
	spanRatio   float64
	allocsRatio float64
	memRatio    float64
	score       float64
}

// scoreResults scores every valid (non-disqualified) result, sorted
// smallest score (best) first, ties broken by raw span (itself
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

	bestSteps, bestSpan, bestAllocs, bestMem := valid[0].steps, valid[0].span, valid[0].allocs, valid[0].memBytes
	for _, r := range valid[1:] {
		if r.steps < bestSteps {
			bestSteps = r.steps
		}
		if r.span < bestSpan {
			bestSpan = r.span
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
	if bestSpan < 1 {
		bestSpan = 1
	}
	if bestAllocs < 1 {
		bestAllocs = 1
	}
	if bestMem < 1 {
		bestMem = 1
	}

	scored := make([]scoredResult, len(valid))
	for i, r := range valid {
		steps, span, allocs, mem := r.steps, r.span, r.allocs, r.memBytes
		if steps < 1 {
			steps = 1
		}
		if span < 1 {
			span = 1
		}
		if allocs < 1 {
			allocs = 1
		}
		if mem < 1 {
			mem = 1
		}

		stepsRatio := float64(steps) / float64(bestSteps)
		spanRatio := float64(span) / float64(bestSpan)
		allocsRatio := float64(allocs) / float64(bestAllocs)
		memRatio := float64(mem) / float64(bestMem)

		scored[i] = scoredResult{
			runResult:   r,
			stepsRatio:  stepsRatio,
			spanRatio:   spanRatio,
			allocsRatio: allocsRatio,
			memRatio:    memRatio,
			// Geometric mean of the four ratios: equal weight per
			// dimension, and - unlike a raw product - stays on the same
			// human-readable scale as the ratios themselves (2.0 means
			// "twice as bad overall", not "sixteen times as bad").
			score: math.Sqrt(math.Sqrt(stepsRatio * spanRatio * allocsRatio * memRatio)),
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
