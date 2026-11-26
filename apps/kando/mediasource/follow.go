package mediasource

import (
	"maps"
	"slices"
)

// Follow is a folder of a source the user follows (docs/media-sources.md 4.4), as a show's whose episodes come out one after another: Kando lists it now and then, down to FolderDepth folders and FolderMax videos as importing it whole does, and imports each video it holds that it did not hold before, the way Link says, indexed and played from the source when set, else copied. Seen are the paths of the videos found in it so far, those there as it was followed among them, so a video is imported once at most: one the user took off the shelf is not imported again.
type Follow struct {
	Source string   `json:"source"`
	Dir    string   `json:"dir"`
	Link   bool     `json:"link,omitempty"`
	Seen   []string `json:"seen,omitempty"`
}

// Fresh are the paths of found that f has not seen, in order.
func (f Follow) Fresh(found map[string]int64) []string {
	var out []string
	for _, p := range slices.Sorted(maps.Keys(found)) {
		if !slices.Contains(f.Seen, p) {
			out = append(out, p)
		}
	}
	return out
}

// See notes the paths of found as seen, keeping those seen before, in order; it reports whether any was new.
func (f *Follow) See(found map[string]int64) bool {
	fresh := f.Fresh(found)
	if len(fresh) == 0 {
		return false
	}
	f.Seen = slices.Sorted(slices.Values(slices.Concat(f.Seen, fresh)))
	return true
}
