package main

func init() {
	RegisterMazeStyle(MazeStyle{Name: "WideCorridors", Generate: generateWideCorridors})
}

// superCell is a coordinate on the super-grid used by generateWideCorridors,
// where each super-cell maps to a 2x2 block of real cells.
type superCell struct{ X, Y int }

// generateWideCorridors builds a maze with genuinely 2-cell-wide passages.
// It runs a randomized DFS (recursive backtracker) over a coarser
// "super-grid" - half the width and height, rounded up - to decide
// connectivity, then blows each super-cell up into a fully-open 2x2 room on
// the real *Maze and opens a full 2-wide boundary between every pair of
// super-cells the DFS connected. The result is corridors and rooms that are
// always at least two real cells wide, instead of the usual single-cell
// passages.
func generateWideCorridors(width, height int, seed int64, teleporters int) *Maze {
	m := NewMaze(width, height, seed)

	superW := (width + 1) / 2
	superH := (height + 1) / 2

	inSuperBounds := func(x, y int) bool {
		return x >= 0 && x < superW && y >= 0 && y < superH
	}

	superOpen := make([][]int, superH)
	visited := make([][]bool, superH)
	for y := range superOpen {
		superOpen[y] = make([]int, superW)
		visited[y] = make([]bool, superW)
	}

	start := superCell{0, 0}
	visited[start.Y][start.X] = true
	stack := []superCell{start}

	dirs := []int{North, South, East, West}

	for len(stack) > 0 {
		cur := stack[len(stack)-1]

		// Shuffle direction order for randomness.
		m.rng.Shuffle(len(dirs), func(i, j int) { dirs[i], dirs[j] = dirs[j], dirs[i] })

		advanced := false
		for _, dir := range dirs {
			d := delta[dir]
			nx, ny := cur.X+d[0], cur.Y+d[1]
			if !inSuperBounds(nx, ny) || visited[ny][nx] {
				continue
			}
			superOpen[cur.Y][cur.X] |= dir
			superOpen[ny][nx] |= opposite[dir]
			visited[ny][nx] = true
			stack = append(stack, superCell{nx, ny})
			advanced = true
			break
		}

		if !advanced {
			stack = stack[:len(stack)-1]
		}
	}

	// Translate the super-grid connectivity into real-cell walls.
	for sy := 0; sy < superH; sy++ {
		for sx := 0; sx < superW; sx++ {
			bx, by := 2*sx, 2*sy
			tl := Cell{bx, by}
			tr := Cell{bx + 1, by}
			bl := Cell{bx, by + 1}
			br := Cell{bx + 1, by + 1}

			// Open every internal wall of this super-cell's 2x2 block so it
			// becomes one fully-open room.
			if m.inBounds(tl) && m.inBounds(tr) {
				m.carve(tl, East)
			}
			if m.inBounds(bl) && m.inBounds(br) {
				m.carve(bl, East)
			}
			if m.inBounds(tl) && m.inBounds(bl) {
				m.carve(tl, South)
			}
			if m.inBounds(tr) && m.inBounds(br) {
				m.carve(tr, South)
			}

			// Open the full 2-wide boundary to every super-cell neighbor the
			// DFS connected.
			if superOpen[sy][sx]&East != 0 {
				if m.inBounds(tr) && m.inBounds(Cell{tr.X + 1, tr.Y}) {
					m.carve(tr, East)
				}
				if m.inBounds(br) && m.inBounds(Cell{br.X + 1, br.Y}) {
					m.carve(br, East)
				}
			}
			if superOpen[sy][sx]&South != 0 {
				if m.inBounds(bl) && m.inBounds(Cell{bl.X, bl.Y + 1}) {
					m.carve(bl, South)
				}
				if m.inBounds(br) && m.inBounds(Cell{br.X, br.Y + 1}) {
					m.carve(br, South)
				}
			}
		}
	}

	m.PlacePoints()
	m.PlaceTeleporters(teleporters)
	return m
}
