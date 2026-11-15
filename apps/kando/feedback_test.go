package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/feedback"
	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// feedbackFilmApp is a test app whose library holds one finished local film, talk, with two bilingual lines, the second without a translation, and its tools: yt-dlp and ffmpeg with their versions, whisper-cli missing.
func feedbackFilmApp(t *testing.T) (*app, *nsgotest.App, chan func(), string) {
	t.Helper()
	dir := t.TempDir()
	video := filepath.Join(dir, "talk", "talk.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	srt := filepath.Join(dir, "talk", "talk.bilingual.srt")
	body := "1\n00:00:01,000 --> 00:00:02,500\n你好。\nHello.\n\n2\n00:00:04,000 --> 00:00:05,000\nBye.\n\n"
	if err := os.WriteFile(srt, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tools := media.Tools{OutputDir: dir, YTDLP: "/opt/tools/yt-dlp", FFmpeg: "/opt/tools/ffmpeg", WhisperModel: "/opt/models/ggml-base.bin",
		Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}}
	a, tt, updates := newTestApp(t, tools)
	a.prefs.toolVersions = map[string]string{tools.YTDLP: "2026.09.14", tools.FFmpeg: "7.1"}
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "job1", Source: core.FileURL(video), Language: "简体中文", Stage: core.StageComplete,
		VideoPath: video, SubtitlePath: srt, LibraryDir: dir,
	}}
	refresh(tt, a)
	return a, tt, updates, dir
}

// openTalk opens the film talk on the player at at and waits for its subtitles.
func openTalk(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), at time.Duration) {
	t.Helper()
	item, ok := films.Find(a.films(), "job1")
	if !ok {
		t.Fatalf("no film talk in %v", a.films())
	}
	a.player.playFrom = at
	a.player.opening = at
	a.Open(item)
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	a.player.opening = at
	refresh(tt, a)
}

// chooseHelp chooses label in the 帮助 menu of the menu bar the app shows now, which opens the dialog afresh.
func chooseHelp(t *testing.T, a *app, tt *nsgotest.App, label string) {
	t.Helper()
	it := findMenuItem(t, a.menuBar(a.cx), "帮助", label)
	gen := a.feedback.gen
	choose(tt, a, it)
	if a.feedback.gen == gen {
		t.Fatalf("%s did not open the dialog", label)
	}
}

// feedbackPreviewText is everything the dialog shows, the preview's lines among it.
func feedbackPreviewText(tt *nsgotest.App) string { return strings.Join(tt.Texts(), "\n") }

func TestHelpMenuOpensFeedback(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	chooseHelp(t, a, tt, "反馈问题…")
	if !a.feedback.open || !hasText(tt, "反馈问题") || !hasText(tt, "在 GitHub 中打开") {
		t.Fatalf("dialog not open: %q", tt.Texts())
	}
	f := a.feedback.form
	if feedback.Kind(f.Kind) != feedback.KindBug || !f.Env || !f.Tools || f.Film || f.Subs {
		t.Fatalf("form %+v", f)
	}
	if !a.modalOpen() {
		t.Fatal("the dialog does not hold the menu bar's commands")
	}
	for _, text := range []string{"当前不在播放页，没有可附带的影片", "打开一部有字幕的影片后才能附带", "标题：（未填写）", "标签：bug", "github.com/yldm-tech/Kando/issues/new"} {
		if !strings.Contains(feedbackPreviewText(tt), text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if !tt.Find("id:feedback-title").Focused {
		t.Fatal("the title field does not have the keyboard as the dialog opens")
	}
	tt.Press("id:feedback-cancel")
	tt.Advance(time.Second)
	if a.feedback.open || a.modalOpen() || hasText(tt, "在 GitHub 中打开") {
		t.Fatalf("取消 left the dialog open %v modal %v", a.feedback.open, a.modalOpen())
	}
	if it := findMenuItem(t, a.menuBar(a.cx), "帮助", "反馈字幕质量…"); it.Disabled {
		t.Fatal("反馈字幕质量… is disabled on the library page")
	}
	chooseHelp(t, a, tt, "反馈字幕质量…")
	if feedback.Kind(a.feedback.form.Kind) != feedback.KindSubs || a.feedback.form.Title != "" {
		t.Fatalf("form %+v", a.feedback.form)
	}
	tt.Key("escape")
	if a.feedback.open || a.modalOpen() {
		t.Fatal("Escape left the dialog open")
	}
}

func TestFeedbackMenuWaitsBehindAModal(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	release := a.holdModal()
	if it := findMenuItem(t, a.menuBar(a.cx), "帮助", "反馈问题…"); !it.Disabled {
		t.Fatal("反馈问题… is enabled under a modal dialog")
	}
	release()
	if it := findMenuItem(t, a.menuBar(a.cx), "帮助", "反馈问题…"); it.Disabled {
		t.Fatal("反馈问题… stays disabled")
	}
}

func TestFeedbackPreviewListsTheRealVersions(t *testing.T) {
	a, tt, _, _ := feedbackFilmApp(t)
	a.feedback.system = feedback.SystemName("darwin", "27.2", "arm64")
	chooseHelp(t, a, tt, "反馈问题…")
	body := a.FeedbackView(a.cx).Issue.Body
	for _, want := range []string{"**类型**：问题", "看懂 " + appVersion + " · macOS 27.2 (arm64)", "yt-dlp 2026.09.14", "ffmpeg 7.1", "whisper-cli 未安装", "识别：ggml-base.bin", "翻译：EveryAPI · " + media.EveryAPIModel} {
		if !strings.Contains(body, want) {
			t.Fatalf("body misses %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{"/opt/", "api.everyapi.ai", "### 当前影片"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body has %q:\n%s", unwanted, body)
		}
	}
	tt.Press("组件与模型")
	if body := a.FeedbackView(a.cx).Issue.Body; strings.Contains(body, "yt-dlp") || !strings.Contains(body, "macOS 27.2") {
		t.Fatalf("unticking 组件与模型 left:\n%s", body)
	}
}

func TestFeedbackOnAFilmAttachesItsStateAndTheLineOnlyWhenAsked(t *testing.T) {
	a, tt, updates, dir := feedbackFilmApp(t)
	openTalk(t, a, tt, updates, 1500*time.Millisecond)
	if it := findMenuItem(t, a.menuBar(a.cx), "帮助", "反馈这部影片的字幕…"); it.Disabled {
		t.Fatal("反馈这部影片的字幕… is disabled")
	}
	chooseHelp(t, a, tt, "反馈这部影片的字幕…")
	f := a.feedback.form
	if feedback.Kind(f.Kind) != feedback.KindSubs || f.Title != "字幕问题：talk" || !f.Film || f.Subs {
		t.Fatalf("form %+v", f)
	}
	if !tt.Find("id:feedback-description").Focused {
		t.Fatal("the description field does not have the keyboard though the dialog opened titled")
	}
	body := a.FeedbackView(a.cx).Issue.Body
	for _, want := range []string{"### 当前影片", "- 来源：本地文件", "- 字幕语言：简体中文", "- 处理：已完成", "- 字幕：2 句 · 1 句没有译文"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body misses %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{dir, "file://", "talk.mp4", "### 字幕片段", "Hello."} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body has %q before the line was asked for:\n%s", unwanted, body)
		}
	}
	tt.Press("相关字幕原文与译文")
	body = a.FeedbackView(a.cx).Issue.Body
	for _, want := range []string{"### 字幕片段", "0:01.0 原文：Hello.", "0:01.0 译文：你好。"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body misses %q once the line was ticked:\n%s", want, body)
		}
	}
	if !strings.Contains(feedbackPreviewText(tt), "0:01.0 原文：Hello.") {
		t.Fatalf("the preview does not show the line: %q", tt.Texts())
	}
	// The line goes once the film closes, and its row stays unticked when a film with subtitles is open again.
	a.openKey = ""
	refresh(tt, a)
	if a.feedback.form.Subs || a.feedback.form.Film {
		t.Fatalf("the rows with nothing to attach stay ticked: %+v", a.feedback.form)
	}
}

func TestPlayerMoreMenuOpensFeedback(t *testing.T) {
	a, tt, updates, _ := feedbackFilmApp(t)
	openTalk(t, a, tt, updates, 0)
	tt.Press("更多操作")
	tt.ChoosePopup("反馈这部影片的字幕…")
	if !a.feedback.open || feedback.Kind(a.feedback.form.Kind) != feedback.KindSubs || a.feedback.form.Title != "字幕问题：talk" {
		t.Fatalf("open %v form %+v", a.feedback.open, a.feedback.form)
	}
}

func TestFeedbackSendOpensThePrefilledIssue(t *testing.T) {
	a, tt, _, _ := feedbackFilmApp(t)
	chooseHelp(t, a, tt, "反馈问题…")
	if tt.Enabled("id:feedback-send") || a.feedback.sending || len(tt.OpenedURLs) > 0 {
		t.Fatal("在 GitHub 中打开 can run without a title")
	}
	clickRow(t, tt, "标题")
	typeFocused(t, tt, "导出后字幕错位")
	clickRow(t, tt, "描述")
	typeFocused(t, tt, "字幕晚了一秒")
	tt.Press("id:feedback-send")
	tt.Advance(time.Second)
	if a.feedback.sending || len(tt.OpenedURLs) != 1 {
		t.Fatalf("sending %v, opened %q", a.feedback.sending, tt.OpenedURLs)
	}
	u, err := url.Parse(tt.OpenedURLs[0])
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "github.com" || u.Path != "/yldm-tech/Kando/issues/new" {
		t.Fatalf("opened %q", u)
	}
	q := u.Query()
	if q.Get("title") != "导出后字幕错位" || q.Get("labels") != "bug" || !strings.Contains(q.Get("body"), "字幕晚了一秒") || !strings.Contains(q.Get("body"), "看懂 "+appVersion) {
		t.Fatalf("query %v", q)
	}
	if a.feedback.open || a.modalOpen() {
		t.Fatal("the dialog stayed open after the browser opened the issue")
	}
}

func TestFeedbackKeepsTheDialogWhenTheBrowserFails(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	tt.OpenURL = func(string) bool { return false }
	chooseHelp(t, a, tt, "反馈问题…")
	a.feedback.form.Title = "崩溃"
	refresh(tt, a)
	tt.Press("id:feedback-send")
	if !a.feedback.open || a.feedback.sending || a.feedback.form.Title != "崩溃" || !strings.Contains(feedbackPreviewText(tt), "没能打开浏览器：没有应用能打开这个链接") {
		t.Fatalf("open %v sending %v texts %q", a.feedback.open, a.feedback.sending, tt.Texts())
	}
}

func TestFeedbackCopyPutsTheWholeBodyOnTheClipboard(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	chooseHelp(t, a, tt, "反馈问题…")
	a.feedback.form.Title = "长"
	a.feedback.form.Description = strings.Repeat("字幕在第二段之后全部晚了一秒，", 600)
	refresh(tt, a)
	view := a.FeedbackView(a.cx)
	if !view.Issue.Truncated || !strings.Contains(feedbackPreviewText(tt), "正文太长") {
		t.Fatalf("a long body was not cut or the dialog does not say so: %q", tt.Texts())
	}
	if len(view.Issue.URL) > feedback.MaxURL {
		t.Fatalf("link is %d bytes", len(view.Issue.URL))
	}
	tt.Press("拷贝正文")
	if tt.Clipboard != view.Issue.Full || !hasText(tt, "已拷贝") {
		t.Fatalf("clipboard has %d bytes, want the whole body (%d)", len(tt.Clipboard), len(view.Issue.Full))
	}
	a.feedback.form.Description += "再补一句"
	refresh(tt, a)
	if hasText(tt, "已拷贝") {
		t.Fatal("已拷贝 stayed after the body changed")
	}
}

// TestFeedbackHoldsTheWindowsKeysOverAFilm checks that, with the dialog open over a film, Space on a focused control of the dialog presses that control and does not play or hold the film behind it, as MyGo's dialogs held the window's shortcuts while they showed.
func TestFeedbackHoldsTheWindowsKeysOverAFilm(t *testing.T) {
	a, tt, updates, _ := feedbackFilmApp(t)
	openTalk(t, a, tt, updates, 0)
	chooseHelp(t, a, tt, "反馈问题…")
	for range 40 {
		if tt.Find("id:feedback-copy").Focused {
			break
		}
		tt.Key("tab")
	}
	if !tt.Find("id:feedback-copy").Focused {
		t.Fatal("Tab never reached 拷贝正文")
	}
	paused := a.player.play.paused
	tt.Key("space")
	if a.player.play.paused != paused {
		t.Fatal("Space under the dialog played or held the film behind it")
	}
	if tt.Clipboard != a.FeedbackView(a.cx).Issue.Full {
		t.Fatalf("Space did not press the focused 拷贝正文: clipboard %q", tt.Clipboard)
	}
}

func TestFeedbackScrubsCredentialsAndTheHomeFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("KANDO_LLM_BASE_URL", "https://llm.internal.example/v1")
	t.Setenv("EVERYAPI_API_KEY", "ea-secret-value-123")
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.prefs.llmKey = "sk-live-0123456789abcdef"
	a.cloud.ossSecret = "oss-secret-998877"
	a.cloud.googleEmail = "someone@example.com"
	chooseHelp(t, a, tt, "反馈问题…")
	a.feedback.form.Title = "key sk-live-0123456789abcdef"
	a.feedback.form.Description = "open " + filepath.Join(home, "Movies", "a.kando") + " with https://llm.internal.example/v1 and ea-secret-value-123, oss-secret-998877, someone@example.com, /Volumes/Disk/x/y"
	refresh(tt, a)
	issue := a.FeedbackView(a.cx).Issue
	for _, s := range []string{issue.Title, issue.Body, issue.Full, issue.URL, issue.Preview(testlocale.Messages()), feedbackPreviewText(tt)} {
		for _, leak := range []string{home, "sk-live", "llm.internal.example", "ea-secret", "oss-secret", "someone@example.com", "/Volumes/Disk"} {
			if strings.Contains(s, leak) {
				t.Fatalf("%q leaked into %q", leak, s)
			}
		}
	}
	if strings.Contains(issue.Body, "a.kando") || !strings.Contains(issue.Body, "open <路径> with") {
		t.Fatalf("the path under the home folder is not hidden: %q", issue.Body)
	}
}

// TestFeedbackScrubsEveryStoredCredential sets every value of the account's part of settings.json (savedCloud) as the saved settings do, and checks that none of them reaches the issue, so the storage a later change adds is scrubbed too. The values are bare hex, as R2's keys are, which no token shape catches. Only the storage's kind and the regions are not secrets.
func TestFeedbackScrubsEveryStoredCredential(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	var saved savedSettings
	v := reflect.ValueOf(&saved.savedCloud).Elem()
	var secrets []string
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		if v.Field(i).Kind() != reflect.String || name == "Storage" || strings.HasSuffix(name, "Region") {
			continue
		}
		t.Setenv("KANDO_"+strings.ToUpper(name), "")
		value := fmt.Sprintf("9f8e7d6c5b4a3928%016x", i+1)
		v.Field(i).SetString(value)
		secrets = append(secrets, value)
	}
	a.cloudHooks().restore(saved)
	chooseHelp(t, a, tt, "反馈问题…")
	a.feedback.form.Title = "upload fails with " + secrets[0]
	a.feedback.form.Description = "my settings: " + strings.Join(secrets, " ")
	refresh(tt, a)
	issue := a.FeedbackView(a.cx).Issue
	for _, s := range []string{issue.Title, issue.Body, issue.Full, issue.URL, issue.Preview(testlocale.Messages()), feedbackPreviewText(tt)} {
		for _, secret := range secrets {
			if strings.Contains(s, secret) {
				t.Fatalf("%q leaked into %q", secret, s)
			}
		}
	}
}

// TestFeedbackHidesTheFilmsPaths checks that the open film's state carries no path, not even one with spaces and the video's title in it from a tool's message, and that a library the app knows is hidden whole wherever it is written, though its name has a comma and a space.
func TestFeedbackHidesTheFilmsPaths(t *testing.T) {
	a, tt, updates, dir := feedbackFilmApp(t)
	other := filepath.Join(t.TempDir(), "私人，家庭 影片.kando")
	a.edit(func(s *session) { s.Libraries = append(s.Libraries, other) })
	openTalk(t, a, tt, updates, 1500*time.Millisecond)
	a.library.Jobs[0].Stage = core.StageFailed
	a.library.Jobs[0].Err = errors.New("download: yt-dlp: [download] Destination: " + filepath.Join(dir, "talk", "My Private Title：第二部 [abc].mp4") + "\nERROR: unable to write the file")
	refresh(tt, a)
	chooseHelp(t, a, tt, "反馈这部影片的字幕…")
	a.feedback.form.Description = "另一个资料库 " + filepath.Join(other, "videos", "a b.mp4") + " 也打不开"
	refresh(tt, a)
	issue := a.FeedbackView(a.cx).Issue
	for _, s := range []string{issue.Title, issue.Body, issue.URL, issue.Preview(testlocale.Messages())} {
		for _, leak := range []string{dir, "My Private", "第二部", "私人", "家庭", "a b.mp4"} {
			if strings.Contains(s, leak) {
				t.Fatalf("%q leaked into %q", leak, s)
			}
		}
	}
	for _, want := range []string{"- 处理：失败：download: yt-dlp: [download] Destination: <路径>\nERROR: unable to write the file", "另一个资料库 <路径> 也打不开"} {
		if !strings.Contains(issue.Body, want) {
			t.Fatalf("body misses %q:\n%s", want, issue.Body)
		}
	}
}

// TestFeedbackHidesTheSubtitlesBesideAVideoWithoutAJob checks that the bilingual .srt the scan found beside a video no job has processed is among the paths an issue hides, as a job's subtitles are.
func TestFeedbackHidesTheSubtitlesBesideAVideoWithoutAJob(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{"interview.mp4": "video", "interview.bilingual.srt": "1\n00:00:00,500 --> 00:00:01,000\n早上好\nGood morning\n\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.refreshLibrary()
	want := filepath.Join(dir, "interview", "video.bilingual.srt")
	if paths := a.feedbackPaths(t.TempDir()); !slices.Contains(paths, want) {
		t.Fatalf("feedback paths %q miss %q", paths, want)
	}
}

// TestFeedbackItemsAreMenuOnlyRows checks that the 反馈 items are rows of the shortcut table without a key: the 帮助 menu lists them after a separator and without an accelerator, and neither the cheat sheet nor the 快捷键 page lists them, as they have no key to show.
func TestFeedbackItemsAreMenuOnlyRows(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	var labels []string
	for _, s := range a.feedbackKeys(a.cx) {
		if s.Key != "" || s.Group != groupHelp {
			t.Fatalf("feedback row %+v", s)
		}
		labels = append(labels, s.Label)
	}
	if !slices.Equal(labels, []string{"反馈问题…", "反馈字幕质量…"}) {
		t.Fatalf("rows %q", labels)
	}
	for _, m := range a.menuBar(a.cx) {
		if m.Label != "帮助" {
			continue
		}
		var got []string
		for _, it := range m.Items {
			if it.Separator {
				got = append(got, "-")
				continue
			}
			if it.Shortcut != "" && it.Label != "快捷键速查" {
				t.Fatalf("%q has shortcut %q", it.Label, it.Shortcut)
			}
			got = append(got, it.Label)
		}
		if !slices.Equal(got, []string{"快捷键速查", "重新查看入门引导", "-", "看懂官网", "更新说明", "反馈问题…", "反馈字幕质量…", "支持的网站…", "显示日志文件"}) {
			t.Fatalf("帮助 menu %q", got)
		}
	}
	for _, g := range a.cheatsheetGroups(a.cx) {
		for _, k := range g.Keys {
			if slices.Contains(labels, k.Label) {
				t.Fatalf("the cheat sheet lists %q", k.Label)
			}
		}
	}
	for _, s := range a.Shortcuts(a.cx) {
		if slices.Contains(labels, s.Label) {
			t.Fatalf("the 快捷键 page lists %q", s.Label)
		}
	}
}

func TestFeedbackCuePicksTheLineAtThePlayhead(t *testing.T) {
	cues := []core.Segment{{Start: time.Second, End: 2 * time.Second, Text: "a"}, {Start: 4 * time.Second, End: 5 * time.Second, Text: "b"}}
	for at, want := range map[time.Duration]string{0: "a", 1500 * time.Millisecond: "a", 3 * time.Second: "a", 4500 * time.Millisecond: "b", time.Minute: "b"} {
		if cue, ok := feedbackCue(cues, at); !ok || cue.Text != want {
			t.Errorf("at %v: %q %v, want %q", at, cue.Text, ok, want)
		}
	}
	if _, ok := feedbackCue(nil, 0); ok {
		t.Error("a line without subtitles")
	}
}

// TestFeedbackCheckboxTogglesOnce checks that a click on an attachment's check box toggles it once, though the row around it toggles on a click too.
func TestFeedbackCheckboxTogglesOnce(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	chooseHelp(t, a, tt, "反馈问题…")
	row := tt.Find("id:feedback-env")
	// The check box sits at the row's start, centered on its two lines.
	tt.PressAt("id:feedback-env", ns.Point{X: 20, Y: row.Frame.Size.Height / 2})
	if a.feedback.form.Env {
		t.Fatal("a click on the check box left 应用与系统版本 ticked")
	}
	tt.Press("应用与系统版本")
	if !a.feedback.form.Env {
		t.Fatal("a click on the row did not tick 应用与系统版本 again")
	}
}

// TestFeedbackDialogInEnglish reads the dialog and its menu items in English, while the issue it builds stays in Chinese for the maintainers.
func TestFeedbackDialogInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, tt, _, _ := feedbackFilmApp(t)
	if got := []string{a.feedbackKeys(a.cx)[0].Label, a.feedbackKeys(a.cx)[1].Label}; !slices.Equal(got, []string{"Send Feedback…", "Send Subtitle Feedback…"}) {
		t.Fatalf("menu items %q", got)
	}
	a.openFeedback(feedback.KindBug)
	refresh(tt, a)
	for _, want := range []string{"Send Feedback", "Problem", "Suggestion", "Subtitle Quality", "Attached Information", "App and System Versions", "Components and Models", "whisper-cli not installed", "Not in the player, so there's no film to attach", "Issue Preview", "Title: (empty)", "Label: bug", "Cancel", "Copy Text", "Open in GitHub"} {
		wantText(t, tt, want)
	}
	for _, unwanted := range []string{"在 GitHub 中打开", "附带信息", "组件与模型", "标题：（未填写）"} {
		if textCount(tt, unwanted) > 0 {
			t.Fatalf("the dialog still reads %q", unwanted)
		}
	}
	body := a.FeedbackView(a.cx).Issue.Body
	for _, want := range []string{"**类型**：问题", "### 描述", "whisper-cli 未安装"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the issue misses %q:\n%s", want, body)
		}
	}
	a.feedback.failed = true
	if got, want := a.FeedbackView(a.cx).Err, "Couldn't open the browser: no app opened the link. Use “Copy Text” to copy the issue and paste it at github.com/yldm-tech/Kando/issues/new."; got != want {
		t.Fatalf("error %q, want %q", got, want)
	}
}
