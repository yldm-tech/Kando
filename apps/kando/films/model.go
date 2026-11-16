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

	"kando/apps/kando/films/sitename"
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
	// Spoken is the language the speech recognition heard in the video, as whisper's code (ja, en); empty until the job transcribed it, and for a video no job has processed.
	Spoken string
	// Progress is how far the job has gone in Stage, as its tool reports it. Thumbnail is the address of the picture the video's site gives it, empty until the download has looked the video up.
	Progress  core.Progress
	Thumbnail string
	// Untranslated is how many lines of a done film's subtitles the translation left untranslated.
	Untranslated int
	// TranslationErr is why a done film translated none of its lines, its subtitles being the original lines alone (core.Job.TranslationErr); empty for one that was translated.
	TranslationErr string
	// Stopped reports a failed job that was cancelled rather than one that went wrong; Canceled reads it.
	Stopped bool
	// Library is the name of the library the job saves into when that is not the open one, as for a job that kept running after another library was opened; empty for the open library's films.
	Library string
	// Ahead is how many jobs run before a waiting one; 0 for any other film.
	Ahead int
	// Target is the language the film's job translates into: the one a job processing or translating a done film again was asked for (core.Job.RedoLanguage), else Language.
	Target string
}

// List is the open library's films: the jobs that belong on its shelf, then the videos found in its folder that no job holds. Such a video shows the bilingual subtitles found beside it (media.Video.Subtitles), as a film copied from another library or one whose job records were lost has them. names is the title the user gave each video in Kando, by path (shelf.Library.Names), which a film shows in place of the name nameOf gives its job or its file's name.
func List(root string, jobs []core.Job, videos []media.Video, names map[string]string, nameOf func(core.Job) string, detailOf func(core.Job) string) []Film {
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
		name := nameOf(job)
		if given := names[job.VideoPath]; job.VideoPath != "" && given != "" {
			name = given
		}
		out = append(out, FromJob(job, name, detailOf(job)))
	}
	for _, video := range videos {
		if used[video.Path] || !InDir(root, video.Path) {
			continue
		}
		name := strings.TrimSuffix(video.Name, filepath.Ext(video.Name))
		if given := names[video.Path]; given != "" {
			name = given
		}
		out = append(out, Film{
			Key: video.Path, Title: Shorten(name, 22), Name: name, Detail: video.Path, VideoPath: video.Path, SubtitlePath: video.Subtitles,
		})
	}
	return out
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
		VideoPath: job.VideoPath, SubtitlePath: job.SubtitlePath, Language: job.Language, Spoken: job.SpokenLanguage,
		Progress: job.Progress, Thumbnail: job.Thumbnail, Untranslated: job.Untranslated, TranslationErr: job.TranslationErr,
		Stopped: errors.Is(job.Err, context.Canceled), Target: jobTarget(job),
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

// QueueSummary is the line beside 处理中, such as "1 个进行中 · 2 个排队 · 2 个需要处理".
func QueueSummary(m locales.Messages, items []Film) string {
	running, queued, failed := 0, 0, 0
	for _, item := range items {
		switch item.Stage {
		case core.StageFailed:
			failed++
		case core.StageQueued:
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

// PosterLanguages is the tag over a done film's poster, in the language of m: the language heard in it, as JA, and the one it was translated into only when that is not target, the language the settings translate new films into, as JA → 简中. A film whose spoken language is not known says it is bilingual in its translation's language.
func PosterLanguages(m locales.Messages, item Film, target string) string {
	if item.Spoken == "" {
		return m.FilmsPosterBilingual(LanguageShort(m, item.Language))
	}
	spoken := strings.ToUpper(item.Spoken)
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
