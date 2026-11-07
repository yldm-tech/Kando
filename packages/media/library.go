package media

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
		if !isVideoFile(entry.Name()) || downloadLeftover(entry.Name()) {
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

// WatchLibrary calls changed with the library directory when its contents change or the directory itself is renamed. The first look is the baseline and does not call changed. stop ends the watch. changed receives the directory to read, which is the renamed path when the folder moved.
func WatchLibrary(dir string, every time.Duration, stop <-chan struct{}, changed func(dir string)) {
	if changed == nil {
		return
	}
	if every <= 0 {
		every = time.Second
	}
	id := identifyDir(dir)
	prev, _ := LibraryStamp(dir)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			current := dir
			if _, err := os.Stat(dir); err != nil {
				if next, ok := findDirID(filepath.Dir(dir), id); ok {
					current = next
				}
			}
			next, err := LibraryStamp(current)
			if err != nil || (current == dir && next == prev) {
				continue
			}
			dir = current
			id = identifyDir(dir)
			prev = next
			changed(dir)
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
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !isVideoFile(entry.Name()) || downloadLeftover(entry.Name()) {
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

// GatherVideos moves a loose video and its subtitles into "<name>/video.<ext>", and renames a video already inside a folder to video.<ext>. The returned map is the old path to the new path. A folder that already contains video.<ext> is left alone, including its other subtitle names, and so is one that holds what a download leaves while it runs (downloadLeftover): its single finished track is not a video to rename.
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
		if !isVideoFile(entry.Name()) || downloadLeftover(entry.Name()) {
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

// AdoptLibrary moves videos from a plain folder into a library bundle, then gives each one the video.<ext> name. An existing destination is skipped, so a video already in the bundle stays put.
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

// RetargetRenamed pairs files that disappeared under root with videos now in the library. A folder renamed in Finder keeps its [id] when the title still has one; otherwise a single missing video is paired with the single unclaimed video. Companion files beside a moved video follow that video.
func RetargetRenamed(root string, missing, live []string) map[string]string {
	if root == "" || len(missing) == 0 {
		return nil
	}
	videos, err := ListVideos(root)
	if err != nil || len(videos) == 0 {
		return nil
	}
	taken := map[string]bool{}
	for _, path := range live {
		taken[filepath.Clean(path)] = true
	}
	free := make([]Video, 0, len(videos))
	for _, video := range videos {
		if !taken[filepath.Clean(video.Path)] {
			free = append(free, video)
		}
	}
	var gone []string
	for _, path := range missing {
		if path == "" || !pathInside(root, path) || !isVideoFile(filepath.Base(path)) {
			continue
		}
		gone = append(gone, path)
	}
	moved := map[string]string{}
	used := map[string]bool{}
	for _, old := range gone {
		id := folderID(filepath.Base(filepath.Dir(old)))
		if id == "" {
			continue
		}
		hit := ""
		ambiguous := false
		for _, video := range free {
			if used[video.Path] || folderID(filepath.Base(filepath.Dir(video.Path))) != id {
				continue
			}
			if hit != "" {
				ambiguous = true
				break
			}
			hit = video.Path
		}
		if hit != "" && !ambiguous {
			moved[old] = hit
			used[hit] = true
		}
	}
	var leftGone []string
	var leftFree []string
	for _, old := range gone {
		if _, ok := moved[old]; !ok {
			leftGone = append(leftGone, old)
		}
	}
	for _, video := range free {
		if !used[video.Path] {
			leftFree = append(leftFree, video.Path)
		}
	}
	if len(leftGone) == 1 && len(leftFree) == 1 {
		moved[leftGone[0]] = leftFree[0]
	}
	for _, path := range missing {
		if path == "" {
			continue
		}
		if _, ok := moved[path]; ok {
			continue
		}
		oldDir := filepath.Clean(filepath.Dir(path))
		for oldVideo, newVideo := range moved {
			if filepath.Clean(filepath.Dir(oldVideo)) != oldDir {
				continue
			}
			next := filepath.Join(filepath.Dir(newVideo), filepath.Base(path))
			if _, err := os.Stat(next); err == nil {
				moved[path] = next
			}
			break
		}
	}
	if len(moved) == 0 {
		return nil
	}
	return moved
}

func folderID(name string) string {
	start := strings.LastIndex(name, "[")
	end := strings.LastIndex(name, "]")
	if start < 0 || end <= start {
		return ""
	}
	return name[start+1 : end]
}

func pathInside(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if downloadLeftover(entry.Name()) {
			return nil, nil
		}
		if isVideoFile(entry.Name()) {
			videos = append(videos, entry.Name())
		}
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

// formatTrack is the name yt-dlp gives one format of a video downloaded as several, video.f<format id>.<ext> for Kando's output template, before it merges them into video.<ext>; YouTube's format ids are digits, sometimes with a suffix such as 251-drc.
var formatTrack = regexp.MustCompile(`^video\.f[0-9]+(-[0-9a-z]+)?\.[0-9a-z]+$`)

// downloadLeftover reports whether name is something yt-dlp writes while it downloads and removes once the video is merged: a .part file or one of its fragments, the .ytdl file that lets it resume, one format's track, or the merge's video.temp.<ext>. Such a file in a library folder means a download is running there, or one stopped before it merged.
func downloadLeftover(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".part") || strings.HasSuffix(lower, ".ytdl") || strings.Contains(lower, ".part-frag") {
		return true
	}
	return formatTrack.MatchString(lower) || strings.TrimSuffix(lower, filepath.Ext(lower)) == "video.temp"
}

// holdsDownloadLeftovers reports whether folder has any file downloadLeftover names.
func holdsDownloadLeftovers(folder string) bool {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && downloadLeftover(entry.Name()) {
			return true
		}
	}
	return false
}

func isVideoFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".flv":
		return true
	default:
		return false
	}
}
