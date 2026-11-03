package media

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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

// LibraryStamp describes the library folder and the files one level down. It changes when a video, subtitle, or folder is added, removed, or rewritten.
func LibraryStamp(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		writeStamp(&b, entry)
		if !entry.IsDir() {
			continue
		}
		nested, err := os.ReadDir(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		for _, item := range nested {
			if strings.HasPrefix(item.Name(), ".") {
				continue
			}
			writeStamp(&b, item)
		}
	}
	return b.String(), nil
}

func writeStamp(b *strings.Builder, entry os.DirEntry) {
	var size, mod int64
	if info, err := entry.Info(); err == nil {
		size = info.Size()
		mod = info.ModTime().UnixNano()
	}
	fmt.Fprintf(b, "%s %t %d %d\n", entry.Name(), entry.IsDir(), size, mod)
}

// WatchLibrary calls changed when LibraryStamp changes. The first look is the baseline and does not call changed. stop ends the watch.
func WatchLibrary(dir string, every time.Duration, stop <-chan struct{}, changed func()) {
	if every <= 0 {
		every = time.Second
	}
	prev, _ := LibraryStamp(dir)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			next, err := LibraryStamp(dir)
			if err != nil || next == prev {
				continue
			}
			prev = next
			changed()
		}
	}
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

// AdoptLibrary moves videos from the old plain folder into a library bundle, then gives each one the video.<ext> name. An existing destination is skipped, so a video already in the bundle stays put.
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

// DetachOwnVideos moves folders out of another app's library. A folder that contains item.json stays, and so do files sitting in the library root.
func DetachOwnVideos(from, to string) (map[string]string, error) {
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
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		src := filepath.Join(from, entry.Name())
		if _, err := os.Stat(filepath.Join(src, "item.json")); err == nil {
			continue
		}
		dst := filepath.Join(to, entry.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			return moved, err
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
