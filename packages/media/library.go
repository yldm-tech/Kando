package media

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Video struct {
	Path    string
	Name    string
	ModTime int64
}

func ListVideos(directory string) ([]Video, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	videos := make([]Video, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			nested, err := videosIn(filepath.Join(directory, entry.Name()))
			if err != nil {
				return nil, err
			}
			videos = append(videos, nested...)
			continue
		}
		if !isVideoFile(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		videos = append(videos, Video{Path: filepath.Join(directory, entry.Name()), Name: entry.Name(), ModTime: info.ModTime().UnixNano()})
	}
	sort.SliceStable(videos, func(i, j int) bool {
		if videos[i].ModTime == videos[j].ModTime {
			return videos[i].Name < videos[j].Name
		}
		return videos[i].ModTime > videos[j].ModTime
	})
	return videos, nil
}

// IsVideoFile reports whether name has an extension the library lists.
func IsVideoFile(name string) bool { return isVideoFile(name) }

func videosIn(dir string) ([]Video, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	videos := make([]Video, 0, 1)
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !isVideoFile(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		videos = append(videos, Video{Path: filepath.Join(dir, entry.Name()), Name: displayName(dir, entry.Name()), ModTime: info.ModTime().UnixNano()})
	}
	return videos, nil
}

// GatherVideos moves a loose video and its subtitles into "<name>/video.<ext>", and renames a video already inside a folder to video.<ext>. The returned map is the old path to the new path. A folder that already contains video.<ext> is left alone, including its other subtitle names.
func GatherVideos(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	moved := map[string]string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			nested, err := normalizeItem(filepath.Join(dir, entry.Name()))
			if err != nil {
				return moved, err
			}
			for src, dst := range nested {
				moved[src] = dst
			}
			continue
		}
		if !isVideoFile(entry.Name()) {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		folder := filepath.Join(dir, base)
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return moved, err
		}
		renames := [][2]string{
			{entry.Name(), "video" + filepath.Ext(entry.Name())},
			{base + ".srt", "video.srt"},
			{base + ".bilingual.srt", "video.bilingual.srt"},
		}
		for _, pair := range renames {
			src := filepath.Join(dir, pair[0])
			if _, err := os.Stat(src); err != nil {
				continue
			}
			dst := filepath.Join(folder, pair[1])
			if _, err := os.Stat(dst); err == nil {
				continue
			}
			if err := os.Rename(src, dst); err != nil {
				return moved, err
			}
			moved[src] = dst
		}
	}
	return moved, nil
}

// AdoptLibrary moves videos from the old plain folder into a vimpo bundle, then gives each one the video.<ext> name. An existing destination is skipped, so a video already in the bundle stays put.
func AdoptLibrary(from, to string) (map[string]string, error) {
	from, to = filepath.Clean(from), filepath.Clean(to)
	if from == to {
		return nil, nil
	}
	entries, err := os.ReadDir(from)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		return nil, err
	}
	moved := map[string]string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		src := filepath.Join(from, entry.Name())
		dst := filepath.Join(to, entry.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			return moved, err
		}
		if !entry.IsDir() {
			moved[src] = dst
			continue
		}
		if err := filepath.WalkDir(dst, func(path string, item os.DirEntry, err error) error {
			if err != nil || item.IsDir() {
				return err
			}
			rel, err := filepath.Rel(dst, path)
			if err != nil {
				return nil
			}
			moved[filepath.Join(src, rel)] = path
			return nil
		}); err != nil {
			return moved, err
		}
		nested, err := normalizeItem(dst)
		if err != nil {
			return moved, err
		}
		for old, next := range nested {
			rel, err := filepath.Rel(dst, old)
			if err != nil {
				continue
			}
			moved[filepath.Join(src, rel)] = next
		}
	}
	return moved, nil
}

// Relocate finds a file under root when path itself is gone but the same folder and file name are there.
func Relocate(root, path string) (string, bool) {
	if root == "" || path == "" {
		return "", false
	}
	if _, err := os.Stat(path); err == nil {
		return "", false
	}
	next := filepath.Join(root, filepath.Base(filepath.Dir(path)), filepath.Base(path))
	if _, err := os.Stat(next); err != nil {
		return "", false
	}
	return next, true
}

// RemoveEmptyLibrary deletes dir when only hidden files remain.
func RemoveEmptyLibrary(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			return
		}
	}
	_ = os.RemoveAll(dir)
}

func normalizeItem(folder string) (map[string]string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	var videos []string
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !isVideoFile(entry.Name()) {
			continue
		}
		videos = append(videos, entry.Name())
	}
	if len(videos) != 1 || itemVideo(videos[0]) {
		return nil, nil
	}
	name := videos[0]
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	moved := map[string]string{}
	for _, pair := range [][2]string{
		{name, "video" + ext},
		{base + ".srt", "video.srt"},
		{base + ".bilingual.srt", "video.bilingual.srt"},
	} {
		src := filepath.Join(folder, pair[0])
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(folder, pair[1])
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			return moved, err
		}
		moved[src] = dst
	}
	return moved, nil
}

func displayName(dir, name string) string {
	if itemVideo(name) {
		return filepath.Base(dir) + filepath.Ext(name)
	}
	return name
}

func itemVideo(name string) bool {
	return strings.TrimSuffix(name, filepath.Ext(name)) == "video"
}

func isVideoFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".flv":
		return true
	default:
		return false
	}
}
