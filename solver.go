package main

import "sort"

// Solution is what a Solver hands back after searching a maze: just the
// route it found. Rendering ("the completed map") and timing are handled
// centrally by the benchmark harness in main.go, so every algorithm is
// judged on identical, trustworthy terms - a Solver can't pad its own
// numbers by skipping rendering work, and can't make its map look nicer
// than a competitor's by hand-tuning symbols.
type Solution struct {
	// Path is the full route walked: Start ... Key ... Exit, inclusive,
	// with every forced teleport jump represented as consecutive entries.
	Path []Cell

	// Visited is every cell the algorithm's search touched, across both
	// legs, whether or not it ended up on the final Path - dead ends,
	// alternate branches, anything it considered. Optional: leave nil if
	// an algorithm has no way to report this (e.g. it wraps a library
	// whose internal search state isn't exposed). The renderer draws
	// these as a red dot, with Path cells always taking visual priority.
	Visited []Cell

	// Edges is the search's actual discovery tree: one entry per cell in
	// Visited (except the leg's own starting cell), pairing it with
	// whichever cell the search was standing on when it first reached it,
	// in the order the search itself discovered them. This is what lets a
	// viewer draw the real decision tree - which branch led to which,
	// where a branch dead-ended and exploration continued from an earlier
	// fork - instead of just an unordered bag of visited cells. Optional,
	// same rule as Visited: nil if the algorithm has no parent pointers to
	// report.
	Edges []Edge

	// Span is the search's critical-path length: the longest chain of
	// sequentially-dependent discoveries the search actually made (cell B
	// can't be discovered before whichever cell discovered it, cell A, so
	// depth(B) = depth(A)+1) - the length len(Edges) would take even with
	// infinite parallel workers, as opposed to len(Edges) itself, which is
	// total work (every discovery, summed, regardless of how many
	// happened concurrently). For a single-threaded search these are
	// identical - one operation strictly follows the last - so Span
	// defaults to 0, a sentinel main.go's benchmark harness reads as
	// "use len(Edges)". Only a genuinely concurrent solver (see
	// BrenThread, BrenThreadOptimized) needs to report a real, smaller
	// Span: it's the deterministic stand-in for "how much did splitting
	// this work across threads actually buy" that real wall-clock/CPU
	// time used to answer, before this project moved to a fully
	// machine-independent score (see scoring.go).
	Span int

	// PrimOps is a deterministic, machine-independent stand-in for real CPU
	// cost - the one dimension wall-clock/CPU time used to capture that
	// nothing else here does. Steps/Span/Allocs/mem all describe *what* got
	// discovered and how much memory it took; none of them charge for the
	// raw looping, branching, and synchronization work spent getting there,
	// which is exactly where two solvers with near-identical allocs can
	// still have wildly different real CPU time.
	//
	// Every direction checked from a cell - open wall or not, teleport-
	// redirected or not, successful or a wasted attempt - counts as 1
	// PrimOp: the same "every candidate move costs the same regardless of
	// outcome" convention Steps/Ops already use. On top of that baseline, an
	// actual synchronization primitive adds its own real, extra cost no
	// plain memory access pays:
	//
	//   - +1 for a single atomic operation (CAS, atomic load/store)
	//   - +3 for a mutex lock/unlock round trip - a rough, documented
	//     multiplier, not a literal cycle count, representing that a lock
	//     round trip does meaningfully more work than one atomic op
	//   - +heapOpCost(n) for a binary-heap push or pop of a heap currently
	//     holding n items - a stand-in for the real comparisons/swaps a
	//     binary heap needs to maintain order that grows with the heap's
	//     depth, unlike a plain FIFO or bucket queue's flat O(1) push/pop
	//     (see heapOpCost's own doc comment)
	//
	// This is a model, not a profiler: a deterministic proxy for relative
	// CPU cost, reproducible identically on any machine, not a claim that
	// any specific operation takes exactly that many nanoseconds on any
	// specific one.
	PrimOps int64
}

// Edge is one step of a search's discovery tree: the search was standing on
// From when it first reached To (a single legal move, or one forced
// teleport jump - the same "one step" semantics as Path).
type Edge struct {
	From, To Cell

	// Depth is To's generation number in a genuinely concurrent search -
	// the same per-cell notion Span (above) already reduces to a single
	// number, but here kept per-edge: two edges that share a Depth were
	// discovered by different threads running at the same time, not one
	// after another. Defaults to 0, a sentinel a viewer reads as "no
	// concurrency info for this edge - reveal it on its own, in array
	// order" (the only option for a single-threaded search, where an edge
	// can't have been found any time other than strictly after the one
	// before it). Only a genuinely concurrent solver (see BrenThread)
	// needs to report real, >=1 Depth values, letting a viewer draw an
	// entire generation's worth of edges appearing at once - the visual
	// counterpart to Span crediting a parallel search for critical-path
	// length instead of total work.
	Depth int
}

// Solver is the contract every maze-solving algorithm implements. To enter
// a new algorithm into the competition, implement this interface in its own
// solver_*.go file and self-register via init() - see solver_bfs.go for the
// minimal reference example.
type Solver interface {
	// Name is shown on the leaderboard. Keep it short.
	Name() string
	// Solve receives a full, unsolved maze (walls, teleporters, Start, Key,
	// Exit - no route drawn in) and must return the shortest route it can
	// find from Start to Key to Exit, respecting forced teleportation.
	//
	// Solve must not mutate m: the benchmark harness times an algorithm by
	// calling Solve repeatedly against the same maze instance, so a Solve
	// that writes back into m would corrupt its own later timing runs (and
	// is disqualified regardless by ValidatePath re-checking the result
	// against the original, untouched maze).
	Solve(m *Maze) Solution
}

var registry []Solver

// enabledSolverNames keeps benchmark and viewer output focused on the
// hand-built solver families. Reference implementations stay compiled so
// their direct unit tests and implementation notes remain available, but
// they are not entered into a normal run.
var enabledSolverNames = map[string]bool{
	"BrenThread":            true,
	"BrenThreadOptimized":   true,
	"BrenThreadOptimizedV2": true,
	"BrenThreadOptimizedV3": true,
	"Hangeon":               true,
	"HangeonOptimized":      true,
	"HangeonOptimisedV2":    true,
	"HangeonOptimisedV3":    true,
}

// Register adds an enabled algorithm to the competition. Call it from an
// init() in your solver_*.go file.
func Register(s Solver) {
	if !enabledSolverNames[s.Name()] {
		return
	}
	registry = append(registry, s)
}

// Solvers returns every registered algorithm, sorted by name for a stable,
// reproducible run order regardless of file compilation order.
func Solvers() []Solver {
	out := append([]Solver{}, registry...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// joinLegs stitches a start->key leg and a key->exit leg into one route
// without repeating the key cell where they meet.
func joinLegs(startToKey, keyToExit []Cell) []Cell {
	if len(startToKey) == 0 || len(keyToExit) == 0 {
		return nil
	}
	full := append([]Cell{}, startToKey...)
	full = append(full, keyToExit[1:]...)
	return full
}

// visitedSlice converts a visited-set map (the usual way a Cell-keyed
// search tracks what it's explored) into the flat slice Solution.Visited
// expects.
func visitedSlice(set map[Cell]bool) []Cell {
	cells := make([]Cell, 0, len(set))
	for c := range set {
		cells = append(cells, c)
	}
	return cells
}

// heapOpCost is a binary heap push or pop's PrimOps charge (see
// Solution.PrimOps): the heap's height in levels (a size-n heap is a
// complete binary tree floor(log2(n))+1 levels deep) plus 1 for the
// operation itself - a deterministic stand-in for the real number of
// comparisons/swaps a sift-up or sift-down might need to restore the
// ordering invariant. A plain FIFO or bucket queue's flat O(1) push/pop
// pays nothing extra beyond the baseline per-direction PrimOp every solver
// already charges; a binary heap's cost grows with its depth instead. Call
// with the heap's size *before* the push/pop being charged.
func heapOpCost(n int) int64 {
	depth := int64(0)
	for n > 0 {
		depth++
		n >>= 1
	}
	return depth + 1
}
