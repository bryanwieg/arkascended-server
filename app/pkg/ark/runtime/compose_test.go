package runtime

import "testing"

func TestMapNameMissingWP(t *testing.T) {
	if MapNameMissingWP("TheIsland_WP") {
		t.Fatal("expected false for map name ending in _WP")
	}
	if !MapNameMissingWP("TheIsland") {
		t.Fatal("expected true for map name not ending in _WP")
	}
	if !MapNameMissingWP("Aberration") {
		t.Fatal("expected true for map name not ending in _WP")
	}
	if MapNameMissingWP("ScorchedEarth_WP") {
		t.Fatal("expected false for map name ending in _WP")
	}
}
