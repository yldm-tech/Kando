package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"kando/apps/kando/feedback"
	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/films/sitename"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// feedbackUI is the state of the 反馈问题 dialog (feedback/): the form, whether it is open, and what the app learned for it.
type feedbackUI struct {
	open bool
	form feedback.Form
	// copied is the body 拷贝正文 copied last, so the button reads 已拷贝 until the form changes the body.
	copied string
	// sending is set while the system is being asked to open the issue in the browser; failed once no app opened it, which FeedbackView says in the interface's language.
	sending bool
	failed  bool
	// gen counts the dialog's openings, so a browser that answers after the dialog closed and opened again does not close the new one.
	gen int
	// release gives back the window's modal flag (holdModal) the dialog holds while it is open; feedbackClose runs it as the dialog closes.
	release func()
	// system is the system's name, version and architecture, as feedbackDetectSystem found it once the window existed.
	system string
	// job is the job the dialog reports, opened from the 报告… of its failed card (ReportFailure), while no film is open; "" otherwise.
	job string
}

func newFeedbackUI() feedbackUI {
	return feedbackUI{system: feedback.SystemName(runtime.GOOS, "", runtime.GOARCH)}
}

// feedbackHooks: the dialog learns the system's version once the window exists, and unticks what it can no longer attach while it is open.
func (a *app) feedbackHooks() areaHooks {
	return areaHooks{
		ready:       a.feedbackDetectSystem,
		afterChange: a.feedbackSettleForm,
	}
}

// feedbackKeys are the dialog's rows of the shortcut table, in the 帮助 menu without a key, as the prototype has them: 反馈问题…, and 反馈这部影片的字幕… while a film is open or 反馈字幕质量… otherwise.
func (a *app) feedbackKeys(cx *ns.Scope) []shortcut {
	subs := locales.FeedbackMenuSubs(cx)
	if a.openKey != "" {
		subs = locales.FeedbackMenuFilmSubs(cx)
	}
	return []shortcut{
		{Group: groupHelp, Label: locales.FeedbackMenuReport(cx), Run: func() { a.openFeedback(feedback.KindBug) }},
		{Group: groupHelp, Label: subs, Run: func() { a.openFeedback(feedback.KindSubs) }},
	}
}

// openFeedback opens the dialog afresh for kind. It attaches the app's and the tools' versions, and the open film's state when a film is open; the subtitle line stays off until the user ticks it, as it goes to a public issue. Feedback on a film's subtitles is titled after the film, in Chinese as the rest of the issue (feedback.Report.Body). A playing film pauses, as the dialog covers it.
func (a *app) openFeedback(kind feedback.Kind) {
	a.feedback.job = ""
	form := feedback.Form{Kind: int(kind), Env: true, Tools: true}
	if item, ok := a.feedbackFilm(); ok {
		form.Film = true
		if kind == feedback.KindSubs {
			form.Title = "字幕问题：" + filmName(item)
		}
	}
	if a.player.play != nil && a.player.play.Playing() {
		a.player.play.toggle()
	}
	a.feedback.open = true
	a.feedback.form = form
	a.feedback.copied, a.feedback.failed, a.feedback.sending = "", false, false
	a.feedback.gen++
	if a.feedback.release == nil {
		a.feedback.release = a.holdModal()
	}
}

// feedbackClose closes the dialog and gives back the window's modal flag, however it closed: by 取消, Escape or once the browser opened the issue.
func (a *app) feedbackClose() {
	a.feedback.open = false
	if a.feedback.release != nil {
		a.feedback.release()
		a.feedback.release = nil
	}
}

// feedbackSettleForm unticks, while the dialog is open, the rows of 附带信息 that have nothing to attach any more, as when the film closed or its subtitles went, so they do not come back ticked once they have something again, and reports whether it unticked one. It runs after every change of the app's state (areaHooks.afterChange).
func (a *app) feedbackSettleForm() bool {
	form := &a.feedback.form
	if !a.feedback.open || (!form.Film && !form.Subs) {
		return false
	}
	facts := a.feedbackFacts()
	changed := false
	if form.Film && facts.Film == nil {
		form.Film, changed = false, true
	}
	if form.Subs && facts.Excerpt == nil {
		form.Subs, changed = false, true
	}
	return changed
}

// ReportFailure opens the dialog to report the failure of job id, from its card, with the job's stage and error attached as an open film's are (feedbackFilmLines).
func (a *app) ReportFailure(id string) {
	a.openFeedback(feedback.KindBug)
	a.feedback.job = id
	if _, ok := a.feedbackFilm(); ok {
		a.feedback.form.Film = true
	}
}

// feedbackFilm is the film the player shows, while one is open, or else the job whose failure the dialog reports (ReportFailure).
func (a *app) feedbackFilm() (films.Film, bool) {
	key := a.openKey
	if key == "" {
		key = a.feedback.job
	}
	if key == "" {
		return films.Film{}, false
	}
	return films.Find(a.films(), key)
}

// FailureDetail is the whole error of a failed job, all its lines, as its card's tooltip shows it, with the credentials the app holds taken out (feedbackSecrets), as the log takes them.
func (a *app) FailureDetail(item films.Film) string {
	job, ok := a.library.Find(item.JobID)
	if !ok || job.Err == nil {
		return ""
	}
	return feedback.Scrubber{Secrets: a.feedbackSecrets()}.NoSecrets(job.Err.Error())
}

// filmName is the film's full name, or its short title without one.
func filmName(item films.Film) string {
	if item.Name != "" {
		return item.Name
	}
	return item.Title
}

// feedbackFacts is what the app knows for the issue now: its version and the system's, the tools' versions and the models in use, the open film's state and the subtitle line at the playhead. None of it is read from disk here; the tools' versions are the ones the settings asked for (loadToolVersions).
func (a *app) feedbackFacts() feedback.Facts {
	facts := feedback.Facts{App: "看懂 " + appVersion, OS: a.feedback.system}
	tools, _ := a.pipeline.(media.Tools)
	for _, tool := range []struct{ name, path string }{{"yt-dlp", tools.YTDLP}, {"ffmpeg", tools.FFmpeg}, {"whisper-cli", tools.Whisper}} {
		facts.Tools = append(facts.Tools, feedback.Tool{Name: tool.name, Version: a.ToolVersion(tool.path), Missing: tool.path == ""})
	}
	if tools.WhisperModel != "" {
		facts.Speech = filepath.Base(tools.WhisperModel)
	}
	facts.Translate = feedbackTranslator(tools.Translator.BaseURL, tools.Translator.Model)
	item, ok := a.feedbackFilm()
	if !ok {
		return facts
	}
	facts.Film = a.feedbackFilmLines(item)
	if cue, ok := feedbackCue(a.player.subs.cues, a.PlayHead()-a.player.film.delay); ok && a.player.subs.key == item.Key {
		translation, original := film.SplitCue(cue.Text)
		facts.Excerpt = &feedback.Excerpt{At: cue.Start, Original: original, Translation: translation}
	}
	return facts
}

// feedbackTranslator names the translation service the pipeline uses at base, as the AI service presets name it (OpenAI 兼容 for an address none of them has), and its model, never its address or key.
func feedbackTranslator(base, model string) string {
	preset, _ := settings.PresetByID(locales.In(feedbackIssueLanguage), settings.ProviderID(base))
	name := preset.Name
	if model = strings.TrimSpace(model); model != "" {
		name += " · " + model
	}
	return name
}

// feedbackFilmLines is the open film's state as the issue lists it, in Chinese as the rest of the issue (feedback.Report.Body): where it comes from, the language it is translated into, how far its job got and why it failed, and how many of its subtitles have a translation. The film's link and folder are left out; feedback.NoLinks and feedback.NoPaths take links and paths out of the errors, as a tool writes them.
func (a *app) feedbackFilmLines(item films.Film) []string {
	lines := []string{"来源：" + feedbackSource(item)}
	if item.Language != "" {
		lines = append(lines, "字幕语言："+item.Language)
	}
	if job, ok := a.library.Find(item.JobID); ok {
		stage := feedbackStage(job.Stage)
		if job.Stage == core.StageFailed && job.Err != nil {
			stage += "：" + feedback.NoPaths(feedback.NoLinks(job.Err.Error()))
		}
		lines = append(lines, "处理："+stage)
	} else {
		lines = append(lines, "处理：没有处理过")
	}
	if a.player.subs.key == item.Key && a.player.subs.ready {
		switch {
		case a.player.subs.err != "":
			lines = append(lines, "字幕："+feedback.NoPaths(feedback.NoLinks(playerErrTextIn(locales.In(feedbackIssueLanguage), a.player.subs.err))))
		case len(a.player.subs.cues) > 0:
			missing := 0
			for _, cue := range a.player.subs.cues {
				if translation, _ := film.SplitCue(cue.Text); translation == "" {
					missing++
				}
			}
			line := "字幕：" + strconv.Itoa(len(a.player.subs.cues)) + " 句"
			if missing > 0 {
				line += " · " + strconv.Itoa(missing) + " 句没有译文"
			}
			lines = append(lines, line)
		}
	}
	if length := a.PlayLength(); length > 0 {
		lines = append(lines, "时长："+feedback.Clock(length)+" · 播放到 "+feedback.Clock(a.PlayHead()))
	}
	return lines
}

// feedbackSource names where the film comes from as the issue writes it, in Chinese whatever the interface's language: the platform of its link, or 本地文件.
func feedbackSource(item films.Film) string {
	link, ok := core.ParseLink(item.Source)
	if !ok {
		return "本地文件"
	}
	switch link.Platform {
	case core.PlatformYouTube:
		return "YouTube"
	case core.PlatformBilibili:
		return "哔哩哔哩"
	case core.PlatformLocal:
		return "本地文件"
	}
	if name := sitename.In(locales.In(feedbackIssueLanguage), link.Site); name != "" {
		return name
	}
	return link.Host
}

// feedbackIssueLanguage is the language the issue is written in, whatever the interface's: the maintainers read the issues in one language, so their headings and facts read alike.
const feedbackIssueLanguage = "zh-Hans"

// feedbackStage names a job's stage as the issue writes it.
func feedbackStage(stage core.Stage) string {
	switch stage {
	case core.StageQueued:
		return "排队中"
	case core.StageDownloading:
		return "下载中"
	case core.StageTranscribing:
		return "识别中"
	case core.StageTranslating:
		return "翻译中"
	case core.StageExporting:
		return "导出中"
	case core.StageComplete:
		return "已完成"
	case core.StageFailed:
		return "失败"
	}
	return string(stage)
}

// feedbackCue is the subtitle line the issue attaches: the one at at, or the last one started before it between two lines, or the first before any has started.
func feedbackCue(cues []core.Segment, at time.Duration) (core.Segment, bool) {
	if cue, ok := film.CueAt(cues, at); ok {
		return cue, true
	}
	found := -1
	for i, cue := range cues {
		if cue.Start > at {
			break
		}
		found = i
	}
	if found < 0 && len(cues) > 0 {
		found = 0
	}
	if found < 0 {
		return core.Segment{}, false
	}
	return cues[found], true
}

// feedbackScrubber takes out of the issue what must not reach it: every path becomes <路径>, the files and folders the app knows among them however they are named (feedbackPaths), the home folder on its own becomes ~, and every credential the app holds, the AI service's key and address, the account's and the storage's (cloudSecrets), and the values of the KANDO_ and EVERYAPI_ environment variables, become ***.
func (a *app) feedbackScrubber() feedback.Scrubber {
	home, _ := os.UserHomeDir()
	s := feedback.Scrubber{Home: home, Paths: a.feedbackPaths(home)}
	// A preset's public address names the service, which the issue says anyway; only an address of the user's own is a secret.
	for _, secret := range a.feedbackSecrets() {
		if !feedbackPublicEndpoint(secret) {
			s.Secrets = append(s.Secrets, secret)
		}
	}
	return s
}

// feedbackSecrets are the credentials the app holds, trimmed, without the empty ones: the AI service's key and address, the account's and the storage's (cloudSecrets), and the values of the KANDO_ and EVERYAPI_ environment variables. The issue and the log file (state_logs.go) leave them out.
func (a *app) feedbackSecrets() []string {
	tools, _ := a.pipeline.(media.Tools)
	secrets := []string{a.prefs.llmKey, a.prefs.llmBase, tools.Translator.APIKey, tools.Translator.BaseURL}
	secrets = append(secrets, a.cloud.cloudSecrets()...)
	for _, kv := range os.Environ() {
		name, value, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "KANDO_") || strings.HasPrefix(name, "EVERYAPI_") {
			secrets = append(secrets, value)
		}
	}
	kept := secrets[:0]
	for _, secret := range secrets {
		if secret = strings.TrimSpace(secret); secret != "" {
			kept = append(kept, secret)
		}
	}
	return kept
}

// feedbackPaths are the files and folders the app knows: the libraries, the folder downloads go to, the tools and the speech model, every scanned video, the subtitles found beside it and its folder, and every job's video and subtitles and the folders they are in. Their names can have spaces and punctuation a path written in text is not taken to go on over, as a video's title does. The home folder and the folders above it are left out, as the home folder on its own is written as ~.
func (a *app) feedbackPaths(home string) []string {
	snap := a.snap()
	tools, _ := a.pipeline.(media.Tools)
	paths := append([]string{snap.LibraryDir, tools.OutputDir, tools.YTDLP, tools.FFmpeg, tools.Whisper, tools.WhisperModel}, snap.Libraries...)
	for _, video := range snap.Videos {
		paths = append(paths, video.Path, filepath.Dir(video.Path), video.Subtitles)
	}
	for _, job := range a.library.Jobs {
		paths = append(paths, job.LibraryDir, job.VideoPath, job.SubtitlePath)
		for _, file := range []string{job.VideoPath, job.SubtitlePath} {
			if file != "" {
				paths = append(paths, filepath.Dir(file))
			}
		}
	}
	home = filepath.Clean(home)
	kept := paths[:0]
	for _, p := range paths {
		if p == "" || !filepath.IsAbs(p) {
			continue
		}
		p = filepath.Clean(p)
		if p == home || strings.HasPrefix(home, p+string(filepath.Separator)) || p == filepath.Dir(p) {
			continue
		}
		kept = append(kept, p)
	}
	slices.Sort(kept)
	return slices.Compact(kept)
}

// feedbackPublicEndpoint reports whether address is the public address of one of the AI service presets.
func feedbackPublicEndpoint(address string) bool {
	address = strings.TrimRight(address, "/")
	for _, preset := range settings.Presets(locales.In(feedbackIssueLanguage)) {
		if preset.BaseURL != "" && strings.TrimRight(preset.BaseURL, "/") == address {
			return true
		}
	}
	return false
}

// feedbackDetectSystem asks the system for its version off the UI thread, once the window exists: sw_vers on macOS, ver on Windows, /etc/os-release elsewhere.
func (a *app) feedbackDetectSystem() {
	update := a.update
	go func() {
		version := feedback.SystemVersion(runtime.GOOS)
		if version == "" {
			return
		}
		update(func() { a.feedback.system = feedback.SystemName(runtime.GOOS, version, runtime.GOARCH) })
	}()
}
