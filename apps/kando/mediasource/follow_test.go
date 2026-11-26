package mediasource

import (
	"slices"
	"testing"
)

// Fresh are the videos found that were not seen, and See adds them, keeping those seen before that are gone.
func TestFollowFreshAndSee(t *testing.T) {
	f := Follow{Seen: []string{"Show/E01.mkv", "Show/E02.mkv"}}
	found := map[string]int64{"Show/E02.mkv": 2, "Show/E04.mkv": 4, "Show/E03.mkv": 3}
	if got := f.Fresh(found); !slices.Equal(got, []string{"Show/E03.mkv", "Show/E04.mkv"}) {
		t.Fatalf("fresh %q", got)
	}
	if !f.See(found) || !slices.Equal(f.Seen, []string{"Show/E01.mkv", "Show/E02.mkv", "Show/E03.mkv", "Show/E04.mkv"}) {
		t.Fatalf("seen %q", f.Seen)
	}
	if f.See(found) || len(f.Fresh(found)) != 0 {
		t.Fatal("seen twice")
	}
}
