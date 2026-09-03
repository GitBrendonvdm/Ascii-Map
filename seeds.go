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

// Fixed dimensions for benchmark mazes, so "the same maps" really means the
// same maps - changing width/height/teleporters would change the layout
// even with an identical seed.
const (
	BenchWidth       = 21
	BenchHeight      = 15
	BenchTeleporters = 3
)
