package main

func init() {
	RegisterMazeStyle(MazeStyle{Name: "Spiral", Generate: generateSpiral})
}

// spiralBranchProbability is the chance, per still-closed internal wall
// (checked East and South only, so every wall is considered exactly once),
// that a shortcut gets carved across the spiral corridor's windings.
const spiralBranchProbability = 0.08

// generateSpiral lays down one long corridor that visits every cell of the
// grid in a shrinking-ring spiral (outside in), then knocks down a handful
// of extra walls at random to add shortcuts across the spiral's windings -
// the "one big winding corridor, with a few loops" style.
func generateSpiral(width, height int, seed int64, teleporters int) *Maze {
	m := NewMaze(width, height, seed)

	order := spiralOrder(width, height)
	for i := 0; i < len(order)-1; i++ {
		cur, next := order[i], order[i+1]
		for _, dir := range allDirections {
			if m.neighbor(cur, dir) == next {
				m.carve(cur, dir)
				break
			}
		}
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := Cell{x, y}
			for _, dir := range []int{East, South} {
				n := m.neighbor(c, dir)
				if !m.inBounds(n) || m.isOpen(c, dir) {
					continue
				}
				if m.rng.Float64() < spiralBranchProbability {
					m.carve(c, dir)
				}
			}
		}
	}

	m.PlacePoints()
	m.PlaceTeleporters(teleporters)
	return m
}

// spiralOrder returns every cell of a width x height grid exactly once, in
// outside-in shrinking-ring spiral order: east along the top edge, south
// down the right edge, west along the bottom edge, north up the left edge,
// then the ring shrinks inward by one on all sides and repeats. This is the
// standard boundary-tracking (top/bottom/left/right) spiral traversal.
func spiralOrder(width, height int) []Cell {
	order := make([]Cell, 0, width*height)
	top, bottom, left, right := 0, height-1, 0, width-1

	for top <= bottom && left <= right {
		for x := left; x <= right; x++ {
			order = append(order, Cell{x, top})
		}
		top++

		for y := top; y <= bottom; y++ {
			order = append(order, Cell{right, y})
		}
		right--

		if top <= bottom {
			for x := right; x >= left; x-- {
				order = append(order, Cell{x, bottom})
			}
			bottom--
		}

		if left <= right {
			for y := bottom; y >= top; y-- {
				order = append(order, Cell{left, y})
			}
			left++
		}
	}

	return order
}
