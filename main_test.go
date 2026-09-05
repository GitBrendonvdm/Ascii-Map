package main

import "testing"

func TestMapSizePresetLargeOptInSizes(t *testing.T) {
	for _, test := range []struct {
		name          string
		width, height int
	}{
		{name: "huge", width: 400, height: 400},
		{name: "massive", width: 1000, height: 1000},
		{name: "colossal", width: 5000, height: 5000},
	} {
		width, height, ok := mapSizePreset(test.name)
		if !ok || width != test.width || height != test.height {
			t.Errorf("mapSizePreset(%q) = (%d, %d, %t), want (%d, %d, true)", test.name, width, height, ok, test.width, test.height)
		}
	}
}

func TestBenchmarkIncludesEveryNamedLargeSize(t *testing.T) {
	want := map[[2]int]bool{
		{250, 250}:   false,
		{400, 400}:   false,
		{1000, 1000}: false,
	}
	for _, tier := range benchmarkSizeTiers {
		key := [2]int{tier.width, tier.height}
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for size, found := range want {
		if !found {
			t.Errorf("benchmark is missing named %dx%d size tier", size[0], size[1])
		}
	}
}
