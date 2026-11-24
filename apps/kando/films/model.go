package films

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/films/sitename"
	"kando/apps/kando/jobfailure"
	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// Film is one card on the shelf or the film open on the player page.
type Film struct {
	Key, Title, Name, Source, Detail string
	Stage                            core.Stage
	JobID, VideoPath, SubtitlePath   string
	// Language is what the job translates into, such as 简体中文; empty for a video no job has processed.
	Language string
	// List is the list the film was added from and ListIndex its place in it (core.Job.List), which play goes on in (Next).
	List      string
	ListIndex int
	// Spoken is the language the speech recognition heard in the video, as whisper's code (ja, en); empty until the job transcribed it, and for a video no job has processed.
	Spoken string
	// SpeechLanguage is the language the user said the video is spoken in, as whisper's code, which its job is transcribed in from then on (core.Job.SpeechLanguage); empty when the user named none.
	SpeechLanguage string
	// Progress is how far the job has gone in Stage, as its tool reports it. Thumbnail is the address of the picture the video's site gives it, empty until the download has looked the video up.
	Progress  core.Progress
	Thumbnail string
	// Untranslated is how many lines of a done film's subtitles the translation left untranslated.
	Untranslated int
	// TranslationErr is why a done film translated none of its lines, its subtitles being the original lines alone (core.Job.TranslationErr); empty for one that was translated.
	// SameLanguage is true for a film heard in the language its subtitles are for, which was not translated: its subtitles are its own lines alone (core.Job.SameLanguage).
	SameLanguage   bool
	TranslationErr string
	// TranslationSource is the translation source other than the AI service whose failure TranslationErr says (core.Job.TranslationSource).
	TranslationSource string
	// PlatformSubtitles is true for a film whose original lines are its site's subtitles, and LocalLines how many of their lines the speech recognition replaced (core.Job.PlatformSubtitles).
	PlatformSubtitles bool
	LocalLines        int
	// SubtitlesOnly is true for a film that keeps no video, which streams from its site when watched (core.Job.SubtitlesOnly); VideoComing is true while its job waits to download the video, or downloads it (core.RedoVideo).
	SubtitlesOnly bool
	VideoComing   bool
	// SoundPath is the sound a link's job downloaded first, which the film plays until its video comes (core.Job.SoundPath).
	SoundPath string
	// Stopped reports a failed job that was cancelled rather than one that went wrong; Canceled reads it.
	Stopped bool
	// Failure is the step a failed job failed at and why (media.StageError), which the card reads its reason from; nil for a job that did not fail, or one whose error names no step.
	Failure *media.StageError
	// Library is the name of the library the job saves into when that is not the open one, as for a job that kept running after another library was opened; empty for the open library's films.
	Library string
	// Ahead is how many jobs run before a waiting one; 0 for any other film.
	Ahead int
	// Paused is true for a waiting film while the whole queue is paused, so none starts until it is resumed.
	Paused bool
	// Target is the language the film's job translates into: the one a job processing or translating a done film again was asked for (core.Job.RedoLanguage), else Language.
	Target string
	// Device is the paired phone that sent the job's link, empty for a film added on the desktop (core.Job.Device).
	Device string
	// PausedBy is the paired phone that paused the waiting job where it was, empty for one that is not paused (core.Job.PausedBy).
	PausedBy string
	// Stored is the ID of the storage location the film's video was moved to (shelf.Library.Stored), empty for a video in the library's folder. Its subtitles and the other files beside the video stay in the folder.
	Stored string
	// Linked is the ID of the media source the film was indexed from rather than copied (shelf.Library.Linked), empty for any other film: its video stays on the source and plays from there, while its subtitles and the other files beside it are in the library's folder, named after the placeholder path its video is said to be at.
	Linked string
	// Missing is true for a done film whose video, kept outside the library's folder where it was picked, is not on disk now: moved or renamed in Finder, deleted, or on a drive that is not connected. The film stays on the shelf with its place, its title and its lines, marked, and plays again once its video is back.
	Missing bool
	// Added is when a video kept outside the library was added to it without a job, as a large folder dropped adds its films (shelf.Library.Added); zero for any other film.
	Added time.Time
}

// List is the open library's films: the jobs that belong on its shelf, with the videos added to it without a job among them by when each was added (media.Video.Added), then the videos found in its folder that no job holds. A video without a job shows the bilingual subtitles found beside it (media.Video.Subtitles), as a film copied from another library or one whose job records were lost has them. names is the title the user gave each video in Kando, by path (shelf.Library.Names), which a film shows in place of the name nameOf gives its job or its file's name.
func List(root string, jobs []core.Job, videos []media.Video, names map[string]string, nameOf func(core.Job) string, detailOf func(core.Job) string) []Film {
	used := map[string]bool{}
	out := make([]Film, 0, len(jobs)+len(videos))
	// done is when each film of a job in out was done, and busy whether it is waiting or being processed now, which is newer than any added; a done film that kept no time, as one an older Kando made, is older than any.
	var done []time.Time
	var busy []bool
	for i := range jobs {
		job := jobs[i]
		if !InLibrary(root, job) {
			continue
		}
		if job.VideoPath != "" {
			used[job.VideoPath] = true
		}
		name := nameOf(job)
		if given := names[job.VideoPath]; job.VideoPath != "" && given != "" {
			name = given
		}
		out = append(out, FromJob(job, name, detailOf(job)))
		done, busy = append(done, job.CompletedAt), append(busy, !job.Stage.Done())
	}
	var added, found []Film
	for _, video := range videos {
		if used[video.Path] || (video.Added.IsZero() && !InDir(root, video.Path)) {
			continue
		}
		used[video.Path] = true
		name := strings.TrimSuffix(video.Name, filepath.Ext(video.Name))
		if given := names[video.Path]; given != "" {
			name = given
		}
		f := Film{Key: video.Path, Title: Shorten(name, 22), Name: name, Detail: video.Path, VideoPath: video.Path, SubtitlePath: video.Subtitles, Added: video.Added}
		if video.Added.IsZero() {
			found = append(found, f)
		} else {
			added = append(added, f)
		}
	}
	if len(added) == 0 {
		return append(out, found...)
	}
	slices.SortStableFunc(added, func(x, y Film) int { return y.Added.Compare(x.Added) })
	merged := make([]Film, 0, len(out)+len(added)+len(found))
	i := 0
	for _, f := range added {
		for i < len(out) && (busy[i] || !f.Added.After(done[i])) {
			merged = append(merged, out[i])
			i++
		}
		merged = append(merged, f)
	}
	merged = append(merged, out[i:]...)
	return append(merged, found...)
}

// Videos is the videos the shelf of the library at root lists beside its jobs: those of its folder (media.ListVideos), then those no job holds that were moved to a storage location (MovedVideos), whose paths are moved, then those added to it without a job (AddedVideos), by path with when each was added. The open library's shelf, the libraries' counts, the paired phone and the control socket all list a library's films from it.
func Videos(root string, jobs []core.Job, moved []string, added map[string]time.Time) ([]media.Video, error) {
	videos, err := media.ListVideos(root)
	if err != nil {
		return nil, err
	}
	return AddedVideos(root, jobs, MovedVideos(root, jobs, videos, moved), added), nil
}

// AddedVideos is videos and then the videos of added, those added to the library at root without a job, by path with when each was added (shelf.Library.Added), that neither a job of that library (InLibrary) nor videos holds, the newest first. A job of another library for the same video does not hide it, as that job is on the other library's shelf, not this one. Each shows the bilingual subtitles beside it, as a video of the folder does, and is named after its file, or after its folder for a video.<ext>. One whose file is not there now is listed all the same, as its drive may be taken out; the shelf marks it (Film.Missing).
func AddedVideos(root string, jobs []core.Job, videos []media.Video, added map[string]time.Time) []media.Video {
	if len(added) == 0 {
		return videos
	}
	held := map[string]bool{}
	for _, job := range jobs {
		if job.VideoPath != "" && InLibrary(root, job) {
			held[filepath.Clean(job.VideoPath)] = true
		}
	}
	for _, v := range videos {
		held[filepath.Clean(v.Path)] = true
	}
	var extra []media.Video
	for path, at := range added {
		if path == "" || held[filepath.Clean(path)] {
			continue
		}
		held[filepath.Clean(path)] = true
		name := filepath.Base(path)
		if strings.TrimSuffix(name, filepath.Ext(name)) == "video" {
			// The shelf names a film by its file's name without its extension, so the folder's keeps a dot in it.
			name = filepath.Base(filepath.Dir(path)) + filepath.Ext(path)
		}
		v := media.Video{Path: path, Name: name, Added: at}
		if sub := strings.TrimSuffix(path, filepath.Ext(path)) + ".bilingual.srt"; fileThere(sub) {
			v.Subtitles = sub
		}
		extra = append(extra, v)
	}
	slices.SortFunc(extra, func(x, y media.Video) int {
		if c := y.Added.Compare(x.Added); c != 0 {
			return c
		}
		return strings.Compare(x.Path, y.Path)
	})
	return append(slices.Clip(videos), extra...)
}

// MovedVideos is videos, the videos found in the library's folder at root, followed by one for each of moved, the paths of videos moved out of that folder to a storage location (shelf.Library.Stored), that no job holds and the folder no longer has: listed by the path it had, which the film is still known by, with the bilingual subtitles left beside it when they are there. A video no job processed is on the shelf by its file alone (List), so without this its film would leave the shelf as its video leaves the folder. It reads the disk, for the subtitles, so the library's refresh calls it rather than a binding.
func MovedVideos(root string, jobs []core.Job, videos []media.Video, moved []string) []media.Video {
	held := map[string]bool{}
	for _, job := range jobs {
		if job.VideoPath != "" {
			held[filepath.Clean(job.VideoPath)] = true
		}
	}
	for _, v := range videos {
		held[filepath.Clean(v.Path)] = true
	}
	var extra []media.Video
	for _, path := range moved {
		if path == "" || held[filepath.Clean(path)] || !InDir(root, path) {
			continue
		}
		held[filepath.Clean(path)] = true
		v := media.Video{Path: path, Name: filepath.Base(path)}
		if sub := strings.TrimSuffix(path, filepath.Ext(path)) + ".bilingual.srt"; fileThere(sub) {
			v.Subtitles = sub
		}
		extra = append(extra, v)
	}
	slices.SortFunc(extra, func(x, y media.Video) int { return strings.Compare(x.Path, y.Path) })
	return append(slices.Clip(videos), extra...)
}

// fileThere reports whether path is a file on disk.
func fileThere(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Queue is the films of 处理中 in the shelf's order, newest first: the open library's jobs that are waiting, running, failed or were cancelled (InQueue), and the jobs elsewhere reports the queue runs, or will run, for another library, each with Library naming that library.
func Queue(root string, jobs []core.Job, elsewhere func(core.Job) bool, nameOf func(core.Job) string, detailOf func(core.Job) string) []Film {
	var out []Film
	for _, job := range jobs {
		switch {
		case InLibrary(root, job):
			if item := FromJob(job, nameOf(job), detailOf(job)); InQueue(item) {
				out = append(out, item)
			}
		case elsewhere(job):
			item := FromJob(job, nameOf(job), detailOf(job))
			item.Library = LibraryName(job.LibraryDir)
			out = append(out, item)
		}
	}
	return out
}

func FromJob(job core.Job, name, detail string) Film {
	return Film{
		Key: job.ID, Title: Shorten(name, 22), Name: name, Source: job.Source,
		Detail: detail, Stage: job.Stage, JobID: job.ID,
		VideoPath: job.VideoPath, SubtitlePath: job.SubtitlePath, Language: job.Language, Spoken: job.SpokenLanguage, SpeechLanguage: job.SpeechLanguage,
		Progress: job.Progress, Thumbnail: job.Thumbnail, Untranslated: job.Untranslated, TranslationErr: job.TranslationErr, TranslationSource: job.TranslationSource, SameLanguage: job.SameLanguage,
		Stopped: errors.Is(job.Err, context.Canceled), Failure: jobfailure.Of(job), Target: jobTarget(job), Device: job.Device, PausedBy: job.PausedBy,
		PlatformSubtitles: job.PlatformSubtitles, LocalLines: job.LocalLines,
		SubtitlesOnly: job.SubtitlesOnly, VideoComing: job.Redo == core.RedoVideo && !job.Stage.Done(), SoundPath: job.SoundPath,
		List: job.List, ListIndex: job.ListIndex,
	}
}

// jobTarget is the language job translates into (Film.Target).
func jobTarget(job core.Job) string {
	if job.Redo != core.RedoMissing && strings.TrimSpace(job.RedoLanguage) != "" {
		return job.RedoLanguage
	}
	return job.Language
}

// InLibrary reports whether job belongs on the open library's shelf. A finished job follows its video file. A job that has not downloaded yet, or whose video is a local file kept where it was picked, follows the library it was started in.
func InLibrary(root string, job core.Job) bool {
	if root == "" {
		return false
	}
	started := job.LibraryDir != "" && filepath.Clean(job.LibraryDir) == filepath.Clean(root)
	if job.VideoPath != "" {
		if InDir(root, job.VideoPath) {
			return true
		}
		link, ok := core.ParseLink(job.Source)
		return started && ok && link.Platform == core.PlatformLocal
	}
	if job.LibraryDir != "" {
		return started
	}
	return !job.Stage.Done()
}

func InDir(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// SourceOf is the sidebar source a film comes from: the platform core.ParseLink reads from its source, or sidebar.SourceLocal for a film with no source it reads, such as a video found in the library folder.
func SourceOf(item Film) string {
	link, ok := core.ParseLink(item.Source)
	// A film copied from a media source is a file of the library, as a local one is.
	if !ok || link.Platform == core.PlatformSource {
		return sidebar.SourceLocal
	}
	return string(link.Platform)
}

// BySource keeps the films that come from source; sidebar.SourceAll keeps them all.
func BySource(items []Film, source string) []Film {
	if source == sidebar.SourceAll {
		return items
	}
	var out []Film
	for _, item := range items {
		if SourceOf(item) == source {
			out = append(out, item)
		}
	}
	return out
}

// Heard is the language a film is filed under by what is spoken in it, as whisper's code: the one the speech recognition heard (Film.Spoken), or, before it told one, as for a film transcribed before Kando kept it, the one the user named for the film (Film.SpeechLanguage). Empty when neither is known.
func Heard(item Film) string {
	if item.Spoken != "" {
		return item.Spoken
	}
	return item.SpeechLanguage
}

// BySpoken keeps the films heard in code, whisper's code for a language (Heard); sidebar.SpokenAll keeps them all.
func BySpoken(items []Film, code string) []Film {
	if code == sidebar.SpokenAll {
		return items
	}
	var out []Film
	for _, item := range items {
		if Heard(item) == code {
			out = append(out, item)
		}
	}
	return out
}

// SpokenCounts is how many of items were heard in each language (Heard), those whose language is not known left out: the language most were heard in first, languages as many films were heard in by their codes.
func SpokenCounts(items []Film) []sidebar.SpokenCount {
	n := map[string]int{}
	for _, item := range items {
		if code := Heard(item); code != "" {
			n[code]++
		}
	}
	out := make([]sidebar.SpokenCount, 0, len(n))
	for code, count := range n {
		out = append(out, sidebar.SpokenCount{Code: code, Count: count})
	}
	slices.SortFunc(out, func(x, y sidebar.SpokenCount) int {
		if c := cmp.Compare(y.Count, x.Count); c != 0 {
			return c
		}
		return strings.Compare(x.Code, y.Code)
	})
	return out
}

// MatchesTitle reports whether the film's name, the whole one the page shows, holds query, ignoring case and the spaces around query. An empty query matches every film. The library's search field filters the done grid with it, and leaves 处理中 whole; `kando list [query]` filters every film with it, queued or done, so the two agree on which done films match.
func MatchesTitle(item Film, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	return query == "" || strings.Contains(strings.ToLower(filmName(item)), query)
}

// ByTitle keeps the films whose names hold query (MatchesTitle); an empty query keeps them all.
func ByTitle(items []Film, query string) []Film {
	if strings.TrimSpace(query) == "" {
		return items
	}
	var out []Film
	for _, item := range items {
		if MatchesTitle(item, query) {
			out = append(out, item)
		}
	}
	return out
}

// Shown is the films of items the library page's grid shows, in their order: the done ones, no longer in 处理中, from source (BySource), heard in spoken (BySpoken), watched as far as watch says (ByWatch, by what p knows) and whose names hold query (ByTitle). Return in the sidebar's search field acts on the same films, so it never opens one the grid hides, and the sidebar counts the films each watch state would show among those of the others (sidebar.WatchAll).
func Shown(items []Film, p Progress, source, spoken, watch, query string) []Film {
	var done []Film
	for _, item := range BySource(items, source) {
		if !InQueue(item) {
			done = append(done, item)
		}
	}
	return ByTitle(ByWatch(BySpoken(done, spoken), p, watch), query)
}

func Find(items []Film, key string) (Film, bool) {
	for _, item := range items {
		if item.Key == key {
			return item, true
		}
	}
	return Film{}, false
}

func DisplayPath(m locales.Messages, path string) string {
	if path == "" {
		return m.FilmsPathNotSet()
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(path, prefix) {
		return "~" + string(filepath.Separator) + path[len(prefix):]
	}
	return path
}

// LibraryLabels names each library by its title, or by its path where two share a title: the library drop-down of Settings › 通用, and File › 切换资料库 and the Dock menu of the menu bar.
func LibraryLabels(m locales.Messages, dirs []string) []string {
	count := map[string]int{}
	for _, dir := range dirs {
		count[sidebar.Title(dir)]++
	}
	labels := make([]string, len(dirs))
	for i, dir := range dirs {
		labels[i] = sidebar.Title(dir)
		if count[labels[i]] > 1 {
			labels[i] = DisplayPath(m, dir)
		}
	}
	return labels
}

func Shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func StageLabel(m locales.Messages, stage core.Stage) string {
	switch stage {
	case core.StageQueued:
		return m.FilmsStageQueued()
	case core.StageDownloading:
		return m.FilmsStageDownloading()
	case core.StageTranscribing:
		return m.FilmsStageTranscribing()
	case core.StageTranslating:
		return m.FilmsStageTranslating()
	case core.StageExporting:
		return m.FilmsStageExporting()
	case core.StageComplete:
		return m.FilmsStageComplete()
	case core.StageFailed:
		return m.FilmsStageFailed()
	default:
		return string(stage)
	}
}

// The done grid and the processing queue are grids of cards that keep a minimum width and fill the row, as the prototype's CSS grids (auto-fill, minmax) do.
const (
	CardMinWidth  ns.Points = 230
	CardGapX      ns.Points = 20
	CardGapY      ns.Points = 26
	QueueMinWidth ns.Points = 320
	QueueGap      ns.Points = 12
	// QueueShown is how many processing cards show before 显示全部.
	QueueShown = 6
)

// Columns is how many posters fit across the window: the sidebar and page padding stay fixed, and each poster keeps its minimum width.
func Columns(windowWidth ns.Points) int {
	return columnsIn(ContentWidth(windowWidth), CardMinWidth, CardGapX)
}

// QueueColumns is how many processing cards fit across the window.
func QueueColumns(windowWidth ns.Points) int {
	return columnsIn(ContentWidth(windowWidth), QueueMinWidth, QueueGap)
}

func columnsIn(content, min, gap ns.Points) int {
	n := int((content + gap) / (min + gap))
	if n < 1 {
		return 1
	}
	return n
}

// ContentWidth is the main column inside the page padding.
func ContentWidth(windowWidth ns.Points) ns.Points {
	w := windowWidth - sidebar.Width - PagePadX*2
	if w < 1 {
		return 1
	}
	return w
}

// InQueue reports whether a film belongs in 处理中 rather than the done grid: a job that is waiting, running, failed or was cancelled. A finished job and a video no job has processed are done.
func InQueue(item Film) bool {
	return item.JobID != "" && item.Stage != core.StageComplete
}

// NotTranslated is the films of items that are done but were not translated at all, their subtitles being the original lines alone (Film.TranslationErr), in the order of items: those Translate N Films and the message bar's Translate translate (App.TranslateFilms). A film whose job is waiting or running again is not one of them.
func NotTranslated(items []Film) []Film {
	var out []Film
	for _, item := range items {
		if item.JobID != "" && item.Stage == core.StageComplete && item.TranslationErr != "" {
			out = append(out, item)
		}
	}
	return out
}

// Refillable reports whether item is a done film the AI service left untranslated as a service gone for a while does, rate-limited, failing on its side, timing out or out of reach (media.ClassifyTranslate), so its lines left can be translated once the service answers again. A film a translation source of the user's own left untranslated (Film.TranslationSource) is not, as Kando cannot tell why it failed; nor one the service refused, as one whose key or credit ran out, which waits for the user.
func Refillable(item Film) bool {
	if item.JobID == "" || item.Stage != core.StageComplete || item.TranslationErr == "" || item.TranslationSource != "" || item.SameLanguage {
		return false
	}
	switch media.ClassifyTranslate(item.TranslationErr) {
	case media.TranslateBusy, media.TranslateTimeout, media.TranslateNetwork:
		return true
	}
	return false
}

// Running reports whether the film's job is at work on a stage.
func Running(item Film) bool {
	return item.JobID != "" && item.Stage != core.StageQueued && !item.Stage.Done()
}

// Canceled reports whether the film's job failed because it was cancelled.
func Canceled(item Film) bool {
	return item.Stage == core.StageFailed && item.Stopped
}

// QueueOrder sorts the processing cards as they will run: the running job, then the waiting ones oldest first, then the failed ones newest first. items is newest first, as the shelf keeps its jobs.
func QueueOrder(items []Film) []Film {
	var running, waiting, failed []Film
	for _, item := range items {
		switch {
		case item.Stage == core.StageFailed:
			failed = append(failed, item)
		case item.Stage == core.StageQueued:
			waiting = append(waiting, item)
		default:
			running = append(running, item)
		}
	}
	slices.Reverse(waiting)
	return slices.Concat(running, waiting, failed)
}

// QueueSummary is the line beside 处理中, such as "1 个进行中 · 2 个排队 · 2 个需要处理". A cancelled job is none of them, as the sidebar's count and the bulk buttons leave it out.
func QueueSummary(m locales.Messages, items []Film) string {
	running, queued, failed := 0, 0, 0
	for _, item := range items {
		switch {
		case Canceled(item):
		case item.Stage == core.StageFailed:
			failed++
		case item.Stage == core.StageQueued:
			queued++
		default:
			running++
		}
	}
	var parts []string
	if running > 0 {
		parts = append(parts, m.FilmsQueueActive(running))
	}
	if queued > 0 {
		parts = append(parts, m.FilmsQueueQueued(queued))
	}
	if failed > 0 {
		parts = append(parts, m.FilmsQueueFailed(failed))
	}
	return strings.Join(parts, " · ")
}

// Local reports whether the film's video is a file on this machine rather than a link to download, or a media source's video to copy first.
func Local(item Film) bool {
	link, ok := core.ParseLink(item.Source)
	return SourceOf(item) == sidebar.SourceLocal && !(ok && link.Platform == core.PlatformSource)
}

// Steps is how many stages a film's job goes through and how many of them are behind it: a local video is not downloaded, so it has three (生成字幕, 翻译, 导出) and a link four. A queued job has none behind it; a finished one has them all.
func Steps(item Film) (total, done int) {
	stages := []core.Stage{core.StageDownloading, core.StageTranscribing, core.StageTranslating, core.StageExporting}
	if Local(item) {
		stages = stages[1:]
	}
	if item.Stage == core.StageComplete {
		return len(stages), len(stages)
	}
	return len(stages), max(slices.Index(stages, item.Stage), 0)
}

// StepFills is how much of each of a film's stages is behind it, from 0 to 1, in the order Steps counts them: the stages behind it whole, the one it is in as far as its tool has said, and the ones ahead empty.
func StepFills(item Film) []float64 {
	total, done := Steps(item)
	fills := make([]float64, total)
	for i := range fills {
		switch {
		case i < done:
			fills[i] = 1
		case i == done && Running(item) && item.Progress.Known:
			fills[i] = item.Progress.Fraction
		}
	}
	return fills
}

// SourceLabel names where a film comes from: YouTube, 哔哩哔哩, the site's name, or 本地文件.
func SourceLabel(m locales.Messages, item Film) string {
	link, ok := core.ParseLink(item.Source)
	if !ok {
		return m.SourceLocal()
	}
	return PlatformLabel(m, link)
}

// PlatformLabel names the platform of a link as the library shows it.
func PlatformLabel(m locales.Messages, link core.Link) string {
	switch link.Platform {
	case core.PlatformYouTube:
		return "YouTube"
	case core.PlatformBilibili:
		return m.SourceBilibili()
	case core.PlatformLocal:
		return m.SourceLocal()
	case core.PlatformSource:
		return m.MediaSourceSidebar()
	}
	if name := sitename.In(m, link.Site); name != "" {
		return name
	}
	return link.Host
}

// LanguageShort is a short tag for a subtitle language, such as 简中 for 简体中文, for the badge on a poster. language is the target language a job keeps, as settings.json and library.json store it.
func LanguageShort(m locales.Messages, language string) string {
	switch language {
	case "简体中文":
		return m.FilmsLanguageTagSimplifiedChinese()
	case "繁体中文":
		return m.FilmsLanguageTagTraditionalChinese()
	case "English":
		return "EN"
	case "日本語":
		return m.FilmsLanguageTagJapanese()
	case "한국어":
		return m.FilmsLanguageTagKorean()
	case "Español":
		return "ES"
	case "Français":
		return "FR"
	case "Deutsch":
		return "DE"
	case "Português":
		return "PT"
	case "Italiano":
		return "IT"
	case "Русский":
		return "RU"
	case "Nederlands":
		return "NL"
	case "Polski":
		return "PL"
	case "Українська":
		return "UK"
	case "Türkçe":
		return "TR"
	case "العربية":
		return "AR"
	case "हिन्दी":
		return "HI"
	case "Bahasa Indonesia":
		return "ID"
	case "Tiếng Việt":
		return "VI"
	case "ไทย":
		return "TH"
	}
	return language
}

// PosterLanguages is the tag over a done film's poster, in the language of m: the language heard in it (Heard), as JA, and the one it was translated into only when that is not target, the language the settings translate new films into, as JA → 简中. A film whose spoken language is not known says it is bilingual in its translation's language.
func PosterLanguages(m locales.Messages, item Film, target string) string {
	heard := Heard(item)
	if heard == "" {
		return m.FilmsPosterBilingual(LanguageShort(m, item.Language))
	}
	spoken := strings.ToUpper(heard)
	if item.Language == target {
		return spoken
	}
	return m.FilmsPosterLanguages(spoken, LanguageShort(m, item.Language))
}

// LanguageName names a subtitle language in the language of m, as the settings and the player name it (subtitle.TargetLanguageName): Simplified Chinese and Japanese in English. language is the target language a job keeps, as settings.json and library.json store it; one typed into an older version is shown as it is.
func LanguageName(m locales.Messages, language string) string {
	switch language {
	case "简体中文":
		return m.SettingsLanguageSimplifiedChinese()
	case "繁体中文":
		return m.SettingsLanguageTraditionalChinese()
	case "English":
		return m.SettingsLanguageEnglish()
	case "日本語":
		return m.SettingsLanguageJapanese()
	case "한국어":
		return m.SettingsLanguageKorean()
	case "Español":
		return m.SettingsLanguageSpanish()
	case "Français":
		return m.SettingsLanguageFrench()
	case "Deutsch":
		return m.SettingsLanguageGerman()
	case "Português":
		return m.SettingsLanguagePortuguese()
	case "Italiano":
		return m.SettingsLanguageItalian()
	case "Русский":
		return m.SettingsLanguageRussian()
	case "Nederlands":
		return m.SettingsLanguageDutch()
	case "Polski":
		return m.SettingsLanguagePolish()
	case "Українська":
		return m.SettingsLanguageUkrainian()
	case "Türkçe":
		return m.SettingsLanguageTurkish()
	case "العربية":
		return m.SettingsLanguageArabic()
	case "हिन्दी":
		return m.SettingsLanguageHindi()
	case "Bahasa Indonesia":
		return m.SettingsLanguageIndonesian()
	case "Tiếng Việt":
		return m.SettingsLanguageVietnamese()
	case "ไทย":
		return m.SettingsLanguageThai()
	}
	return language
}

// Clock writes a duration as a player shows it: 3:05, or 1:02:03 past an hour.
func Clock(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	if s < 0 {
		s = 0
	}
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func posterColor(key string) ns.Color {
	colors := []ns.Color{ns.RGB(0x3a2c1e), ns.RGB(0x243044), ns.RGB(0x1e3330), ns.RGB(0x3a2430)}
	n := 0
	for _, r := range key {
		n += int(r)
	}
	return colors[n%len(colors)]
}
