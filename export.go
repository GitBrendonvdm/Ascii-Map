package main

import (
	"encoding/json"
	"os"
	"time"
)

// exportFormatVersion bumps whenever the JSON shape changes in a way that
// would break an older copy of viewer.html reading a newer file (or vice
// versa), so the viewer can show a clear "please refresh the page" error
// instead of silently misrendering.
//
// v2: score no longer derives from elapsedNs/cpuNs (see scoring.go for
// why) - opsCount/allocsCount/opsRatio/allocsRatio replace timeRatio/
// cpuRatio. elapsedNs/cpuNs/memBytes are still present and still real
// measurements, just informational now rather than score inputs.
//
// v3: score no longer derives from opsCount either - spanCount/spanRatio
// replace opsRatio (opsCount itself stays, informational: total work
// performed, as opposed to spanCount's critical-path length - see
// scoring.go for why a concurrent solver needs that distinction, not just
// a raw discovery count).
//
// v4: each edge carries an optional Depth (generation number) - see Edge
// in solver.go. Lets a viewer reveal a whole generation's worth of edges
// at once for a solver that reports real values, instead of always one
// edge at a time in array order.
//
// v5: primOpsCount/primOpsRatio join the score (steps/span/primOps/allocs/
// mem, a 5-way geometric mean now) - a deterministic proxy for raw CPU
// cost (looping, branching, synchronization) that steps/span/allocs/mem
// all leave uncharged - see Solution.PrimOps in solver.go and scoredResult
// in scoring.go for why.
//
// v6: top-level Mazes replaced by Sizes, a list of jsonSizeGroup (width/
// height + that size's own mazes + that size's own leaderboard) - one
// export file now holds every benchmark size tier (see benchmarkSizeTiers
// in seeds.go) instead of one file per size. The top-level Leaderboard
// field is repurposed to mean the OVERALL leaderboard (every maze run
// across every size tier, not just one) - see runBenchmark's own doc
// comment for why mixing sizes into one leaderboard is still meaningful
// (score is already a ratio, not an absolute).
//
// v7: the file is no longer one big indented JSON document - it's
// newline-delimited (NDJSON): a small header line (jsonExportHeader -
// formatVersion/generatedAt/directionBits/sizeIndex/the overall
// leaderboard) followed by one compact JSON line per size group
// (jsonSizeGroup, in the same order as the header's sizeIndex). This is
// what lets viewer.html stream-read and parse ONLY the one size tier a
// viewer actually asks for, instead of the whole file - a full
// benchmarkSizeTiers sweep measures at 700MB+ as one document, well past
// what a browser can reliably buffer and parse in one shot (confirmed:
// fetch() silently returns an empty body for a response that size in
// testing, even though the same bytes transfer fine via curl), while the
// single largest tier alone (100x100, ~430MB) has been measured to parse
// fine on its own. Reading line-by-line and discarding every line that
// isn't the requested one bounds the viewer's memory use to roughly one
// tier's worth, however many tiers the file actually has - see
// streamNDJSONLine in viewer.html.
//
// v8: deterministic score uses total ops rather than Span. Span remains
// exported for diagnostics and animation; PrimOps continues to account for
// deterministic low-level work such as synchronization.
//
// v9: spanRatio returns to the score at half the weight of every other
// dimension, rewarding lower dependency depth without making total work free.
const exportFormatVersion = 9

type jsonCell struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type jsonTeleporter struct {
	Label string   `json:"label"`
	From  jsonCell `json:"from"`
	To    jsonCell `json:"to"`
}

// jsonEdge is one step of a search's discovery tree - see Edge and
// Solution.Edges in solver.go.
type jsonEdge struct {
	From  jsonCell `json:"from"`
	To    jsonCell `json:"to"`
	Depth int      `json:"depth,omitempty"` // generation number; 0 = no concurrency info, see Edge in solver.go
}

// jsonResult mirrors runResult plus its score breakdown, in a shape a
// browser can render directly: nanosecond/byte counts instead of
// time.Duration, and the actual visited/path cell lists needed to animate a
// solve rather than just its final ASCII rendering.
type jsonResult struct {
	Algorithm     string     `json:"algorithm"`
	Valid         bool       `json:"valid"`
	InvalidReason string     `json:"invalidReason,omitempty"`
	Steps         int        `json:"steps"`
	Path          []jsonCell `json:"path"`
	Visited       []jsonCell `json:"visited"`
	Edges         []jsonEdge `json:"edges"`
	OpsCount      int        `json:"opsCount"`     // len(Edges); total work done, deterministic score input
	SpanCount     int        `json:"spanCount"`    // critical-path length; lower is better, half-weight score input
	PrimOpsCount  int64      `json:"primOpsCount"` // deterministic CPU-cost proxy; deterministic score input - see Solution.PrimOps
	AllocsCount   int64      `json:"allocsCount"`  // heap allocation count; deterministic score input
	MemBytes      int64      `json:"memBytes"`     // bytes allocated; deterministic score input
	ElapsedNs     int64      `json:"elapsedNs"`    // informational only - see scoring.go
	CPUNs         int64      `json:"cpuNs"`        // informational only - see scoring.go
	Score         float64    `json:"score,omitempty"`
	StepsRatio    float64    `json:"stepsRatio,omitempty"`
	OpsRatio      float64    `json:"opsRatio,omitempty"`
	SpanRatio     float64    `json:"spanRatio,omitempty"`
	PrimOpsRatio  float64    `json:"primOpsRatio,omitempty"`
	AllocsRatio   float64    `json:"allocsRatio,omitempty"`
	MemRatio      float64    `json:"memRatio,omitempty"`
}

// jsonMaze carries everything needed to redraw a maze from scratch: every
// cell's open-direction bitmask (see directionBits in jsonExport), the
// three landmark cells, teleporter pairs, and every algorithm's result on
// this specific maze.
type jsonMaze struct {
	Seed        int64            `json:"seed"`
	Style       string           `json:"style"`
	Width       int              `json:"width"`
	Height      int              `json:"height"`
	Start       jsonCell         `json:"start"`
	Key         jsonCell         `json:"key"`
	Exit        jsonCell         `json:"exit"`
	Teleporters []jsonTeleporter `json:"teleporters"`
	OpenMask    []int            `json:"openMask"` // row-major, length width*height
	Results     []jsonResult     `json:"results"`
}

type jsonAggregate struct {
	Algorithm    string  `json:"algorithm"`
	AvgScore     float64 `json:"avgScore"`
	AvgSteps     float64 `json:"avgSteps"`
	AvgOps       float64 `json:"avgOps"`     // total work; deterministic score input
	AvgSpan      float64 `json:"avgSpan"`    // animation/diagnostic only
	AvgPrimOps   float64 `json:"avgPrimOps"` // deterministic CPU-cost proxy - see Solution.PrimOps
	AvgAllocs    float64 `json:"avgAllocs"`
	AvgMemBytes  float64 `json:"avgMemBytes"`
	AvgElapsedNs int64   `json:"avgElapsedNs"` // informational only - see scoring.go
	AvgCPUNs     int64   `json:"avgCpuNs"`     // informational only - see scoring.go
	Wins         int     `json:"wins"`
	Runs         int     `json:"runs"`
}

// jsonSizeGroup is every maze/leaderboard entry for one benchmark size
// tier (see benchmarkSizeTiers in seeds.go) - width/height match every
// maze inside it exactly, since a tier's own mazes are all generated at
// that one size.
type jsonSizeGroup struct {
	Width       int             `json:"width"`
	Height      int             `json:"height"`
	Mazes       []jsonMaze      `json:"mazes"`
	Leaderboard []jsonAggregate `json:"leaderboard,omitempty"`
}

// jsonSizeIndexEntry is a size tier's dimensions only - no mazes, no
// leaderboard - so the header line can tell a viewer every tier that
// exists (to populate a size picker) without it having to stream past any
// of the actual (large) group lines first.
type jsonSizeIndexEntry struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// jsonExportHeader is line 1 of the NDJSON export file (see
// exportFormatVersion's v7 note) - deliberately small (no maze data) so a
// viewer can safely read and parse it whole every time, the same way the
// old single-document format worked, before deciding which (much larger)
// group line to stream in next.
type jsonExportHeader struct {
	FormatVersion int                  `json:"formatVersion"`
	GeneratedAt   string               `json:"generatedAt"`
	DirectionBits jsonDirection        `json:"directionBits"`
	SizeIndex     []jsonSizeIndexEntry `json:"sizeIndex"`
	Leaderboard   []jsonAggregate      `json:"leaderboard,omitempty"` // overall - every maze run across every size tier, see exportFormatVersion's v6 note
}

type jsonDirection struct {
	North int `json:"north"`
	South int `json:"south"`
	East  int `json:"east"`
	West  int `json:"west"`
}

func cellToJSON(c Cell) jsonCell { return jsonCell{X: c.X, Y: c.Y} }

func cellsToJSON(cells []Cell) []jsonCell {
	out := make([]jsonCell, len(cells))
	for i, c := range cells {
		out[i] = cellToJSON(c)
	}
	return out
}

func edgesToJSON(edges []Edge) []jsonEdge {
	out := make([]jsonEdge, len(edges))
	for i, e := range edges {
		out[i] = jsonEdge{From: cellToJSON(e.From), To: cellToJSON(e.To), Depth: e.Depth}
	}
	return out
}

// buildJSONMaze captures a maze's static layout plus every solver's result
// against it (path, visited set, timing, and score breakdown when scored is
// non-nil). results and scored are matched up by algorithm name since
// scoreResults drops disqualified entries and may reorder the rest.
func buildJSONMaze(seed int64, style string, m *Maze, results []runResult, scored []scoredResult) jsonMaze {
	scoreByName := make(map[string]scoredResult, len(scored))
	for _, s := range scored {
		scoreByName[s.name] = s
	}

	teleporters := make([]jsonTeleporter, len(m.Teleporters))
	for i, t := range m.Teleporters {
		teleporters[i] = jsonTeleporter{
			Label: string(t.Label),
			From:  cellToJSON(t.From),
			To:    cellToJSON(t.To),
		}
	}

	openMask := make([]int, m.Width*m.Height)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			openMask[y*m.Width+x] = m.CellOpenMask(Cell{X: x, Y: y})
		}
	}

	jsonResults := make([]jsonResult, len(results))
	for i, r := range results {
		jr := jsonResult{
			Algorithm:     r.name,
			Valid:         r.valid,
			InvalidReason: r.invalidReason,
			Steps:         r.steps,
			OpsCount:      r.ops,
			SpanCount:     r.span,
			PrimOpsCount:  r.primOps,
			AllocsCount:   r.allocs,
			MemBytes:      r.memBytes,
			ElapsedNs:     r.elapsed.Nanoseconds(),
			CPUNs:         r.cpuTime.Nanoseconds(),
		}
		if s, ok := scoreByName[r.name]; ok {
			jr.Score = s.score
			jr.StepsRatio = s.stepsRatio
			jr.OpsRatio = s.opsRatio
			jr.SpanRatio = s.spanRatio
			jr.PrimOpsRatio = s.primOpsRatio
			jr.AllocsRatio = s.allocsRatio
			jr.MemRatio = s.memRatio
		}
		jsonResults[i] = jr
	}

	return jsonMaze{
		Seed:        seed,
		Style:       style,
		Width:       m.Width,
		Height:      m.Height,
		Start:       cellToJSON(m.Start),
		Key:         cellToJSON(m.Key),
		Exit:        cellToJSON(m.Exit),
		Teleporters: teleporters,
		OpenMask:    openMask,
		Results:     jsonResults,
	}
}

// buildJSONMazeWithSolutions is buildJSONMaze plus the actual per-algorithm
// path/visited cell lists, which runAllSolvers discards after rendering and
// validating (they never lived on runResult, since the console renderer
// only ever needed the final ASCII string). solveAgain re-solves the maze
// once per algorithm purely to recover those cell lists for export - solves
// are cheap (that's the whole premise of the 200x timing loop elsewhere)
// so a single extra pass here is negligible next to the benchmark itself.
func buildJSONMazeWithSolutions(seed int64, style string, m *Maze, results []runResult, scored []scoredResult) jsonMaze {
	jm := buildJSONMaze(seed, style, m, results, scored)
	byName := make(map[string]*jsonResult, len(jm.Results))
	for i := range jm.Results {
		byName[jm.Results[i].Algorithm] = &jm.Results[i]
	}
	for _, solver := range Solvers() {
		jr, ok := byName[solver.Name()]
		if !ok || !jr.Valid {
			continue
		}
		sol := solver.Solve(m.Clone())
		jr.Path = cellsToJSON(sol.Path)
		jr.Visited = cellsToJSON(sol.Visited)
		jr.Edges = edgesToJSON(sol.Edges)
	}
	return jm
}

func buildJSONAggregate(agg []aggregate) []jsonAggregate {
	out := make([]jsonAggregate, len(agg))
	for i, a := range agg {
		out[i] = jsonAggregate{
			Algorithm:    a.name,
			AvgScore:     a.avgScore,
			AvgSteps:     a.avgSteps,
			AvgOps:       a.avgOps,
			AvgSpan:      a.avgSpan,
			AvgPrimOps:   a.avgPrimOps,
			AvgAllocs:    a.avgAllocs,
			AvgMemBytes:  a.avgMem,
			AvgElapsedNs: a.avgTime.Nanoseconds(),
			AvgCPUNs:     a.avgCPU.Nanoseconds(),
			Wins:         a.wins,
			Runs:         a.runs,
		}
	}
	return out
}

// writeBenchExportNDJSON writes the newline-delimited export format (see
// exportFormatVersion's v7 note): a small header line, then one compact
// JSON line per group in groups, in that same order. json.Encoder.Encode
// already writes its value compact (no indentation, since SetIndent is
// never called here) followed by exactly one trailing '\n' per call - that
// per-call newline is the whole of the line-framing this format needs, so
// nothing here manually concatenates a "\n" onto anything.
func writeBenchExportNDJSON(path string, directionBits jsonDirection, groups []jsonSizeGroup, overallLeaderboard []jsonAggregate) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sizeIndex := make([]jsonSizeIndexEntry, len(groups))
	for i, g := range groups {
		sizeIndex[i] = jsonSizeIndexEntry{Width: g.Width, Height: g.Height}
	}
	header := jsonExportHeader{
		FormatVersion: exportFormatVersion,
		GeneratedAt:   time.Now().Format(time.RFC3339),
		DirectionBits: directionBits,
		SizeIndex:     sizeIndex,
		Leaderboard:   overallLeaderboard,
	}

	enc := json.NewEncoder(f)
	if err := enc.Encode(header); err != nil {
		return err
	}
	for _, g := range groups {
		if err := enc.Encode(g); err != nil {
			return err
		}
	}
	return nil
}
