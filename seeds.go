package main

// BenchmarkSeeds are fixed so the exact same maze layouts can be regenerated
// and re-run across algorithms/runs/machines for a fair, repeatable
// leaderboard comparison.
var BenchmarkSeeds = [10]int64{
	1337, 2024, 8675309, 90210, 42, 12345, 555, 777888, 99, 314159,
}

// BenchmarkStyleNames pairs each of the 10 benchmark seeds (by index) with
// a distinct maze-generation style, so the 10 benchmark mazes aren't just
// 10 random layouts of the same algorithm - they're 10 structurally
// different kinds of maze, each stressing pathfinding differently: a
// classic multi-route maze, a maze with exactly one path, wide corridors,
// rooms and corridors, two halves joined by a single chokepoint, and
// several more (see the mazegen_*.go files, one per style).
var BenchmarkStyleNames = [10]string{
	"Braided", "Perfect", "WideCorridors", "Rooms", "SplitHalves",
	"Prim", "Kruskal", "Spiral", "Cave", "Winding",
}

// BenchTeleporters is the fixed teleporter-pair count for every benchmark
// maze, at every size in benchmarkSizeTiers - changing it would change the
// layout even with an identical seed, the same reason width and height
// stay fixed per tier rather than per run.
const BenchTeleporters = 3

// sizeTier is one entry in benchmarkSizeTiers.
type sizeTier struct {
	width, height int
}

// benchmarkSizeTiers is every size runBenchmark sweeps, in one -bench
// invocation, into one combined export file (see jsonSizeGroup) - this is
// the full set viewer.html's Size selector offers. 21x15 stays first/
// default since it's the smallest, fastest tier and matches every export
// this project ever shipped before size tiers existed.
//
// The large tiers deliberately remain in this shared export so the viewer's
// size selector can compare the same two V3 solvers at every named size.
// 5000x5000 was tried and dropped - even with the min-cut fix to
// EnsureRedundantRoutes (see flow.go), 25M cells x 10 mazes was too
// resource-intensive to be worth keeping in the regular sweep.
var benchmarkSizeTiers = []sizeTier{
	{width: 21, height: 15},
	{width: 10, height: 10},
	{width: 25, height: 25},
	{width: 50, height: 50},
	{width: 100, height: 100},
	{width: 250, height: 250},
	{width: 400, height: 400},
	{width: 1000, height: 1000},
}
