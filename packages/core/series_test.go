package core

import "testing"

func TestEpisodeOf(t *testing.T) {
	for _, tc := range []struct{ name, show string }{
		{"Planet.Earth.S01E02.1080p", "Planet Earth"},
		{"planet earth s02e10", "planet earth"},
		{"[Group] Planet Earth - EP03 [1080p]", "Planet Earth"},
		{"Lecture Series Episode 4", "Lecture Series"},
		{"地理课 第3集", "地理课"},
		{"地理课第十二讲", "地理课"},
		{"Physics 101 - 04", "Physics 101"},
		{"Cooking with Ana #12", "Cooking with Ana"},
		{"Tokyo Story", ""},
		{"E3 2024", ""},
		{"第3集", ""},
	} {
		show, key, ok := EpisodeOf(tc.name)
		if show != tc.show || ok != (tc.show != "") {
			t.Errorf("EpisodeOf(%q) = %q, %v, want %q", tc.name, show, ok, tc.show)
		}
		if ok && key == "" {
			t.Errorf("EpisodeOf(%q) has no key", tc.name)
		}
	}
	_, a, _ := EpisodeOf("Planet.Earth.S01E02")
	_, b, _ := EpisodeOf("planet earth - EP 7")
	if a != b {
		t.Errorf("the same show has keys %q and %q", a, b)
	}
}
