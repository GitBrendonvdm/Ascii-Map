package main

func init() {
	RegisterMazeStyle(MazeStyle{Name: "Perfect", Generate: generatePerfect})
}

// generatePerfect is a strict "perfect" maze: a spanning tree with no
// braiding at all, so there is exactly one route between any two cells -
// the "only one path" style. Teleporters are deliberately skipped for this
// style regardless of the requested count: a teleporter is a second,
// forced connection between two cells that could otherwise be far apart,
// which would hand the maze a second route and break the entire premise of
// "exactly one path." Every other style is free to place them; this one
// intentionally isn't.
func generatePerfect(width, height int, seed int64, teleporters int) *Maze {
	m := NewMaze(width, height, seed)
	m.GeneratePerfectMaze()
	m.PlacePoints()
	return m
}
