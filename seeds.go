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
// Measured real export sizes for the full 10-style-maze sweep at each of
// these: 21x15 ~15MB, 10x10 ~4.6MB, 25x25 ~30MB, 50x50 ~115MB, 100x100
// ~433MB (one combined file is the sum of all of these, so budget for a
// file in the 600MB range). That's real weight to ship and parse
// client-side - measured to still work (100x100 alone parses in a few
// seconds using several hundred MB of browser heap) but it's genuinely
// heavy, which is the tradeoff of a full 10-maze sweep at every size in a
// single file rather than fewer mazes at the larger sizes or separate
// per-size files fetched on demand.
var benchmarkSizeTiers = []sizeTier{
	{width: 21, height: 15},
	{width: 10, height: 10},
	{width: 25, height: 25},
	{width: 50, height: 50},
	{width: 100, height: 100},
}
