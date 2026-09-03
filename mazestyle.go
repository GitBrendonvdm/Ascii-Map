package main

import "sort"

// MazeStyle is a pluggable maze-generation algorithm. Every style takes the
// same inputs and must return a fully-formed, playable *Maze - Start, Key,
// Exit set, teleporters placed (or deliberately none), and Start/Key/Exit
// mutually reachable via ordinary walls alone (teleporters layer on top of
// that, they must never be load-bearing for basic connectivity - see
// Maze.PlaceTeleporters, which only ever adds teleporters, never relies on
// them to fix an already-broken maze).
type MazeStyle struct {
	Name     string
	Generate func(width, height int, seed int64, teleporters int) *Maze
}

var mazeStyleRegistry []MazeStyle

// RegisterMazeStyle adds a style to the available set. Call it from an
// init() in your mazegen_*.go file - see mazegen_braided.go and
// mazegen_perfect.go for the two reference examples.
func RegisterMazeStyle(s MazeStyle) {
	mazeStyleRegistry = append(mazeStyleRegistry, s)
}

// MazeStyles returns every registered style, sorted by name for a stable,
// reproducible order regardless of file compilation order.
func MazeStyles() []MazeStyle {
	out := append([]MazeStyle{}, mazeStyleRegistry...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// mazeStyleByName looks up a registered style by exact name. ok is false if
// no such style is registered.
func mazeStyleByName(name string) (MazeStyle, bool) {
	for _, s := range mazeStyleRegistry {
		if s.Name == name {
			return s, true
		}
	}
	return MazeStyle{}, false
}
