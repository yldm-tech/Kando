package films

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/films/sitename"
	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// Film is one card on the shelf or the film open on the player page.
type Film struct {
	Key, Title, Name, Source, Detail string
	Stage                            core.Stage
	JobID, VideoPath, SubtitlePath   string
	// Language is what the job translates into, such as 简体中文; empty for a video no job has processed.
	Language string
	// Progress is how far the job has gone in Stage, as its tool reports it. Thumbnail is the address of the picture the video's site gives it, empty until the download has looked the video up.
	Progress  core.Progress
	Thumbnail string
	// Untranslated is how many lines of a done film's subtitles the translation left untranslated.
	Untranslated int
	// Stopped reports a failed job that was cancelled rather than one that went wrong; Canceled reads it.
	Stopped bool
}

func List(root string, jobs []core.Job, videos []media.Video, nameOf func(core.Job) string, detailOf func(core.Job) string) []Film {
	used := map[string]bool{}
	out := make([]Film, 0, len(jobs)+len(videos))
	for i := range jobs {
		job := jobs[i]
		if !InLibrary(root, job) {
			continue
		}
		if job.VideoPath != "" {
			used[job.VideoPath] = true
		}
		out = append(out, FromJob(job, nameOf(job), detailOf(job)))
	}
	for _, video := range videos {
		if used[video.Path] || !InDir(root, video.Path) {
			continue
		}
		name := strings.TrimSuffix(video.Name, filepath.Ext(video.Name))
		out = append(out, Film{
			Key: video.Path, Title: Shorten(name, 22), Name: name, Detail: video.Path, VideoPath: video.Path,
		})
	}
	return out
}

func FromJob(job core.Job, name, detail string) Film {
	return Film{
		Key: job.ID, Title: Shorten(name, 22), Name: name, Source: job.Source,
		Detail: detail, Stage: job.Stage, JobID: job.ID,
		VideoPath: job.VideoPath, SubtitlePath: job.SubtitlePath, Language: job.Language,
		Progress: job.Progress, Thumbnail: job.Thumbnail, Untranslated: job.Untranslated,
		Stopped: errors.Is(job.Err, context.Canceled),
	}
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
	if link, ok := core.ParseLink(item.Source); ok {
		return string(link.Platform)
	}
	return sidebar.SourceLocal
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

func Find(items []Film, key string) (Film, bool) {
	for _, item := range items {
		if item.Key == key {
			return item, true
		}
	}
	return Film{}, false
}

func DisplayPath(path string) string {
	if path == "" {
		return locales.FilmsPathNotSet()
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

func Shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func StageLabel(stage core.Stage) string {
	switch stage {
	case core.StageQueued:
		return locales.FilmsStageQueued()
	case core.StageDownloading:
		return locales.FilmsStageDownloading()
	case core.StageTranscribing:
		return locales.FilmsStageTranscribing()
	case core.StageTranslating:
		return locales.FilmsStageTranslating()
	case core.StageExporting:
		return locales.FilmsStageExporting()
	case core.StageComplete:
		return locales.FilmsStageComplete()
	case core.StageFailed:
		return locales.FilmsStageFailed()
	default:
		return string(stage)
	}
}

// The done grid and the processing queue are grids of cards that keep a minimum width and fill the row, as the prototype's CSS grids (auto-fill, minmax) do.
const (
	CardMinWidth  float32 = 230
	CardGapX      float32 = 20
	CardGapY      float32 = 26
	QueueMinWidth float32 = 320
	QueueGap      float32 = 12
	// QueueShown is how many processing cards show before 显示全部.
	QueueShown = 6
)

// Columns is how many posters fit across the window: the sidebar and page padding stay fixed, and each poster keeps its minimum width.
func Columns(windowWidth float32) int {
	return columnsIn(ContentWidth(windowWidth), CardMinWidth, CardGapX)
}

// QueueColumns is how many processing cards fit across the window.
func QueueColumns(windowWidth float32) int {
	return columnsIn(ContentWidth(windowWidth), QueueMinWidth, QueueGap)
}

func columnsIn(content, min, gap float32) int {
	n := int((content + gap) / (min + gap))
	if n < 1 {
		return 1
	}
	return n
}

// ContentWidth is the main column inside the page padding.
func ContentWidth(windowWidth float32) float32 {
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

// QueueSummary is the line beside 处理中, such as "1 个进行中 · 2 个需要处理".
func QueueSummary(items []Film) string {
	active, failed := 0, 0
	for _, item := range items {
		if item.Stage == core.StageFailed {
			failed++
		} else {
			active++
		}
	}
	var parts []string
	if active > 0 {
		parts = append(parts, locales.FilmsQueueActive(active))
	}
	if failed > 0 {
		parts = append(parts, locales.FilmsQueueFailed(failed))
	}
	return strings.Join(parts, " · ")
}

// Local reports whether the film's video is a file on this machine rather than a link to download.
func Local(item Film) bool { return SourceOf(item) == sidebar.SourceLocal }

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
func SourceLabel(item Film) string {
	link, ok := core.ParseLink(item.Source)
	if !ok {
		return locales.SourceLocal()
	}
	return PlatformLabel(link)
}

// PlatformLabel names the platform of a link as the library shows it.
func PlatformLabel(link core.Link) string {
	switch link.Platform {
	case core.PlatformYouTube:
		return "YouTube"
	case core.PlatformBilibili:
		return locales.SourceBilibili()
	case core.PlatformLocal:
		return locales.SourceLocal()
	}
	if name := sitename.Of(link.Site); name != "" {
		return name
	}
	return link.Host
}

// LanguageShort is a short tag for a subtitle language, such as 简中 for 简体中文, for the badge on a poster. language is the target language a job keeps, as settings.json and library.json store it.
func LanguageShort(language string) string {
	switch language {
	case "简体中文":
		return locales.FilmsLanguageTagSimplifiedChinese()
	case "繁体中文":
		return locales.FilmsLanguageTagTraditionalChinese()
	case "English":
		return "EN"
	case "日本語":
		return locales.FilmsLanguageTagJapanese()
	case "한국어":
		return locales.FilmsLanguageTagKorean()
	case "Español":
		return "ES"
	case "Français":
		return "FR"
	case "Deutsch":
		return "DE"
	}
	return language
}

// LanguageName names a subtitle language in the interface's language (subtitle.TargetLanguageName). language is the target language a job keeps, as settings.json and library.json store it.
func LanguageName(language string) string { return subtitle.TargetLanguageName(language) }

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

func posterColor(key string) ui.Color {
	colors := []ui.Color{ui.Hex("#3a2c1e"), ui.Hex("#243044"), ui.Hex("#1e3330"), ui.Hex("#3a2430")}
	n := 0
	for _, r := range key {
		n += int(r)
	}
	return colors[n%len(colors)]
}
