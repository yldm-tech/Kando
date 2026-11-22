package media

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

type Video struct {
	Path    string
	Name    string
	ModTime int64
	// Subtitles is the bilingual .srt beside the video, named as the pipeline exports it (<video>.bilingual.srt), or "" when its folder holds none. A video no job has processed shows these, as one copied in from another library or kept after its job records were lost.
	Subtitles string
	// Added is when a video kept outside the library was added to it without a job, as a large folder dropped adds its films, which the shelf lists as it lists a video of its folder; zero for a video found in the library's folder.
	Added time.Time
}

// ListVideos is the films' files directly in directory and directly in each folder in it (filmFile), newest first. away are the paths of films that belong under directory but are not on disk now, as those moved to a storage location: a file of the same name beside one of them is its companion, not a film of its own.
func ListVideos(directory string, away ...string) ([]Video, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	files := fileNames(entries)
	first := firstOfName(files, awayIn(directory, away))
	videos := make([]Video, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			nested, err := videosIn(filepath.Join(directory, entry.Name()), away)
			if err != nil {
				return nil, err
			}
			videos = append(videos, nested...)
			continue
		}
		if !filmFile(entry.Name(), first) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		videos = append(videos, Video{Path: filepath.Join(directory, entry.Name()), Name: entry.Name(), ModTime: info.ModTime().UnixNano(), Subtitles: bilingualBeside(directory, entry.Name(), files)})
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

func videosIn(dir string, away []string) ([]Video, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := fileNames(entries)
	first := firstOfName(files, awayIn(dir, away))
	videos := make([]Video, 0, 1)
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !filmFile(entry.Name(), first) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		videos = append(videos, Video{Path: filepath.Join(dir, entry.Name()), Name: displayName(dir, entry.Name()), ModTime: info.ModTime().UnixNano(), Subtitles: bilingualBeside(dir, entry.Name(), files)})
	}
	return videos, nil
}

// awayIn is the paths among away directly in dir.
func awayIn(dir string, away []string) []string {
	var out []string
	for _, path := range away {
		if filepath.Clean(filepath.Dir(path)) == filepath.Clean(dir) {
			out = append(out, path)
		}
	}
	return out
}

// fileNames is the names of the files, not folders, among entries, so the videos listed from them can find their subtitles without another look at the disk.
func fileNames(entries []os.DirEntry) map[string]bool {
	names := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names[entry.Name()] = true
		}
	}
	return names
}

// bilingualBeside is the path of video's bilingual subtitles in dir, <video>.bilingual.srt as Make writes them, when files, the names of the files in dir, holds it; "" otherwise.
func bilingualBeside(dir, video string, files map[string]bool) string {
	name := strings.TrimSuffix(video, filepath.Ext(video)) + ".bilingual.srt"
	if !files[name] {
		return ""
	}
	return filepath.Join(dir, name)
}

// GatherVideos moves a loose video and its subtitles, the transcript Kando keeps beside subtitles of the user's (asideTranscriptPath) and the user's corrections of its lines (CorrectionsPath) among them, into "<name>/video.<ext>", and renames a video already inside a folder to video.<ext>. The returned map is the old path to the new path. A folder that already contains video.<ext> is left alone, including its other subtitle names, and so is one that holds what a download leaves while it runs (downloadLeftover): its single finished track is not a video to rename. away are the paths of videos that belong in dir but are not on disk now, as those moved to a storage location, which come back to those paths: each counts as a video of its folder, so a folder that holds one besides a video on disk is not one video's folder, and no file is renamed or moved to one of those paths.
func GatherVideos(dir string, away []string) (map[string]string, error) {
	held := map[string]bool{}
	for _, path := range away {
		held[filepath.Clean(path)] = true
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	first := firstOfName(fileNames(entries), awayIn(dir, away))
	moved := map[string]string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			nested, err := normalizeItem(filepath.Join(dir, entry.Name()), held)
			if err != nil {
				return moved, err
			}
			for src, dst := range nested {
				moved[src] = dst
			}
			continue
		}
		if !filmFile(entry.Name(), first) {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		folder := filepath.Join(dir, base)
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return moved, err
		}
		renames := append([][2]string{{entry.Name(), "video" + filepath.Ext(entry.Name())}}, itemRenames(dir, entry.Name())...)
		for _, pair := range renames {
			src := filepath.Join(dir, pair[0])
			if _, err := os.Stat(src); err != nil {
				continue
			}
			dst := filepath.Join(folder, pair[1])
			if _, err := os.Stat(dst); err == nil || held[dst] {
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
		nested, err := normalizeItem(dst, nil)
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
		if path == "" || !pathInside(root, path) || !IsMediaFile(filepath.Base(path)) {
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

// RetargetRenamedFolder is where a file that disappeared under root went when its folder was renamed in Finder, for a film that keeps no video (core.Job.SubtitlesOnly), which RetargetRenamed cannot pair as it pairs videos: the one other folder directly under root that keeps the old folder's [id] and holds a file of the same name. A folder without an [id], or an [id] that more than one folder holding that file keeps, pairs nothing.
func RetargetRenamedFolder(root, missing string) (string, bool) {
	if root == "" || missing == "" || !pathInside(root, missing) {
		return "", false
	}
	if _, err := os.Stat(missing); err == nil {
		return "", false
	}
	oldDir := filepath.Dir(missing)
	id := folderID(filepath.Base(oldDir))
	if id == "" || filepath.Clean(filepath.Dir(oldDir)) != filepath.Clean(root) {
		return "", false
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	hit := ""
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == filepath.Base(oldDir) || folderID(entry.Name()) != id {
			continue
		}
		next := filepath.Join(root, entry.Name(), filepath.Base(missing))
		if _, err := os.Stat(next); err != nil {
			continue
		}
		if hit != "" {
			return "", false
		}
		hit = next
	}
	return hit, hit != ""
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

// normalizeItem renames the one video of folder and the files beside it that are its to video.<ext> and the names that go with it, unless folder holds no video, more than one, a video of held (one that belongs there but is not on disk now, GatherVideos' away), or what a download leaves while it runs. No file goes to a path of held.
func normalizeItem(folder string, held map[string]bool) (map[string]string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	for path := range held {
		if filepath.Dir(path) == filepath.Clean(folder) {
			return nil, nil
		}
	}
	first := firstOfName(fileNames(entries), nil)
	var videos []string
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if downloadLeftover(entry.Name()) {
			return nil, nil
		}
		if filmFile(entry.Name(), first) {
			videos = append(videos, entry.Name())
		}
	}
	if len(videos) != 1 || itemVideo(videos[0]) {
		return nil, nil
	}
	name := videos[0]
	moved := map[string]string{}
	for _, pair := range append([][2]string{{name, "video" + filepath.Ext(name)}}, itemRenames(folder, name)...) {
		src := filepath.Join(folder, pair[0])
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(folder, pair[1])
		if _, err := os.Stat(dst); err == nil || held[dst] {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			return moved, err
		}
		moved[src] = dst
	}
	return moved, nil
}

// itemRenames are the files of the film's file name in dir that go with it when it becomes video.<ext>, each as its name and the name it takes: its subtitles, the user's corrections of its lines and the media files of its name (SameNamed).
func itemRenames(dir, name string) [][2]string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	out := [][2]string{
		{base + ".srt", "video.srt"},
		{base + ".kando.srt", "video.kando.srt"},
		{base + ".bilingual.srt", "video.bilingual.srt"},
		{"." + base + ".edits.json", ".video.edits.json"},
	}
	for _, path := range SameNamed(filepath.Join(dir, name)) {
		out = append(out, [2]string{filepath.Base(path), "video" + filepath.Ext(path)})
	}
	return out
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

// The extensions of the media files a library takes, as Kando opens them: videos, then sound alone, as podcasts, listening practice and audiobooks come. A file of either is a film (filmFile). Their order ranks files of one name in one folder: the first is the film and the others its own (SameNamed), as a lesson.mp3 beside lesson.mp4 is the lesson's sound, which shares its subtitles and logs, as every file named after a film does.
var (
	videoExtensions = []string{".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".flv"}
	soundExtensions = []string{".m4a", ".m4b", ".mp3", ".aac", ".flac", ".wav", ".ogg", ".opus", ".wma"}
)

// MediaExtensions are the extensions, without the dot, of the files the library takes, videos first: those IsMediaFile takes.
func MediaExtensions() []string {
	out := make([]string, 0, len(videoExtensions)+len(soundExtensions))
	for _, ext := range append(append([]string(nil), videoExtensions...), soundExtensions...) {
		out = append(out, ext[1:])
	}
	return out
}

// IsMediaFile reports whether name has an extension the library takes, a video's or a sound's.
func IsMediaFile(name string) bool { return mediaRank(name) >= 0 }

// IsSoundFile reports whether name has the extension of a file of sound alone, as mp3. A file whose extension is a video's may hold sound alone too, as yt-dlp's best sound of a YouTube video in a .webm; what has a picture is known from the file itself (GrabPoster, the system's player), and this is for what goes by the type of the file, as the media type it is served with.
func IsSoundFile(name string) bool {
	return slices.Contains(soundExtensions, strings.ToLower(filepath.Ext(name)))
}

// IsFilm reports whether the file at path is a film's file: a media file, not what a download leaves while it runs, not a clip of Kando's own export, and not the sound of a file of the same name beside it (filmFile).
func IsFilm(path string) bool {
	film, ok := FilmOf(path)
	return ok && film == path
}

// FilmOf is the film's file the media file at path is, as a file added to the library is taken: path itself, or the file of the same name beside it that ranks first, whose sound it is (SameNamed), so adding lesson.mp3 beside lesson.mp4 adds the lesson once, with the subtitles both are named for. false for what is not a film's: a file the library does not take, what a download leaves, or a clip of Kando's own export. A file whose folder cannot be read is taken as it is, so its job says why.
func FilmOf(path string) (string, bool) {
	dir, name := filepath.Split(path)
	if !filmFile(name, nil) {
		return "", false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return path, true
	}
	first := firstOfName(fileNames(entries), nil)
	if filmFile(name, first) {
		return path, true
	}
	for _, entry := range entries {
		if !entry.IsDir() && stemKey(entry.Name()) == stemKey(name) && filmFile(entry.Name(), first) {
			return filepath.Join(dir, entry.Name()), true
		}
	}
	return path, true
}

// mediaRank is name's place among the media extensions, -1 for a file the library does not take.
func mediaRank(name string) int {
	ext := strings.ToLower(filepath.Ext(name))
	if i := slices.Index(videoExtensions, ext); i >= 0 {
		return i
	}
	if i := slices.Index(soundExtensions, ext); i >= 0 {
		return len(videoExtensions) + i
	}
	return -1
}

// stemKey is name without its extension, lower-cased, which files of one name in a folder share.
func stemKey(name string) string {
	return strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
}

// firstOfName is, for each name without its extension among files and the base names of away, the paths of films that belong in the folder but are not on disk now (GatherVideos), the best rank among the media files of that name.
func firstOfName(files map[string]bool, away []string) map[string]int {
	first := map[string]int{}
	add := func(name string) {
		r := mediaRank(name)
		if r < 0 || downloadLeftover(name) || exportedClip(name) {
			return
		}
		if best, ok := first[stemKey(name)]; !ok || r < best {
			first[stemKey(name)] = r
		}
	}
	for name := range files {
		add(name)
	}
	for _, path := range away {
		add(filepath.Base(path))
	}
	return first
}

// filmFile reports whether name, a file in a folder whose first media files by name are first (firstOfName), is a film's file: a media file, not a download's leftover, not a clip of Kando's own export, and the first of its name. A nil first knows of no other file.
func filmFile(name string, first map[string]int) bool {
	r := mediaRank(name)
	if r < 0 || downloadLeftover(name) || exportedClip(name) {
		return false
	}
	best, ok := first[stemKey(name)]
	return !ok || best == r
}

// SameNamed are the media files beside the film's file at video that share its name and rank after it, those that exist, as a lesson.mp3 beside lesson.mp4: the film's own sound, which shares the files named after it (Companions) and is not a film of its own (filmFile), so it goes where the film goes when it moves or is deleted. They are kept apart from Companions, which are small files read whole.
func SameNamed(video string) []string {
	r := mediaRank(video)
	if r < 0 {
		return nil
	}
	dir := filepath.Dir(video)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == filepath.Base(video) || stemKey(name) != stemKey(filepath.Base(video)) {
			continue
		}
		if other := mediaRank(name); other > r && !downloadLeftover(name) {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out
}

// ClipName is the name of the file Kando exports the sound of one saved line in, for Anki, from the hash sum of what it is: kando- and the sum in 16 hex digits, .m4a.
func ClipName(sum uint64) string { return fmt.Sprintf("kando-%016x.m4a", sum) }

// exportedClipName matches ClipName's names.
var exportedClipName = regexp.MustCompile(`^kando-[0-9a-f]{16}\.m4a$`)

// exportedClip reports whether name is a clip of Kando's own export (ClipName), which an export into the library's folder leaves there: the library does not take it as a film, nor gather it into a folder of its own, which would break the notes that play it.
func exportedClip(name string) bool { return exportedClipName.MatchString(strings.ToLower(name)) }
