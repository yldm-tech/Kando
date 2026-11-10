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
	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// feedbackBrowser replaces the browser for t and returns the links it was asked to open, failing with err when err is not nil.
func feedbackBrowser(t *testing.T, err error) *[]string {
	t.Helper()
	var opened []string
	prev := feedbackOpenURL
	feedbackOpenURL = func(link string) error {
		opened = append(opened, link)
		return err
	}
	t.Cleanup(func() { feedbackOpenURL = prev })
	return &opened
}

// feedbackFilmApp is a test app whose library holds one finished local film, talk, with two bilingual lines, the second without a translation, and its tools: yt-dlp and ffmpeg with their versions, whisper-cli missing.
func feedbackFilmApp(t *testing.T) (*app, *ui.Tester, chan func(), string) {
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
	a, tt, updates := newTestApp(tools)
	a.prefs.toolVersions = map[string]string{tools.YTDLP: "2026.09.14", tools.FFmpeg: "7.1"}
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "job1", Source: (&url.URL{Scheme: "file", Path: video}).String(), Language: "简体中文", Stage: core.StageComplete,
		VideoPath: video, SubtitlePath: srt, LibraryDir: dir,
	}}
	tt.Frame()
	return a, tt, updates, dir
}

// openTalk opens the film talk on the player at at and waits for its subtitles.
func openTalk(t *testing.T, a *app, tt *ui.Tester, updates chan func(), at time.Duration) {
	t.Helper()
	a.player.playFrom = at
	a.player.opening = at
	if err := tt.Click("talk"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	waitUntil(t, updates, func() bool { return a.player.subs.ready })
	a.player.opening = at
	tt.Frame()
}

// chooseHelp chooses label in the 帮助 menu of the menu bar the app shows now.
func chooseHelp(t *testing.T, a *app, tt *ui.Tester, updates chan func(), label string) {
	t.Helper()
	it := findMenuItem(t, a.menuBar(), "帮助", label)
	gen := a.feedback.gen
	it.Click(it, nil)
	waitUntil(t, updates, func() bool { return a.feedback.gen != gen })
	tt.Frame()
}

func TestHelpMenuOpensFeedback(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	tt.Frame()
	chooseHelp(t, a, tt, updates, "反馈问题…")
	if !a.feedback.open || !tt.HasText("反馈问题") || !tt.HasText("在 GitHub 中打开") {
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
		if !tt.HasText(text) && !strings.Contains(strings.Join(tt.Texts(), "\n"), text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.feedback.open || a.modalOpen() || tt.HasText("在 GitHub 中打开") {
		t.Fatalf("取消 left the dialog open %v modal %v", a.feedback.open, a.modalOpen())
	}
	if it := findMenuItem(t, a.menuBar(), "帮助", "反馈字幕质量…"); it.Disabled {
		t.Fatal("反馈字幕质量… is disabled on the library page")
	}
	chooseHelp(t, a, tt, updates, "反馈字幕质量…")
	if feedback.Kind(a.feedback.form.Kind) != feedback.KindSubs || a.feedback.form.Title != "" {
		t.Fatalf("form %+v", a.feedback.form)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Frame()
	if a.feedback.open || a.modalOpen() {
		t.Fatal("Escape left the dialog open")
	}
}

func TestFeedbackMenuWaitsBehindAModal(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	release := a.holdModal()
	if it := findMenuItem(t, a.menuBar(), "帮助", "反馈问题…"); !it.Disabled {
		t.Fatal("反馈问题… is enabled under a modal dialog")
	}
	release()
	if it := findMenuItem(t, a.menuBar(), "帮助", "反馈问题…"); it.Disabled {
		t.Fatal("反馈问题… stays disabled")
	}
}

func TestFeedbackPreviewListsTheRealVersions(t *testing.T) {
	a, tt, updates, _ := feedbackFilmApp(t)
	a.feedback.system = feedbackSystemName("darwin", "27.2", "arm64")
	chooseHelp(t, a, tt, updates, "反馈问题…")
	body := a.FeedbackView().Issue.Body
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
	if err := tt.Click("组件与模型"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if body := a.FeedbackView().Issue.Body; strings.Contains(body, "yt-dlp") || !strings.Contains(body, "macOS 27.2") {
		t.Fatalf("unticking 组件与模型 left:\n%s", body)
	}
}

func TestFeedbackOnAFilmAttachesItsStateAndTheLineOnlyWhenAsked(t *testing.T) {
	a, tt, updates, dir := feedbackFilmApp(t)
	openTalk(t, a, tt, updates, 1500*time.Millisecond)
	if it := findMenuItem(t, a.menuBar(), "帮助", "反馈这部影片的字幕…"); it.Disabled {
		t.Fatal("反馈这部影片的字幕… is disabled")
	}
	chooseHelp(t, a, tt, updates, "反馈这部影片的字幕…")
	f := a.feedback.form
	if feedback.Kind(f.Kind) != feedback.KindSubs || f.Title != "字幕问题：talk" || !f.Film || f.Subs {
		t.Fatalf("form %+v", f)
	}
	body := a.FeedbackView().Issue.Body
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
	if err := tt.Click("相关字幕原文与译文"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	body = a.FeedbackView().Issue.Body
	for _, want := range []string{"### 字幕片段", "0:01.0 原文：Hello.", "0:01.0 译文：你好。"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body misses %q once the line was ticked:\n%s", want, body)
		}
	}
	if !strings.Contains(strings.Join(tt.Texts(), "\n"), "0:01.0 原文：Hello.") {
		t.Fatalf("the preview does not show the line: %q", tt.Texts())
	}
}

func TestPlayerMoreMenuOpensFeedback(t *testing.T) {
	a, tt, updates, _ := feedbackFilmApp(t)
	openTalk(t, a, tt, updates, 0)
	if err := tt.Click("更多操作"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	if err := tt.ChooseMenuItem("反馈这部影片的字幕…"); err != nil {
		t.Fatalf("%v menu %q", err, tt.Menu())
	}
	tt.Frame()
	if !a.feedback.open || feedback.Kind(a.feedback.form.Kind) != feedback.KindSubs || a.feedback.form.Title != "字幕问题：talk" {
		t.Fatalf("open %v form %+v", a.feedback.open, a.feedback.form)
	}
}

func TestFeedbackSendOpensThePrefilledIssue(t *testing.T) {
	opened := feedbackBrowser(t, nil)
	a, tt, updates, _ := feedbackFilmApp(t)
	chooseHelp(t, a, tt, updates, "反馈问题…")
	if err := tt.Click("在 GitHub 中打开"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.feedback.sending || len(*opened) > 0 {
		t.Fatal("在 GitHub 中打开 ran without a title")
	}
	clickRow(t, tt, "标题")
	tt.Type("导出后字幕错位")
	tt.Frame()
	clickRow(t, tt, "描述")
	tt.Type("字幕晚了一秒")
	tt.Frame()
	if err := tt.Click("在 GitHub 中打开"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, updates, func() bool { return !a.feedback.sending })
	tt.Frame()
	tt.Frame()
	if len(*opened) != 1 {
		t.Fatalf("opened %q", *opened)
	}
	u, err := url.Parse((*opened)[0])
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
	feedbackBrowser(t, errors.New("no browser"))
	a, tt, updates := newTestApp(&fakePipeline{})
	tt.Frame()
	chooseHelp(t, a, tt, updates, "反馈问题…")
	a.feedback.form.Title = "崩溃"
	tt.Frame()
	if err := tt.Click("在 GitHub 中打开"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, updates, func() bool { return !a.feedback.sending })
	tt.Frame()
	if !a.feedback.open || a.feedback.form.Title != "崩溃" || !strings.Contains(strings.Join(tt.Texts(), "\n"), "没能打开浏览器：no browser") {
		t.Fatalf("open %v texts %q", a.feedback.open, tt.Texts())
	}
}

func TestFeedbackCopyPutsTheWholeBodyOnTheClipboard(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	tt.Frame()
	chooseHelp(t, a, tt, updates, "反馈问题…")
	a.feedback.form.Title = "长"
	a.feedback.form.Description = strings.Repeat("字幕在第二段之后全部晚了一秒，", 600)
	tt.Frame()
	view := a.FeedbackView()
	if !view.Issue.Truncated || !strings.Contains(strings.Join(tt.Texts(), "\n"), "正文太长") {
		t.Fatalf("a long body was not cut or the dialog does not say so: %q", tt.Texts())
	}
	if len(view.Issue.URL) > feedback.MaxURL {
		t.Fatalf("link is %d bytes", len(view.Issue.URL))
	}
	if err := tt.Click("拷贝正文"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.Clipboard() != view.Issue.Full || !tt.HasText("已拷贝") {
		t.Fatalf("clipboard has %d bytes, want the whole body (%d)", len(tt.Clipboard()), len(view.Issue.Full))
	}
	a.feedback.form.Description += "再补一句"
	tt.Frame()
	if tt.HasText("已拷贝") {
		t.Fatal("已拷贝 stayed after the body changed")
	}
}

func TestFeedbackScrubsCredentialsAndTheHomeFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("KANDO_LLM_BASE_URL", "https://llm.internal.example/v1")
	t.Setenv("EVERYAPI_API_KEY", "ea-secret-value-123")
	a, tt, updates := newTestApp(&fakePipeline{})
	a.prefs.llmKey = "sk-live-0123456789abcdef"
	a.cloud.ossSecret = "oss-secret-998877"
	a.cloud.googleEmail = "someone@example.com"
	tt.Frame()
	chooseHelp(t, a, tt, updates, "反馈问题…")
	a.feedback.form.Title = "key sk-live-0123456789abcdef"
	a.feedback.form.Description = "open " + filepath.Join(home, "Movies", "a.kando") + " with https://llm.internal.example/v1 and ea-secret-value-123, oss-secret-998877, someone@example.com, /Volumes/Disk/x/y"
	tt.Frame()
	issue := a.FeedbackView().Issue
	for _, s := range []string{issue.Title, issue.Body, issue.Full, issue.URL, issue.Preview(), strings.Join(tt.Texts(), "\n")} {
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
	a, tt, updates := newTestApp(&fakePipeline{})
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
	tt.Frame()
	chooseHelp(t, a, tt, updates, "反馈问题…")
	a.feedback.form.Title = "upload fails with " + secrets[0]
	a.feedback.form.Description = "my settings: " + strings.Join(secrets, " ")
	tt.Frame()
	issue := a.FeedbackView().Issue
	for _, s := range []string{issue.Title, issue.Body, issue.Full, issue.URL, issue.Preview(), strings.Join(tt.Texts(), "\n")} {
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
	tt.Frame()
	chooseHelp(t, a, tt, updates, "反馈这部影片的字幕…")
	a.feedback.form.Description = "另一个资料库 " + filepath.Join(other, "videos", "a b.mp4") + " 也打不开"
	tt.Frame()
	issue := a.FeedbackView().Issue
	for _, s := range []string{issue.Title, issue.Body, issue.URL, issue.Preview()} {
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
	a, _, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.refreshLibrary()
	want := filepath.Join(dir, "interview", "video.bilingual.srt")
	if paths := a.feedbackPaths(t.TempDir()); !slices.Contains(paths, want) {
		t.Fatalf("feedback paths %q miss %q", paths, want)
	}
}

// TestFeedbackItemsAreMenuOnlyRows checks that the 反馈 items are rows of the shortcut table without a key: the 帮助 menu lists them after a separator and without an accelerator, and neither the cheat sheet nor the 快捷键 page lists them, as they have no key to show.
func TestFeedbackItemsAreMenuOnlyRows(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	var labels []string
	for _, s := range a.feedbackKeys() {
		if s.Key != 0 || s.Mods != 0 || s.Group != groupHelp {
			t.Fatalf("feedback row %+v", s)
		}
		labels = append(labels, s.Label)
	}
	if !slices.Equal(labels, []string{"反馈问题…", "反馈字幕质量…"}) {
		t.Fatalf("rows %q", labels)
	}
	for _, m := range a.menuBar() {
		if m.Label != "帮助" {
			continue
		}
		var got []string
		for _, it := range m.Submenu {
			if it.Type == mygo.MenuItemSeparator {
				got = append(got, "-")
				continue
			}
			if it.Accelerator != "" && it.Label != "快捷键速查" {
				t.Fatalf("%q has accelerator %q", it.Label, it.Accelerator)
			}
			got = append(got, it.Label)
		}
		if !slices.Equal(got, []string{"快捷键速查", "重新查看入门引导", "-", "反馈问题…", "反馈字幕质量…", "支持的网站…"}) {
			t.Fatalf("帮助 menu %q", got)
		}
	}
	for _, g := range a.cheatsheetGroups() {
		for _, k := range g.Keys {
			if slices.Contains(labels, k.Label) {
				t.Fatalf("the cheat sheet lists %q", k.Label)
			}
		}
	}
	for _, s := range a.Shortcuts() {
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

func TestFeedbackSystemNames(t *testing.T) {
	cases := []struct{ goos, version, arch, want string }{
		{"darwin", "27.2", "arm64", "macOS 27.2 (arm64)"},
		{"darwin", "", "arm64", "macOS (arm64)"},
		{"windows", windowsVersion("\r\nMicrosoft Windows [Version 10.0.22631.4317]\r\n"), "amd64", "Windows 10.0.22631.4317 (amd64)"},
		{"linux", osRelease("NAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nID=ubuntu\n"), "amd64", "Ubuntu 24.04.1 LTS (amd64)"},
		{"linux", "", "arm64", "Linux (arm64)"},
	}
	for _, c := range cases {
		if got := feedbackSystemName(c.goos, c.version, c.arch); got != c.want {
			t.Errorf("%s %q: %q, want %q", c.goos, c.version, got, c.want)
		}
	}
}

// TestFeedbackCheckboxTogglesOnce checks that a click on an attachment's check box toggles it once, though the row around it toggles on a click too.
func TestFeedbackCheckboxTogglesOnce(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	tt.Frame()
	chooseHelp(t, a, tt, updates, "反馈问题…")
	row, ok := tt.Find("应用与系统版本")
	if !ok {
		t.Fatalf("no row: %q", tt.Texts())
	}
	// The check box sits at the row's start, centered on its two lines.
	tt.ClickAt(row.X+20, row.Y+row.H/2)
	tt.Frame()
	if a.feedback.form.Env {
		t.Fatal("a click on the check box left 应用与系统版本 ticked")
	}
	if err := tt.Click("应用与系统版本"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.feedback.form.Env {
		t.Fatal("a click on the row did not tick 应用与系统版本 again")
	}
}

// TestFeedbackDialogInEnglish reads the dialog and its menu items in English, while the issue it builds stays in Chinese for the maintainers.
func TestFeedbackDialogInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, tt, _, _ := feedbackFilmApp(t)
	if got := []string{a.feedbackKeys()[0].Label, a.feedbackKeys()[1].Label}; !slices.Equal(got, []string{"Send Feedback…", "Send Subtitle Feedback…"}) {
		t.Fatalf("menu items %q", got)
	}
	a.openFeedback(feedback.KindBug)
	tt.Frame()
	for _, want := range []string{"Send Feedback", "Problem", "Suggestion", "Subtitle Quality", "Attached Information", "App and System Versions", "Components and Models", "whisper-cli not installed", "Not in the player, so there's no film to attach", "Issue Preview", "Title: (empty)", "Label: bug", "Cancel", "Copy Text", "Open in GitHub"} {
		wantText(t, tt, want)
	}
	for _, unwanted := range []string{"在 GitHub 中打开", "附带信息", "组件与模型", "标题：（未填写）"} {
		if textCount(tt, unwanted) > 0 {
			t.Fatalf("the dialog still reads %q", unwanted)
		}
	}
	body := a.FeedbackView().Issue.Body
	for _, want := range []string{"**类型**：问题", "### 描述", "whisper-cli 未安装"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the issue misses %q:\n%s", want, body)
		}
	}
	a.feedback.err = errors.New("no browser")
	if got, want := a.FeedbackView().Err, "Couldn't open the browser: no browser. Use “Copy Text” to copy the issue and paste it at github.com/yldm-tech/Kando/issues/new."; got != want {
		t.Fatalf("error %q, want %q", got, want)
	}
}
