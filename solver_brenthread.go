package main

import (
	"sync"
	"sync/atomic"
)

func init() {
	Register(&BrenThreadSolver{})
}

// BrenThreadSolver starts one goroutine per starting direction (N, E, S, W)
// from Start. From there, every single thread behaves the same way: look
// at every direction from its current cell; for each one that is a legal
// move, claim the destination cell and spawn a brand new thread to
// continue from it if - and only if - no other thread got there first; no
// preference for any particular direction, every viable option gets its
// own thread. Once a thread has spawned a child for every viable option
// from its cell, its job is done and it exits; a thread with zero viable
// options simply crashes (exits without spawning anything). A shared,
// mutex-protected map is the single source of truth for who owns which
// cell, so the mutex guards the whole check-then-claim step as one atomic
// move - two threads can never both claim the same cell, and never spawn
// duplicate exploration from it.
//
// This amounts to a fully parallel flood-fill: because every viable branch
// gets explored by its own thread rather than a single greedy walker
// picking one option, the threads collectively claim every cell reachable
// from Start that's needed to reach both Key and Exit - not just whatever
// one lucky path happens to stumble across. Exploration stops as soon as
// both are claimed (see runBrenThreads); there's no reason to keep racing
// threads into the rest of the maze once nothing left to find can change
// the route.
//
// Hitting a teleporter is forced, exactly like every other algorithm here:
// a thread stepping toward a teleporter tile immediately continues from
// its paired cell, with no choice in the matter and no requirement to have
// used it.
//
// The route is reconstructed purely by walking the claim tree - whichever
// cell won a given cell's claim race is that cell's one and only parent,
// permanently - with no separate BFS or other reconstruction search run
// over any richer view of the discovered structure. That deliberately
// means this is NOT guaranteed optimal: many threads racing outward
// concurrently, with no synchronized notion of "distance so far", can
// easily claim a farther cell before a shorter route to it is discovered
// through a different branch a moment later, and because a claim is
// permanent the instant it's won, there's no mechanism left to ever
// revisit that decision. This is the same class of trade
// `BrenThreadOptimized` makes (see that solver's own doc comment), just
// via plain unsynchronized claiming here instead of a distance-heuristic-
// guided one.
//
// One exploration reaches both Key and Exit, and is allowed to use a
// genuine shortcut between them - a connection through the claim tree
// that doesn't backtrack all the way through Start - wherever the tree
// actually has one. That needs real care: a tree built from teleport-
// assisted claims isn't safely reversible everywhere (see
// keyToExitPath's doc comment for exactly why and how that's handled).
type BrenThreadSolver struct{}

func (BrenThreadSolver) Name() string { return "BrenThread" }

func (BrenThreadSolver) Solve(m *Maze) Solution {
	lookup := m.TeleportLookup()
	parent, exploreEdges, visited, span := runBrenThreads(m, lookup, m.Start, m.Key, m.Exit)

	if _, ok := parent[m.Key]; !ok {
		return Solution{Visited: visited, Edges: exploreEdges} // unreachable; shouldn't happen for a generator-verified maze
	}
	if _, ok := parent[m.Exit]; !ok {
		return Solution{Visited: visited, Edges: exploreEdges} // unreachable; shouldn't happen for a generator-verified maze
	}

	leg1 := reconstructPath(parent, m.Start, m.Key) // Start is always an ancestor of every claimed cell

	leg2, extraVisited, extraEdges, extraSpan := keyToExitPath(m, lookup, parent, m.Start, m.Key, m.Exit)

	// The fallback exploration inside keyToExitPath (if it ran at all) is
	// its own independent runBrenThreads call, rooted at Key - its Depth
	// numbering starts back at 1 relative to Key, not span+1 relative to
	// Start. Offsetting it here keeps every edge's Depth meaningful as one
	// continuous timeline across the whole combined search, matching how
	// Span itself is already combined additively (span + extraSpan) rather
	// than by taking a max: leg2 conceptually happens after leg1 finishes,
	// since Key has to be found before a Key-rooted fallback search can
	// even start.
	for i := range extraEdges {
		extraEdges[i].Depth += span
	}

	return Solution{
		Path:    joinLegs(leg1, leg2),
		Visited: append(visited, extraVisited...),
		Edges:   append(exploreEdges, extraEdges...),
		Span:    span + extraSpan,
	}
}

// keyToExitPath finds the Key->Exit leg through the single tree runBrenThreads
// built from Start, using a genuine shortcut wherever the tree's structure
// actually allows one to be reconstructed safely - and paying for a
// second, independent exploration (rooted at Key) only in the one case
// where it provably can't be.
//
// The constraint: reconstructing a path only ever works by walking a
// chain of claims in the same direction it was originally discovered
// (parent -> child - see reconstructPath). Reading that chain the other
// way round - child up toward an ancestor - happens to give the correct
// *cells*, but only represents a legal sequence of *moves* when every
// edge crossed is safely reversible. An edge parent[cur] -> cur is safely
// reversible (cur can walk straight back to parent[cur]) only when
// parent[cur] is NOT itself a teleporter endpoint: stepping toward any
// registered teleporter cell's position always redirects to its partner,
// regardless of which direction you're approaching from or whether that
// specific edge happened to be an ordinary move - a trigger tile is never
// a resting cell for anyone, including the cell that legitimately moved
// away from it a moment earlier. And a cell that was itself reached via a
// redirect has no general one-step way back at all (its landing position
// usually isn't even geometrically adjacent to whichever cell discovered
// it).
//
// So a cell's localRoot (see that function) - the nearest point above it
// where every edge crossed to get there was safely reversible - marks
// exactly how far it's safe to walk that cell's chain backward. From
// there:
//
//   - If Key is already an ancestor of Exit in the full tree, Exit ->
//     ... -> Key never needed reversing in the first place -
//     reconstructPath handles it directly, teleports and all.
//   - If Key and Exit share the same localRoot, the entire path between
//     them lies inside one such safely-reversible branch, so
//     pathBetween's own ancestor-chain reversal (see that function) is
//     safe by construction - a genuine shortcut, not a walk back through
//     Start.
//   - Otherwise, a shortcut doesn't have to be a tree relationship at
//     all: Start -> ... -> Key and Start -> ... -> Exit are two entirely
//     separate routes through the same maze, and it's entirely possible
//     for a completely ordinary wall opening to directly connect some
//     cell near one route to some cell near the other, without that
//     connection ever having been walked during exploration at all (see
//     findBridge). If one exists within Key's safely-reversible reach,
//     it's used - a genuine cross-route shortcut, still needing no
//     extra exploration.
//   - Only if none of the above apply does this fall back to a second,
//     independent flood-fill rooted at Key, paying for that extra
//     exploration only in this one case.
func keyToExitPath(m *Maze, lookup map[Cell]Cell, parent map[Cell]Cell, root, key, exit Cell) (path []Cell, visited []Cell, edges []Edge, span int) {
	if isAncestor(parent, root, key, exit) {
		return reconstructPath(parent, key, exit), nil, nil, 0
	}

	rk := localRoot(parent, lookup, root, key)
	re := localRoot(parent, lookup, root, exit)
	if rk == re {
		return pathBetween(parent, rk, key, exit), nil, nil, 0
	}

	// keyReach: Key back to its localRoot - as far as it's ever safe to
	// walk Key's own chain backward (see localRoot). exitRoute: root all
	// the way to Exit - always fully safe, root being the tree's true
	// root. A bridge anywhere between these two is a genuine shortcut.
	keyReach := reconstructPath(parent, rk, key)
	exitRoute := reconstructPath(parent, root, exit)
	if a, b, ok := findBridge(m, lookup, keyReach, exitRoute); ok {
		toBridge := reverseUpTo(keyReach, a) // key -> ... -> a, within keyReach's already-safe direction
		fromBridge := exitRoute[indexOf(exitRoute, b):]
		return append(toBridge, fromBridge...), nil, nil, 0
	}

	fallbackParent, fallbackEdges, fallbackVisited, fallbackSpan := runBrenThreads(m, lookup, key, exit)
	return reconstructPath(fallbackParent, key, exit), fallbackVisited, fallbackEdges, fallbackSpan
}

// findBridge looks for a single, ordinary legal move directly connecting
// some cell on route1 to some cell on route2 - a wall opening (or
// teleport) that was never walked during exploration at all, since the
// two routes were discovered as part of the same undifferentiated
// flood-fill, not as two separate goal-seeking searches that would have
// noticed touching each other. Search order favors a bridge close to
// route1's end and close to route2's start, which is where a shortcut
// actually shortens the resulting path - the first hit found is used
// as-is, not the shortest possible one, matching this solver's disclosed
// no-optimality trade elsewhere.
func findBridge(m *Maze, lookup map[Cell]Cell, route1, route2 []Cell) (a, b Cell, ok bool) {
	for i := len(route1) - 1; i >= 0; i-- {
		for j := 0; j < len(route2); j++ {
			if legalMove(m, lookup, route1[i], route2[j]) {
				return route1[i], route2[j], true
			}
		}
	}
	return Cell{}, Cell{}, false
}

// legalMove reports whether a single step from a to b is legal - an open
// wall in some direction (teleport-resolved), the same one-step legality
// ValidatePath itself checks.
func legalMove(m *Maze, lookup map[Cell]Cell, a, b Cell) bool {
	for _, dir := range allDirections {
		if !m.isOpen(a, dir) {
			continue
		}
		next := m.neighbor(a, dir)
		if dest, ok := lookup[next]; ok {
			next = dest
		}
		if next == b {
			return true
		}
	}
	return false
}

// reverseUpTo returns route (ordered root-to-leaf, e.g. rk->...->key)
// reversed from its end back to and including cell - i.e. leaf-to-cell
// order, which is what's needed as the first half of a Key->Exit path
// (Key -> ... -> the bridge point).
func reverseUpTo(route []Cell, cell Cell) []Cell {
	i := indexOf(route, cell)
	out := make([]Cell, 0, len(route)-i)
	for j := len(route) - 1; j >= i; j-- {
		out = append(out, route[j])
	}
	return out
}

func indexOf(route []Cell, cell Cell) int {
	for i, c := range route {
		if c == cell {
			return i
		}
	}
	return -1
}

// localRoot walks cell's ancestor chain up through safely-reversible
// edges only, stopping at the first cell it's not safe to go past - see
// keyToExitPath's doc comment for the exact reversibility rule this
// enforces (an edge parent[cur]->cur is safe to reverse only when
// parent[cur] isn't a teleporter endpoint, and cur itself has no general
// way back at all if it was reached via a redirect - equivalently, cur
// itself being a teleporter endpoint, since resolve() means a cell can
// only ever be reached via redirect if it's registered as one).
func localRoot(parent map[Cell]Cell, lookup map[Cell]Cell, root, cell Cell) Cell {
	cur := cell
	for cur != root {
		if _, ok := lookup[cur]; ok {
			return cur // cur itself was reached via a redirect; no general way back
		}
		p := parent[cur]
		if _, ok := lookup[p]; ok {
			return cur // stepping toward p's position would redirect elsewhere, never landing on p
		}
		cur = p
	}
	return cur
}

// isAncestor reports whether a is an ancestor of b in the full claim
// tree, potentially crossing several teleport hops along the way. This
// is a plain membership walk, not a reconstruction - it never turns the
// chain into a sequence of moves - so it's safe regardless of how many
// teleport-assisted edges it crosses; only *using* a chain as a reversed
// path is what needs the reversibility restriction elsewhere.
func isAncestor(parent map[Cell]Cell, root, a, b Cell) bool {
	for cur := b; ; {
		if cur == a {
			return true
		}
		if cur == root {
			return false
		}
		cur = parent[cur]
	}
}

// pathBetween finds the route from a to b within a single claim tree
// rooted at root, via their lowest common ancestor - the genuine shortcut
// between two cells that share a closer common ancestor than root, not a
// walk back out to root and down again every time. Callers (see
// keyToExitPath) are responsible for only calling this where root, a,
// and b all sit within one safely-reversible branch (see localRoot):
// every edge this function reverses needs to actually be reversible,
// which not every edge in this solver's claim tree is.
//
// Because a cell's claim in this tree is permanent the instant it's won
// (see runBrenThreads), the LCA found here isn't necessarily the true
// graph-shortest meeting point between a and b - it's whichever meeting
// point the concurrent claiming process actually produced. That's the
// same disclosed trade as everywhere else in this solver: a real,
// deterministic shortcut through the tree as it was actually built, not a
// guarantee of the shortest one that could exist.
func pathBetween(parent map[Cell]Cell, root, a, b Cell) []Cell {
	// ancestorDist maps every cell on a's chain up to root to its
	// distance from a, so the walk up from b below can recognize the LCA
	// in O(1) per step instead of rescanning a's whole chain each time.
	ancestorDist := map[Cell]int{}
	for cur, dist := a, 0; ; dist++ {
		ancestorDist[cur] = dist
		if cur == root {
			break
		}
		cur = parent[cur]
	}

	// Walk up from b until landing on a cell that's also on a's chain -
	// that first hit is the LCA, guaranteed to exist since root itself is
	// on both chains.
	bToLCA := []Cell{b}
	cur := b
	for {
		if _, onAChain := ancestorDist[cur]; onAChain {
			break
		}
		cur = parent[cur]
		bToLCA = append(bToLCA, cur)
	}
	lca := cur

	aToLCA := []Cell{a}
	for cur := a; cur != lca; {
		cur = parent[cur]
		aToLCA = append(aToLCA, cur)
	}

	// a -> ... -> LCA, then LCA -> ... -> b (bToLCA reversed, skipping its
	// own last element since that's LCA itself, already included above).
	path := aToLCA
	for i := len(bToLCA) - 2; i >= 0; i-- {
		path = append(path, bToLCA[i])
	}
	return path
}

// runBrenThreads races the threads to completion, starting from `root`,
// stopping as soon as every cell in `targets` has been claimed - there's
// nothing left for further exploration to contribute once the only cells
// any caller here ever needs (Key and Exit, or just Exit for the fallback
// leg) are already in the tree, ancestor chains and all (see below). A
// target equal to `root` needs no claiming and is treated as already found.
// Passing no targets explores the entire reachable maze, matching this
// function's old unconditional behavior.
//
// Returns:
//
//   - parent: the claim tree itself - for every cell but root, the one
//     cell that won its claim race, permanently. This is what routes get
//     reconstructed from directly (see reconstructPath, pathBetween), with
//     no other search run over any richer view of what was discovered.
//     Every ancestor of a claimed cell is necessarily claimed too, and
//     strictly earlier - a cell can only be claimed by a thread already
//     running at its parent - so stopping early never leaves a target's
//     own chain back to root incomplete.
//   - exploreEdges: one entry per claimed cell, `parent` in Edge form -
//     the exploration's actual discovery tree, in commit order.
//   - visited: root plus every claimed cell.
//   - span: the depth, in actual goroutine-spawn generations, of the
//     deepest cell any thread reached - the longest chain of discoveries
//     that had to happen strictly in sequence, since a cell can't be
//     claimed before whichever cell claimed it was. Because a cell's
//     exact claim-winning depth depends on real goroutine-scheduling
//     timing (which thread's claim call happens to acquire the mutex
//     first), span itself can vary slightly run to run on this solver -
//     the same small residual non-determinism this project's ops/allocs
//     counters already have on BrenThreadOptimized, and for the identical
//     underlying reason. Stopping early also means span now reflects the
//     effort actually spent reaching the targets, not incidental depth
//     reached somewhere else in the maze that nobody needed.
func runBrenThreads(m *Maze, lookup map[Cell]Cell, root Cell, targets ...Cell) (parent map[Cell]Cell, exploreEdges []Edge, visited []Cell, span int) {
	claimed := map[Cell]bool{root: true}
	parent = map[Cell]Cell{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	maxDepth := 0

	remaining := 0
	for _, t := range targets {
		if t != root {
			remaining++
		}
	}
	var done int32
	if remaining == 0 {
		atomic.StoreInt32(&done, 1)
	}

	// claim atomically checks-then-claims `next` as one indivisible step,
	// so two threads racing for the same cell can never both win it and
	// spawn duplicate exploration from it. A losing claim records nothing
	// at all now - it never becomes any thread's starting point, so
	// there's nothing for it to contribute to the route, Visited, or
	// span. depth is `next`'s generation number if this claim wins - one
	// more than the thread that's attempting it. Once every target has
	// been claimed, done is flipped so every thread still running stops
	// spawning further branches instead of continuing to flood-fill the
	// rest of the maze.
	claim := func(from, next Cell, depth int) bool {
		mu.Lock()
		defer mu.Unlock()
		if claimed[next] {
			return false
		}
		claimed[next] = true
		parent[next] = from
		exploreEdges = append(exploreEdges, Edge{From: from, To: next, Depth: depth})
		if depth > maxDepth {
			maxDepth = depth
		}
		for _, t := range targets {
			if t == next {
				remaining--
			}
		}
		if remaining <= 0 {
			atomic.StoreInt32(&done, 1)
		}
		return true
	}

	// spawnFrom is one thread's entire life: try every direction from pos,
	// spawn a new thread for every one that's a legal, unclaimed move,
	// then exit. No direction is preferred over any other. It bails out
	// early once done is set, rather than checking only once up front, so
	// a thread that was mid-loop when the last target got claimed doesn't
	// go on to spawn several more needless branches before noticing.
	var spawnFrom func(pos Cell, depth int)
	spawnFrom = func(pos Cell, depth int) {
		defer wg.Done()
		for _, dir := range allDirections {
			if atomic.LoadInt32(&done) != 0 {
				return
			}
			if !m.isOpen(pos, dir) {
				continue // move not allowed
			}
			next := m.neighbor(pos, dir)
			if dest, ok := lookup[next]; ok {
				next = dest // forced, no choice - continue from the new point
			}
			if !claim(pos, next, depth+1) {
				continue // already covered by another thread
			}
			wg.Add(1)
			go spawnFrom(next, depth+1)
		}
	}

	wg.Add(1)
	go spawnFrom(root, 0)
	wg.Wait()

	visited = make([]Cell, 0, len(parent)+1)
	visited = append(visited, root)
	for c := range parent {
		visited = append(visited, c)
	}

	return parent, exploreEdges, visited, maxDepth
}
