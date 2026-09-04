package main

import "testing"

func TestHangeonOptimisedV3FindsValidShortestRoutesWithTeleporters(t *testing.T) {
	solver := HangeonOptimisedV3Solver{}
	baseline := BFSSolver{}
	for _, seed := range []int64{1, 2, 3, 17, 99} {
		m := NewMaze(19, 13, seed)
		m.GeneratePerfectMaze()
		m.PlacePoints()
		m.PlaceTeleporters(3)

		solution := solver.Solve(m)
		if err := ValidatePath(m, solution.Path); err != nil {
			t.Fatalf("seed %d: v3 returned an invalid path: %v", seed, err)
		}
		reference := baseline.Solve(m)
		if len(solution.Path) != len(reference.Path) {
			t.Fatalf("seed %d: v3 path has %d cells, BFS has %d", seed, len(solution.Path), len(reference.Path))
		}
		if solution.Span <= 0 {
			t.Fatalf("seed %d: expected a positive parallel-search span", seed)
		}
		startWave, exitWave, maxDepth := false, false, 0
		for _, edge := range solution.Edges {
			if edge.Depth > maxDepth {
				maxDepth = edge.Depth
			}
			startWave = startWave || (edge.From == m.Start && edge.Depth == 1)
			// The exit-rooted tree is exported in legal forward direction, so
			// its first discovered edges point into Exit rather than out of it.
			exitWave = exitWave || (edge.To == m.Exit && edge.Depth == 1)
		}
		if !startWave || !exitWave {
			t.Fatalf("seed %d: expected simultaneous depth-1 waves from Start and Exit", seed)
		}
		if solution.Span != maxDepth {
			t.Fatalf("seed %d: span=%d, want BFS critical-path depth %d", seed, solution.Span, maxDepth)
		}
	}
}
