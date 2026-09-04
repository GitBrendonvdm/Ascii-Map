package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"
)

type runResult struct {
	name     string
	elapsed  time.Duration // informational only - see scoring.go's doc comment for why this no longer feeds the score
	cpuTime  time.Duration // informational only - see scoring.go's doc comment for why this no longer feeds the score
	memBytes int64
	allocs   int64 // heap allocation count (runtime.MemStats.Mallocs delta) - deterministic, feeds the score
	ops      int   // search effort: len(Solution.Edges) - deterministic, informational (see scoring.go for why this doesn't feed the score - span does)
	span     int   // critical-path length: Solution.Span, or ops itself for a solver that leaves Span unset - deterministic, feeds the score
	primOps  int64 // deterministic CPU-cost proxy: Solution.PrimOps - feeds the score, see that field's doc comment
	steps    int   // moves taken (path cell count - 1)

	valid         bool
	invalidReason string
	rendered      string
}

// mapSizePreset keeps the common run sizes named and repeatable. The
// xlarge preset deliberately has enough cells to expose scaling problems in
// a solver or its bookkeeping; use it with -summary-only or a focused style
// while iterating, since the harness runs every registered solver.
func mapSizePreset(name string) (width, height int, ok bool) {
	switch strings.ToLower(name) {
	case "normal":
		return 21, 15, true
	case "large":
		return 100, 100, true
	case "xlarge", "x-large":
		return 250, 250, true
	default:
		return 0, 0, false
	}
}

func main() {
	enableANSI()

	width := flag.Int("width", 21, "maze width in cells")
	height := flag.Int("height", 15, "maze height in cells")
	size := flag.String("size", "normal", "maze-size preset: normal (21x15), large (100x100), or xlarge (250x250)")
	seed := flag.Int64("seed", 42, "random seed (same seed -> same maze)")
	teleporters := flag.Int("teleporters", 2, "number of teleporter pairs")
	braid := flag.Float64("braid", 0.15, "probability of opening an extra wall to create loops (0-1)")
	minPaths := flag.Int("min-routes", 2, "minimum edge-disjoint routes required to the key and to the exit")
	bench := flag.Bool("bench", false, "run every algorithm against all 10 hardcoded benchmark seeds and print a leaderboard")
	summaryOnly := flag.Bool("summary-only", false, "skip printing each solved map; show only the leaderboard(s)")
	noColor := flag.Bool("no-color", false, "disable ANSI color output")
	consoleWidthFlag := flag.Int("console-width", 0, "override detected console width for side-by-side maze panels (0 = auto)")
	style := flag.String("style", "Braided", "maze generation style (-list-styles to see all); -braid/-min-routes only apply to Braided")
	listStyles := flag.Bool("list-styles", false, "print every registered maze style and exit")
	consoleRoutes := flag.Bool("console-routes", false, "also dump every solved maze as ASCII panels to the console (default: write JSON for viewer.html instead)")
	outPath := flag.String("out", "", "path to write the JSON results file (default: bench_results.json for -bench, maze_result.json otherwise)")
	flag.Parse()

	if *noColor {
		colorsEnabled = false
	}

	if *listStyles {
		for _, s := range MazeStyles() {
			fmt.Println(s.Name)
		}
		return
	}

	if *bench {
		runBenchmark(*summaryOnly, *consoleWidthFlag, *consoleRoutes, *outPath)
		return
	}

	presetWidth, presetHeight, ok := mapSizePreset(*size)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown size preset %q; choose normal, large, or xlarge\n", *size)
		os.Exit(1)
	}
	widthSpecified, heightSpecified := false, false
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "width":
			widthSpecified = true
		case "height":
			heightSpecified = true
		}
	})
	// A named size selects both dimensions, but callers can tune either
	// axis explicitly: -size xlarge -width 400, for example, is 400x250.
	if !widthSpecified {
		*width = presetWidth
	}
	if !heightSpecified {
		*height = presetHeight
	}

	if *width < 5 || *height < 5 {
		fmt.Fprintln(os.Stderr, "width and height must be at least 5")
		os.Exit(1)
	}
	if *minPaths < 1 {
		fmt.Fprintln(os.Stderr, "min-routes must be at least 1")
		os.Exit(1)
	}

	var m *Maze
	if *style == "Braided" {
		m = buildMaze(*width, *height, *seed, *teleporters, *braid, *minPaths)
	} else {
		s, ok := mazeStyleByName(*style)
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown style %q; run -list-styles to see the available ones\n", *style)
			os.Exit(1)
		}
		m = s.Generate(*width, *height, *seed, *teleporters)
	}
	fmt.Printf("seed=%d  style=%s  size=%dx%d\n\n", *seed, *style, *width, *height)
	fmt.Print(m.Render())
	fmt.Println()
	fmt.Print(m.Legend())
	fmt.Println()

	results := runAllSolvers(m)
	scored := scoreResults(results)
	printLeaderboard("Leaderboard", results)

	if *consoleRoutes {
		printPanelsOrSummary(results, *summaryOnly, resolveConsoleWidth(*consoleWidthFlag))
	}

	path := *outPath
	if path == "" {
		path = "maze_result.json"
	}
	jm := buildJSONMazeWithSolutions(*seed, *style, m, results, scored)
	group := jsonSizeGroup{Width: m.Width, Height: m.Height, Mazes: []jsonMaze{jm}}
	directionBits := jsonDirection{North: North, South: South, East: East, West: West}
	if err := writeBenchExportNDJSON(path, directionBits, []jsonSizeGroup{group}, nil); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", path, err)
	} else {
		fmt.Printf("Full results + animated route data written to %s - open viewer.html in a browser to explore.\n", path)
	}
}

// buildMaze runs the full generation pipeline: carve, braid, place start/
// key/exit, guarantee redundant routes, then place teleporters without
// breaking solvability.
func buildMaze(width, height int, seed int64, teleporters int, braid float64, minPaths int) *Maze {
	m := NewMaze(width, height, seed)
	m.GeneratePerfectMaze()
	m.Braid(braid)
	m.PlacePoints()
	m.EnsureRedundantRoutes(m.Start, m.Key, m.Exit, minPaths)
	m.PlaceTeleporters(teleporters)
	return m
}

// timingRuns is how many times each algorithm re-solves the same maze to
// produce its reported time and CPU time. A single Solve() call on a small
// maze routinely finishes faster than the OS clock can reliably resolve (it
// reads as a flat 0s), which defeats the entire point of a speed
// leaderboard - averaging over many repeats gives a real, comparable
// number instead. These stay purely informational now (see scoring.go) -
// they're still worth reporting since they're the closest thing to "how
// this would actually feel," but a shared machine's own business (whatever
// else is competing for the CPU that moment) can and does swing them run to
// run, which is exactly what makes them unfit to decide rank.
const timingRuns = 200

// runAllSolvers hands every registered algorithm its own untouched clone of
// the maze, measures it, and independently validates what comes back before
// trusting it. Two different kinds of measurement happen here, and they're
// used for two different purposes:
//
//   - allocs and memBytes (runtime.MemStats deltas around one Solve() call)
//     and ops (len(Solution.Edges), search effort) are a pure function of
//     the code path taken for this exact maze - same maze, same algorithm,
//     same numbers every time, on any machine, busy or idle. These are what
//     scoreResults actually scores on.
//   - elapsed and cpuTime (averaged over many repeats, to get above the
//     OS clock's resolution floor) reflect real wall-clock/CPU behavior,
//     which is exactly what makes them useful as a "does this feel fast"
//     sanity check and exactly what makes them unsuitable to rank on: how
//     busy the machine happens to be at that moment shows up directly in
//     the number.
func runAllSolvers(m *Maze) []runResult {
	var results []runResult
	for _, solver := range Solvers() {
		clone := m.Clone()

		// One call to capture the actual answer to validate/render - also
		// used to measure allocs/memBytes, since runtime.ReadMemStats has
		// its own non-trivial overhead unsuitable for the tight timing
		// loop below, and since a single call's allocation profile is
		// already exactly reproducible (no averaging needed for a
		// deterministic quantity).
		var memBefore, memAfter runtime.MemStats
		runtime.ReadMemStats(&memBefore)
		sol := solver.Solve(clone)
		runtime.ReadMemStats(&memAfter)
		memBytes := int64(memAfter.TotalAlloc - memBefore.TotalAlloc)
		allocs := int64(memAfter.Mallocs - memBefore.Mallocs)

		// Repeat runs, reusing the same clone (all shipped solvers are
		// read-only over the maze), purely to get wall-clock and CPU-time
		// measurements finer than a single call's resolution. Informational
		// only - see this function's doc comment.
		cpuBefore := processCPUTime()
		start := time.Now()
		for i := 0; i < timingRuns; i++ {
			solver.Solve(clone)
		}
		elapsed := time.Since(start) / timingRuns
		cpuElapsed := (processCPUTime() - cpuBefore) / time.Duration(timingRuns)

		ops := len(sol.Edges)
		// Span defaults to 0 on Solution - the sentinel a single-threaded
		// solver leaves unset, meaning "every discovery was sequentially
		// dependent on the last, so span equals total work" (see
		// Solution.Span's doc comment). Only a genuinely concurrent
		// solver (BrenThread, BrenThreadOptimized) reports a real,
		// smaller span.
		span := sol.Span
		if span == 0 {
			span = ops
		}

		res := runResult{
			name:     solver.Name(),
			elapsed:  elapsed,
			cpuTime:  cpuElapsed,
			memBytes: memBytes,
			allocs:   allocs,
			ops:      ops,
			span:     span,
			primOps:  sol.PrimOps,
			steps:    len(sol.Path) - 1,
		}
		if err := ValidatePath(m, sol.Path); err != nil {
			res.valid = false
			res.invalidReason = err.Error()
		} else {
			res.valid = true
			res.rendered = m.RenderPath(sol.Path, sol.Visited)
		}
		results = append(results, res)
	}
	return results
}

// formatBytes renders a byte count in whatever unit reads most naturally.
func formatBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.2fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.2fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// printPanelsOrSummary shows each algorithm's result either as a plain text
// line (summaryOnly) or as a solved-maze panel, packed side by side with
// its peers to use the console's width instead of dumping every maze in
// one long vertical column.
func printPanelsOrSummary(results []runResult, summaryOnly bool, consoleWidth int) {
	if summaryOnly {
		for _, r := range results {
			if !r.valid {
				fmt.Printf("%-20s DISQUALIFIED: %s\n", r.name, r.invalidReason)
				continue
			}
			fmt.Printf("%-20s steps: %-6d time: %-14s cpu: %-14s mem: %s\n",
				r.name, r.steps, r.elapsed, r.cpuTime, formatBytes(r.memBytes))
		}
		fmt.Println()
		return
	}

	panels := make([]panel, len(results))
	for i, r := range results {
		panels[i] = buildPanel(r)
	}
	printPanelsGrid(panels, consoleWidth)
}

// printLeaderboard ranks algorithms by combined score (see scoring.go) and
// calls out the run's shortest route and fastest solve up front, so those
// headline numbers are visible even if the per-algorithm output above has
// scrolled off.
func printLeaderboard(title string, results []runResult) {
	scored := scoreResults(results)

	fmt.Printf("--- %s (score = geometric mean of steps/span/primOps/allocs/mem ratios vs. the best on this maze; 1.0 = best in everything, smallest first; ops/time/cpu below are informational only - see scoring.go) ---\n", title)

	if len(scored) > 0 {
		minSteps := scored[0].steps
		minElapsed := scored[0].elapsed
		for _, r := range scored {
			if r.steps < minSteps {
				minSteps = r.steps
			}
			if r.elapsed < minElapsed {
				minElapsed = r.elapsed
			}
		}
		var shortestBy, fastestBy []string
		for _, r := range scored {
			if r.steps == minSteps {
				shortestBy = append(shortestBy, r.name)
			}
			if r.elapsed == minElapsed {
				fastestBy = append(fastestBy, r.name)
			}
		}
		fmt.Printf("Shortest route this run: %d steps (%s)\n", minSteps, strings.Join(shortestBy, ", "))
		fmt.Printf("Fastest solve this run (informational, not scored): %s (%s)\n\n", minElapsed, strings.Join(fastestBy, ", "))
	}

	for i, r := range scored {
		fmt.Printf("%2d. %-20s score %6.2f   %6d steps   %6d span   %8d primOps   %6d allocs   mem %10s   (ops %d, time %s, cpu %s)\n",
			i+1, r.name, r.score, r.steps, r.span, r.primOps, r.allocs, formatBytes(r.memBytes), r.ops, r.elapsed, r.cpuTime)
	}
	disqualified := len(results) - len(scored)
	if disqualified > 0 {
		fmt.Printf("(%d algorithm(s) disqualified - see output above)\n", disqualified)
	}
	fmt.Println()
}

type seedRun struct {
	seed    int64
	style   string
	maze    *Maze
	results []runResult
}

type aggregate struct {
	name       string
	avgScore   float64
	avgSteps   float64
	avgOps     float64 // total work; informational only - see scoring.go
	avgSpan    float64
	avgPrimOps float64 // deterministic CPU-cost proxy - feeds the score, see Solution.PrimOps
	avgAllocs  float64
	avgMem     float64
	avgTime    time.Duration // informational only - see scoring.go
	avgCPU     time.Duration // informational only - see scoring.go
	wins       int
	runs       int // how many maze runs this average is actually over - 10 for a per-size aggregate, up to 10*len(benchmarkSizeTiers) for the cross-size overall one
}

// runBenchmark always sweeps every tier in benchmarkSizeTiers (see
// seeds.go) and writes them all into ONE export file - one file, not one
// per size, so the viewer's Size selector switches between in-memory
// groups already sitting in the loaded JSON instead of triggering a
// separate fetch. There is deliberately no way to ask for a single size
// here: the whole point is that `-bench` always produces the complete
// picture in one shot, the same way it always solved every registered
// algorithm rather than a chosen subset.
func runBenchmark(summaryOnly bool, consoleWidthOverride int, consoleRoutes bool, outPath string) {
	consoleWidth := resolveConsoleWidth(consoleWidthOverride)

	var sizeGroups []jsonSizeGroup
	var allRuns []seedRun // every seed's results across every tier - overall (cross-size) leaderboard, below the loop, aggregates over this
	for _, tier := range benchmarkSizeTiers {
		fmt.Fprintf(os.Stderr, "=== size %dx%d ===\n", tier.width, tier.height)

		var runsBySeed []seedRun
		for i, seed := range BenchmarkSeeds {
			styleName := BenchmarkStyleNames[i]
			style, ok := mazeStyleByName(styleName)
			if !ok {
				fmt.Fprintf(os.Stderr, "internal error: benchmark style %q is not registered\n", styleName)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "[%d/%d] %s (seed=%d): generating maze...\n", i+1, len(BenchmarkSeeds), styleName, seed)
			m := style.Generate(tier.width, tier.height, seed, BenchTeleporters)
			fmt.Fprintf(os.Stderr, "[%d/%d] %s (seed=%d): running %d algorithms...\n", i+1, len(BenchmarkSeeds), styleName, seed, len(Solvers()))
			runsBySeed = append(runsBySeed, seedRun{seed: seed, style: styleName, maze: m, results: runAllSolvers(m)})
		}
		fmt.Fprintln(os.Stderr, "done.")
		fmt.Fprintln(os.Stderr)
		allRuns = append(allRuns, runsBySeed...)

		agg := aggregateScores(runsBySeed)

		fmt.Printf("=== %dx%d - average score across all %d seeds (score = geometric mean of steps/span/primOps/allocs/mem ratios vs. the best on each maze; smallest first; ops/time/cpu below are informational only - see scoring.go) ===\n", tier.width, tier.height, len(BenchmarkSeeds))
		for i, a := range agg {
			fmt.Printf("%2d. %-20s avg score %6.2f   avg %6.1f steps   avg %7.1f span   avg %9.1f primOps   avg %7.1f allocs   avg mem %10s   won %d/%d   (avg ops %.1f, avg time %s, avg cpu %s)\n",
				i+1, a.name, a.avgScore, a.avgSteps, a.avgSpan, a.avgPrimOps, a.avgAllocs, formatBytes(int64(a.avgMem)), a.wins, a.runs, a.avgOps, a.avgTime, a.avgCPU)
		}
		fmt.Println()

		// Every algorithm's path/visited cells against every one of the 10
		// mazes, plus the layout needed to redraw each maze - this is what
		// lets viewer.html animate solves and compare algorithms side by
		// side, instead of the console dumping ten mazes times N
		// algorithms of ASCII art that scrolls past faster than anyone can
		// read it.
		fmt.Fprintf(os.Stderr, "building export for %dx%d (re-solving each algorithm once per maze to capture path/visited data)...\n", tier.width, tier.height)
		jsonMazes := make([]jsonMaze, len(runsBySeed))
		for i, sr := range runsBySeed {
			scored := scoreResults(sr.results)
			jsonMazes[i] = buildJSONMazeWithSolutions(sr.seed, sr.style, sr.maze, sr.results, scored)
		}
		sizeGroups = append(sizeGroups, jsonSizeGroup{
			Width:       tier.width,
			Height:      tier.height,
			Mazes:       jsonMazes,
			Leaderboard: buildJSONAggregate(agg),
		})

		if !summaryOnly && consoleRoutes {
			fmt.Printf("=== Route visualizer (%dx%d) - each algorithm's solved map for all 10 seeds ===\n", tier.width, tier.height)
			fmt.Println()
			printRouteVisualizer(agg, runsBySeed, consoleWidth)
		}
	}

	// The overall leaderboard aggregates every maze run across every size
	// tier - up to 10*len(benchmarkSizeTiers) runs per algorithm, not just
	// 10. That mixing is only meaningful because score is already a ratio
	// against the best algorithm on that exact same maze (see scoring.go),
	// so it stays comparable across wildly different maze sizes the same
	// way it already does across the 10 differently-styled mazes within
	// one size - unlike the raw steps/span/etc. averages also shown here,
	// which mix scales freely and are informational only, same as within
	// one size tier.
	overallAgg := aggregateScores(allRuns)
	fmt.Printf("=== Overall - average score across all %d maze runs (%d seeds x %d size tiers) ===\n", len(allRuns), len(BenchmarkSeeds), len(benchmarkSizeTiers))
	for i, a := range overallAgg {
		fmt.Printf("%2d. %-20s avg score %6.2f   won %d/%d   (avg %6.1f steps, avg %7.1f span, avg %9.1f primOps, avg %7.1f allocs, avg mem %s - all informational when mixed across sizes)\n",
			i+1, a.name, a.avgScore, a.wins, a.runs, a.avgSteps, a.avgSpan, a.avgPrimOps, a.avgAllocs, formatBytes(int64(a.avgMem)))
	}
	fmt.Println()

	path := outPath
	if path == "" {
		path = "bench_results.json"
	}
	directionBits := jsonDirection{North: North, South: South, East: East, West: West}
	if err := writeBenchExportNDJSON(path, directionBits, sizeGroups, buildJSONAggregate(overallAgg)); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", path, err)
	} else {
		fmt.Printf("Full results + animated route data for all %d size tiers written to %s - open viewer.html in a browser to explore (compare algorithms side by side, animate solves, see cpu/mem).\n\n", len(sizeGroups), path)
	}
}

// printRouteVisualizer is runBenchmark's optional -console-routes dump for
// one size tier: one row per algorithm, that algorithm's solved maze for
// all 10 seeds side by side.
func printRouteVisualizer(agg []aggregate, runsBySeed []seedRun, consoleWidth int) {
	for _, a := range agg {
		fmt.Printf("--- %s ---\n", a.name)
		var panels []panel
		for _, sr := range runsBySeed {
			var res runResult
			for _, r := range sr.results {
				if r.name == a.name {
					res = r
					break
				}
			}
			p := buildPanel(res)
			p.title = fmt.Sprintf("seed=%d [%s]: %s", sr.seed, sr.style, p.title)
			panels = append(panels, p)
		}
		printPanelsGrid(panels, consoleWidth)
	}
}

// aggregateScores scores every seed's results (see scoreResults in
// scoring.go), averages each algorithm's score/steps/span/allocs/mem (the
// deterministic quantities scored on) plus ops/time/cpu (informational
// only) across all seeds it was valid in, and returns them sorted
// best-average-score first. A seed's top scorer counts as a "win" for
// that seed.
func aggregateScores(runsBySeed []seedRun) []aggregate {
	totalScore := map[string]float64{}
	totalSteps := map[string]int{}
	totalOps := map[string]int{}
	totalSpan := map[string]int{}
	totalPrimOps := map[string]int64{}
	totalAllocs := map[string]int64{}
	totalMem := map[string]int64{}
	totalTime := map[string]time.Duration{}
	totalCPU := map[string]time.Duration{}
	wins := map[string]int{}
	runs := map[string]int{}

	for _, sr := range runsBySeed {
		scored := scoreResults(sr.results)
		for i, r := range scored {
			totalScore[r.name] += r.score
			totalSteps[r.name] += r.steps
			totalOps[r.name] += r.ops
			totalSpan[r.name] += r.span
			totalPrimOps[r.name] += r.primOps
			totalAllocs[r.name] += r.allocs
			totalMem[r.name] += r.memBytes
			totalTime[r.name] += r.elapsed
			totalCPU[r.name] += r.cpuTime
			runs[r.name]++
			if i == 0 {
				wins[r.name]++ // best combined score for this seed
			}
		}
	}

	var agg []aggregate
	for name, n := range runs {
		if n == 0 {
			continue
		}
		agg = append(agg, aggregate{
			name:       name,
			avgScore:   totalScore[name] / float64(n),
			avgSteps:   float64(totalSteps[name]) / float64(n),
			avgOps:     float64(totalOps[name]) / float64(n),
			avgSpan:    float64(totalSpan[name]) / float64(n),
			avgPrimOps: float64(totalPrimOps[name]) / float64(n),
			avgAllocs:  float64(totalAllocs[name]) / float64(n),
			avgMem:     float64(totalMem[name]) / float64(n),
			avgTime:    totalTime[name] / time.Duration(n),
			avgCPU:     totalCPU[name] / time.Duration(n),
			wins:       wins[name],
			runs:       n,
		})
	}
	sort.Slice(agg, func(i, j int) bool { return agg[i].avgScore < agg[j].avgScore })
	return agg
}
