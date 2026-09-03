package main

func init() {
	RegisterMazeStyle(MazeStyle{Name: "Rooms", Generate: generateRooms})
}

// room is a rectangular dungeon room, bounds inclusive.
type room struct {
	minX, minY, maxX, maxY int
}

func (r room) center() Cell {
	return Cell{(r.minX + r.maxX) / 2, (r.minY + r.maxY) / 2}
}

// roomSpan picks a room's width or height as 60-100% of the space
// available in its dimension, clamped to at least 3 (or the full budget,
// if that's smaller still) so a slot never produces a degenerate sliver.
func roomSpan(m *Maze, budget int) int {
	if budget < 3 {
		if budget < 1 {
			return 1
		}
		return budget
	}
	minSpan := budget * 3 / 5
	if minSpan < 3 {
		minSpan = 3
	}
	return minSpan + m.rng.Intn(budget-minSpan+1)
}

// generateRooms builds a classic roguelike-dungeon layout: a handful of
// non-overlapping rectangular rooms, each fully carved out internally, then
// linked into one connected structure by L-shaped corridors running between
// room centers. Unlike the whole-grid styles, cells outside every room and
// not on a corridor stay solid rock - unreachable, unused space, which is
// expected here.
func generateRooms(width, height int, seed int64, teleporters int) *Maze {
	m := NewMaze(width, height, seed)

	// Few, large rooms - each one should read as a genuine wide-open space,
	// not a closet. Fewer, bigger rooms also leave less leftover "solid
	// rock" between them, which is what actually shows as maze-like
	// checkerboard texture.
	roomCount := 3 + (width*height)/200
	if roomCount > 7 {
		roomCount = 7
	}

	// Lay rooms out in a coarse cols x rows grid of non-overlapping slots,
	// one room per slot, sized to fill most of it - rather than the
	// obvious "place each room by random retry" approach, which can
	// easily run out of attempts and silently drop rooms once several
	// large rooms have to fit in a small grid without touching (measured:
	// as few as 1-2 of the intended 4 rooms actually landing on some
	// seeds, leaving most of the maze untouched "solid rock" instead of
	// the wide-open spaces this style promises). Slots are disjoint by
	// construction, so every requested room is guaranteed to actually get
	// placed, at a genuinely large size, with zero retries needed.
	cols := 1
	for cols*cols < roomCount {
		cols++
	}
	rows := (roomCount + cols - 1) / cols
	slotW, slotH := width/cols, height/rows

	var rooms []room
	for i := 0; i < roomCount; i++ {
		col, row := i%cols, i/cols
		slotX, slotY := col*slotW, row*slotH
		sw, sh := slotW, slotH
		if col == cols-1 {
			sw = width - slotX // last column/row absorbs the remainder
		}
		if row == rows-1 {
			sh = height - slotY
		}

		// Fill most of the slot (60-100% of what's available after
		// leaving a 1-cell gap at the trailing edge for a corridor to run
		// through), so rooms still vary in size without ever risking
		// collision - there's nothing on the other side of that gap but
		// another slot's own margin.
		w := roomSpan(m, sw-1)
		h := roomSpan(m, sh-1)

		x := slotX + m.rng.Intn(sw-w+1)
		y := slotY + m.rng.Intn(sh-h+1)
		rooms = append(rooms, room{minX: x, minY: y, maxX: x + w - 1, maxY: y + h - 1})
	}

	for _, r := range rooms {
		for y := r.minY; y <= r.maxY; y++ {
			for x := r.minX; x <= r.maxX; x++ {
				c := Cell{x, y}
				if x+1 <= r.maxX {
					m.carve(c, East)
				}
				if y+1 <= r.maxY {
					m.carve(c, South)
				}
			}
		}
	}

	start := rooms[0].center()

	shuffled := append([]room{}, rooms...)
	m.rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	for i := 0; i < len(shuffled)-1; i++ {
		carveCorridor(m, shuffled[i].center(), shuffled[i+1].center())
	}

	m.PlacePointsFrom(start)
	m.PlaceTeleporters(teleporters)
	return m
}

// carveCorridor opens an L-shaped path from a to b: straight along X first,
// then straight along Y, one step at a time via m.carve so every cell along
// the way is properly connected to its neighbor.
func carveCorridor(m *Maze, a, b Cell) {
	c := a
	for c.X != b.X {
		dir := East
		if b.X < c.X {
			dir = West
		}
		m.carve(c, dir)
		c = m.neighbor(c, dir)
	}
	for c.Y != b.Y {
		dir := South
		if b.Y < c.Y {
			dir = North
		}
		m.carve(c, dir)
		c = m.neighbor(c, dir)
	}
}
