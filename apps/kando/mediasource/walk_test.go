package mediasource

import (
	"context"
	"errors"
	"io"
	"maps"
	"path"
	"slices"
	"strings"
	"testing"
)

// treeDriver is a source of files by path, which lists the folders above them as folders too; listing a folder in fail fails.
type treeDriver struct {
	files map[string]int64
	fail  string
}

func (d treeDriver) List(_ context.Context, dir string) ([]Entry, error) {
	if d.fail != "" && dir == d.fail {
		return nil, errors.New("connection reset")
	}
	seen := map[string]bool{}
	var out []Entry
	for p, size := range d.files {
		rest, ok := strings.CutPrefix(p, dir+"/")
		if dir == "" {
			rest, ok = p, true
		}
		if !ok {
			continue
		}
		name, _, isDir := strings.Cut(rest, "/")
		if seen[name] {
			continue
		}
		seen[name] = true
		e := Entry{Name: name, Dir: isDir}
		if !isDir {
			e.Size = size
		}
		out = append(out, e)
	}
	return out, nil
}

func (d treeDriver) Stat(context.Context, string) (Entry, error) { return Entry{}, ErrNotExist }
func (d treeDriver) Open(context.Context, string, int64, int64) (io.ReadCloser, error) {
	return nil, ErrNotExist
}
func (d treeDriver) Close() error { return nil }

// TestVideosOfFolders checks the videos a folder imported whole takes: the media files of the folders chosen and of those below them, down to the depth given, with their sizes, the depth counted from each folder chosen; neither hidden ones, other files nor what lies below the depth; each once when two folders chosen hold it; at most max, more said; and a folder that cannot be listed failing the walk.
func TestVideosOfFolders(t *testing.T) {
	d := treeDriver{files: map[string]int64{
		"Shows/Night/Season 1/S01E01.mkv":   10,
		"Shows/Night/Season 1/S01E01.srt":   1,
		"Shows/Night/Season 2/S02E01.mp4":   20,
		"Shows/Night/Season 2/Deep/x/y.mkv": 5,
		"Shows/Night/.trash/old.mkv":        3,
		"Shows/Day/Pilot.avi":               30,
		"Films/Heat.mkv":                    40,
		"Films/poster.jpg":                  2,
	}}
	ctx := context.Background()
	found, more, err := Videos(ctx, d, []string{"Shows", "Shows/Night"}, 3, 500)
	// The depth counts from each folder chosen: Deep/x is three below Night, which is chosen too, and each video is taken once.
	want := map[string]int64{"Shows/Night/Season 1/S01E01.mkv": 10, "Shows/Night/Season 2/S02E01.mp4": 20, "Shows/Night/Season 2/Deep/x/y.mkv": 5, "Shows/Day/Pilot.avi": 30}
	if err != nil || more || !maps.Equal(found, want) {
		t.Fatalf("found %v, more %v, err %v", found, more, err)
	}
	found, _, _ = Videos(ctx, d, []string{""}, 3, 500)
	if _, ok := found["Shows/Night/Season 2/Deep/x/y.mkv"]; ok || found["Films/Heat.mkv"] != 40 {
		t.Errorf("from the root: %v", found)
	}
	found, more, err = Videos(ctx, d, []string{"Shows"}, 3, 2)
	if err != nil || !more || len(found) != 2 {
		t.Errorf("at most 2: %v, more %v, err %v", slices.Sorted(maps.Keys(found)), more, err)
	}
	d.fail = path.Join("Shows", "Day")
	if _, _, err := Videos(ctx, d, []string{"Shows"}, 3, 500); err == nil {
		t.Error("a folder that cannot be listed did not fail the walk")
	}
}
