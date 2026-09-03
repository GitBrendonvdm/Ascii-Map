package main

func init() {
	RegisterMazeStyle(MazeStyle{Name: "Braided", Generate: generateBraided})
}

// defaultBraidProbability and defaultMinRoutes are the knobs the
// single-maze CLI flags (-braid, -min-routes) apply to; the Braided style's
// benchmark/default instance uses these same values.
const (
	defaultBraidProbability = 0.15
	defaultMinRoutes        = 2
)

// generateBraided is the original, default maze style: a perfect
// (spanning-tree) maze with some walls randomly knocked down afterward to
// create loops, then a formal guarantee (via max-flow / Menger's theorem,
// not a guess) of at least defaultMinRoutes edge-disjoint routes to both
// the key and the exit. This is the "multiple valid paths" style.
func generateBraided(width, height int, seed int64, teleporters int) *Maze {
	return buildMaze(width, height, seed, teleporters, defaultBraidProbability, defaultMinRoutes)
}
