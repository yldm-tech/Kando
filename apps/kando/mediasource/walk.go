package mediasource

import (
	"context"
	"path"
	"slices"
	"strings"

	"kando/packages/media"
)

// The bounds of importing a folder as a whole (docs/media-sources.md 4.1): its videos down to FolderDepth folders below it, FolderMax of them at most.
const (
	FolderDepth = 3
	FolderMax   = 500
)

// Importable reports whether a folder's entry named name is one the 媒体来源 page lists: not hidden, a name Kando can write back the same way (CleanPath), and a folder or a media file Kando reads.
func Importable(e Entry) bool {
	if e.Name == "" || strings.HasPrefix(e.Name, ".") || strings.Contains(e.Name, "/") {
		return false
	}
	if _, err := CleanPath(e.Name); err != nil {
		return false
	}
	return e.Dir || media.IsMediaFile(e.Name)
}

// Videos are the media files in the folders dirs of d and the folders below them, down to depth levels, each by its path with its size, in the order of the folders given and then of the paths, max of them at most; more reports whether there were others past max. A folder that cannot be listed fails the walk, as the user asked for all of it.
func Videos(ctx context.Context, d Driver, dirs []string, depth, max int) (found map[string]int64, more bool, err error) {
	found = map[string]int64{}
	type folder struct {
		path  string
		level int
	}
	queue := make([]folder, 0, len(dirs))
	for _, dir := range dirs {
		queue = append(queue, folder{dir, 0})
	}
	for len(queue) > 0 {
		f := queue[0]
		queue = queue[1:]
		entries, err := d.List(ctx, f.path)
		if err != nil {
			return nil, false, err
		}
		slices.SortFunc(entries, func(x, y Entry) int { return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name)) })
		for _, e := range entries {
			if !Importable(e) {
				continue
			}
			p := e.Name
			if f.path != "" {
				p = path.Join(f.path, e.Name)
			}
			if e.Dir {
				if f.level+1 <= depth {
					queue = append(queue, folder{p, f.level + 1})
				}
				continue
			}
			if _, ok := found[p]; ok {
				continue
			}
			if len(found) >= max {
				return found, true, nil
			}
			found[p] = e.Size
		}
	}
	return found, false, nil
}
