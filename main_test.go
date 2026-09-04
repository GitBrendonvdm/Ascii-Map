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
