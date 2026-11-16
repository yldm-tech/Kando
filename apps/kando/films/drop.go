package films

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kando/packages/media"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// DroppedVideos is the videos among paths, the files and folders dropped on the window, picked or opened with Kando, in the order they are to be queued, and how many files were left out because they are not videos. A file is kept when its name is a video's (media.IsVideoFile), as before folders could be dropped, so one that cannot be read still makes a job that says why. A folder gives the videos directly in it, then those in each folder directly in it, as a course keeps its lessons in a folder per section and a show its episodes in a folder per season; folders deeper than that are not looked into, so dropping a large folder by mistake does not queue everything under it. A folder's videos and folders come in their names' order as tag, the interface's language, orders words, numbers by value, so "Episode 2" comes before "Episode 10" and episodes queue in order. Hidden files and folders are neither added nor counted, and neither is a folder that cannot be read.
func DroppedVideos(paths []string, tag string) (videos []string, skipped int) {
	col := collate.New(language.Make(tag), collate.Numeric)
	byName := func(x, y os.DirEntry) int { return col.CompareString(x.Name(), y.Name()) }
	// folder lists dir's videos, and those of its folders while depth is above 0.
	var folder func(dir string, depth int)
	folder = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		slices.SortStableFunc(entries, byName)
		var folders []string
		for _, entry := range entries {
			name := entry.Name()
			switch {
			case strings.HasPrefix(name, "."):
			case entry.IsDir():
				folders = append(folders, filepath.Join(dir, name))
			case media.IsVideoFile(name):
				videos = append(videos, filepath.Join(dir, name))
			default:
				skipped++
			}
		}
		if depth > 0 {
			for _, sub := range folders {
				folder(sub, depth-1)
			}
		}
	}
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			folder(path, 1)
			continue
		}
		if media.IsVideoFile(path) {
			videos = append(videos, path)
		} else {
			skipped++
		}
	}
	return videos, skipped
}
