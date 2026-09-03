package main

func init() {
	RegisterMazeStyle(MazeStyle{Name: "Cave", Generate: generateCave})
}

// caveBraidProbability is much higher than Braided's defaultBraidProbability
// (0.15) - it knocks down the large majority of extra closed walls, giving
// the maze a very open, "swiss cheese" texture: lots of loops and alternate
// routes, few if any long single-width corridors, more like an open cave
// system than a tight maze.
const caveBraidProbability = 0.55

// generateCave is the deliberately opposite extreme from the "Perfect"
// single-path style: a spanning-tree maze that then gets heavily braided so
// almost every possible loop gets opened up, followed by the same formal
// redundant-routes guarantee every other style uses.
func generateCave(width, height int, seed int64, teleporters int) *Maze {
	m := NewMaze(width, height, seed)
	m.GeneratePerfectMaze()
	m.Braid(caveBraidProbability)
	m.PlacePoints()
	m.EnsureRedundantRoutes(m.Start, m.Key, m.Exit, defaultMinRoutes)
	m.PlaceTeleporters(teleporters)
	return m
}
