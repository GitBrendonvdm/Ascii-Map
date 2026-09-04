package main

import (
	"math"
	"testing"
)

func TestScoreResultsIncludesSpanAtHalfWeight(t *testing.T) {
	results := []runResult{
		{name: "short-span", valid: true, steps: 10, ops: 10, span: 10, primOps: 10, allocs: 10, memBytes: 10},
		{name: "long-span", valid: true, steps: 10, ops: 10, span: 40, primOps: 10, allocs: 10, memBytes: 10},
	}
	scored := scoreResults(results)
	if scored[0].name != "short-span" {
		t.Fatalf("best span should win when every other metric ties, got %q", scored[0].name)
	}
	if got, want := scored[1].score, math.Pow(4, spanScoreWeight/totalScoreWeight); math.Abs(got-want) > 1e-12 {
		t.Fatalf("long-span score = %v, want %v", got, want)
	}
}
