package main

import "fmt"

// ValidatePath independently re-checks a Solver's returned route against
// the maze's actual rules. No algorithm - however clever, buggy, or
// malicious - gets to have its result trusted, timed, or shown on the
// leaderboard without passing this first. It knows nothing about how the
// path was produced; it only checks the walk itself:
//
//  1. every cell in the path is in bounds
//  2. the path starts at Start and ends at Exit
//  3. the key is visited somewhere along the way
//  4. every single step is a legal move: either through an open wall to a
//     grid-adjacent cell, or a forced teleport jump (stepping onto a
//     teleporter tile must land on its paired cell - not anywhere else,
//     and not nowhere)
//  5. no step is a no-op (staying on the same cell doesn't count as a move)
func ValidatePath(m *Maze, path []Cell) error {
	if len(path) == 0 {
		return fmt.Errorf("empty path")
	}
	for _, c := range path {
		if !m.inBounds(c) {
			return fmt.Errorf("cell %v is out of bounds", c)
		}
	}
	if path[0] != m.Start {
		return fmt.Errorf("path starts at %v, not the maze start %v", path[0], m.Start)
	}
	if path[len(path)-1] != m.Exit {
		return fmt.Errorf("path ends at %v, not the maze exit %v", path[len(path)-1], m.Exit)
	}

	visitedKey := false
	for _, c := range path {
		if c == m.Key {
			visitedKey = true
			break
		}
	}
	if !visitedKey {
		return fmt.Errorf("path never visits the key at %v", m.Key)
	}

	lookup := m.TeleportLookup()
	for i := 0; i+1 < len(path); i++ {
		cur, next := path[i], path[i+1]
		if cur == next {
			return fmt.Errorf("step %d stays on %v instead of moving", i, cur)
		}

		legal := false
		for _, dir := range allDirections {
			if !m.isOpen(cur, dir) {
				continue
			}
			raw := m.neighbor(cur, dir)
			landing := raw
			if dest, ok := lookup[raw]; ok {
				landing = dest
			}
			if landing == next {
				legal = true
				break
			}
		}
		if !legal {
			return fmt.Errorf("step %d: %v -> %v is not a legal move (no open wall or teleport connects them)", i, cur, next)
		}
	}

	return nil
}
