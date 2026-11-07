package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"kando/apps/kando/feedback"
	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo"
)

// feedbackUI is the state of the 反馈问题 dialog (feedback/): the form, whether it is open, and what the app learned for it.
type feedbackUI struct {
	open bool
	form feedback.Form
	// copied is the body 拷贝正文 copied last, so the button reads 已拷贝 until the form changes the body.
	copied string
	// sending is set while the browser is being asked to open the issue; err says why it could not.
	sending bool
	err     string
	// gen counts the dialog's openings, so a browser that answers after the dialog closed and opened again does not close the new one.
	gen int
	// release gives back the window's modal flag (holdModal) the dialog holds while it is open; feedbackHooks' afterFrame runs it once the dialog has closed.
	release func()
	// system is the system's name, version and architecture, as feedbackDetectSystem found it once the window existed.
	system string
}

func newFeedbackUI() feedbackUI {
	return feedbackUI{system: feedbackSystemName(runtime.GOOS, "", runtime.GOARCH)}
}

// feedbackOpenURL opens the issue in the user's browser. Tests replace it.
var feedbackOpenURL = openBrowser

// feedbackHooks: the dialog learns the system's version once the window exists, and gives back the window's modal flag once it has closed.
func (a *app) feedbackHooks() areaHooks {
	return areaHooks{
		ready:      a.feedbackDetectSystem,
		afterFrame: a.feedbackModal,
	}
}

// feedbackMenu is what the dialog adds to the 帮助 menu: 反馈问题…, and 反馈这部影片的字幕… while a film is open or 反馈字幕质量… otherwise, both as the prototype has them.
func (a *app) feedbackMenu() []*mygo.MenuItem {
	subs := "反馈字幕质量…"
	if a.openKey != "" {
		subs = "反馈这部影片的字幕…"
	}
	return []*mygo.MenuItem{
		{Label: "反馈问题…", Disabled: a.holds(false), Click: a.menuClick(false, func() { a.openFeedback(feedback.KindBug) })},
		{Label: subs, Disabled: a.holds(false), Click: a.menuClick(false, func() { a.openFeedback(feedback.KindSubs) })},
	}
}

// openFeedback opens the dialog afresh for kind. It attaches the app's and the tools' versions, and the open film's state when a film is open; the subtitle line stays off until the user ticks it, as it goes to a public issue. Feedback on a film's subtitles is titled after the film. A playing film pauses, as the dialog covers it.
func (a *app) openFeedback(kind feedback.Kind) {
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
	a.fb.open = true
	a.fb.form = form
	a.fb.copied, a.fb.err, a.fb.sending = "", "", false
	a.fb.gen++
	if a.fb.release == nil {
		a.fb.release = a.holdModal()
	}
}

// feedbackModal gives back the modal flag once the dialog has closed, by 取消, Escape or after the browser opened the issue.
func (a *app) feedbackModal() {
	if !a.fb.open && a.fb.release != nil {
		a.fb.release()
		a.fb.release = nil
	}
}

// feedbackFilm is the film the player shows, while one is open.
func (a *app) feedbackFilm() (films.Film, bool) {
	if a.openKey == "" {
		return films.Film{}, false
	}
	return films.Find(a.films(), a.openKey)
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
	facts := feedback.Facts{App: "看懂 " + appVersion, OS: a.fb.system}
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
	preset, _ := llmPresetByID(llmProviderID(base))
	name := preset.Name
	if model = strings.TrimSpace(model); model != "" {
		name += " · " + model
	}
	return name
}

// feedbackFilmLines is the open film's state as the issue lists it: where it comes from, the language it is translated into, how far its job got and why it failed, and how many of its subtitles have a translation. The film's link and folder are left out; feedback.NoLinks takes links out of the error, and the issue scrubs paths.
func (a *app) feedbackFilmLines(item films.Film) []string {
	lines := []string{"来源：" + films.SourceLabel(item)}
	if item.Language != "" {
		lines = append(lines, "字幕语言："+item.Language)
	}
	if job, ok := a.library.Find(item.JobID); ok {
		stage := feedbackStage(job.Stage)
		if job.Stage == core.StageFailed && job.Err != nil {
			stage += "：" + feedback.NoLinks(job.Err.Error())
		}
		lines = append(lines, "处理："+stage)
	} else {
		lines = append(lines, "处理：没有处理过")
	}
	if a.player.subs.key == item.Key && a.player.subs.ready {
		switch {
		case a.player.subs.err != "":
			lines = append(lines, "字幕："+a.player.subs.err)
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

// feedbackScrubber takes out of the issue what must not reach it: the home folder becomes ~, and every credential the app holds, the AI service's key and address, the storage's keys, buckets and endpoint, the Google account, and the values of the KANDO_ and EVERYAPI_ environment variables, become ***.
func (a *app) feedbackScrubber() feedback.Scrubber {
	home, _ := os.UserHomeDir()
	s := feedback.Scrubber{Home: home}
	tools, _ := a.pipeline.(media.Tools)
	c := a.cloud
	s.Secrets = append(s.Secrets, a.prefs.llmKey, a.prefs.llmBase, tools.Translator.APIKey, tools.Translator.BaseURL,
		c.googleEmail, c.googleRefresh,
		c.ossAccessKey, c.ossSecret, c.ossBucket, c.cosAccessKey, c.cosSecret, c.cosBucket,
		c.compatEndpoint, c.compatAccessKey, c.compatSecret, c.compatBucket)
	for _, kv := range os.Environ() {
		name, value, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "KANDO_") || strings.HasPrefix(name, "EVERYAPI_") {
			s.Secrets = append(s.Secrets, value)
		}
	}
	// A preset's public address names the service, which the issue says anyway; only an address of the user's own is a secret.
	kept := s.Secrets[:0]
	for _, secret := range s.Secrets {
		if secret = strings.TrimSpace(secret); secret != "" && !feedbackPublicEndpoint(secret) {
			kept = append(kept, secret)
		}
	}
	s.Secrets = kept
	return s
}

// feedbackPublicEndpoint reports whether address is the public address of one of the AI service presets.
func feedbackPublicEndpoint(address string) bool {
	address = strings.TrimRight(address, "/")
	for _, preset := range llmPresets() {
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
		version := feedbackSystemVersion(runtime.GOOS)
		if version == "" {
			return
		}
		update(func() { a.fb.system = feedbackSystemName(runtime.GOOS, version, runtime.GOARCH) })
	}()
}

// feedbackSystemVersion is the system's version as its own tool says it, or "" when it does not.
func feedbackSystemVersion(goos string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	switch goos {
	case "darwin":
		out, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	case "windows":
		out, err := exec.CommandContext(ctx, "cmd", "/c", "ver").Output()
		if err != nil {
			return ""
		}
		return windowsVersion(string(out))
	}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	return osRelease(string(data))
}

// windowsVersion is the number in ver's "Microsoft Windows [Version 10.0.22631.4317]".
func windowsVersion(out string) string {
	out = strings.TrimSpace(out)
	if i := strings.LastIndexByte(out, ' '); i >= 0 {
		return strings.Trim(out[i+1:], "[]")
	}
	return ""
}

// osRelease is the PRETTY_NAME of an /etc/os-release, such as Ubuntu 24.04.1 LTS.
func osRelease(data string) string {
	for line := range strings.SplitSeq(data, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "PRETTY_NAME="); ok {
			return strings.Trim(value, `"'`)
		}
	}
	return ""
}

// feedbackSystemName writes the system as the issue does: macOS 27.2 (arm64), Windows 10.0.22631 (amd64), or a Linux's own name; without a version, only the system's name.
func feedbackSystemName(goos, version, arch string) string {
	name := goos
	switch goos {
	case "darwin":
		name = "macOS"
	case "windows":
		name = "Windows"
	case "linux":
		name = "Linux"
		if version != "" {
			name, version = version, ""
		}
	}
	if version != "" {
		name += " " + version
	}
	return name + " (" + arch + ")"
}
