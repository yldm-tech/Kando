package main

import (
	"context"
	"errors"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"lingocut/apps/lingocut/film"
	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/shelf"
	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// fakePipeline finishes every stage at once unless block is set, in which case Download waits for cancellation.
type fakePipeline struct {
	block    bool
	language string
}

func (f *fakePipeline) Check() error { return nil }

func (f *fakePipeline) Download(ctx context.Context, _ string) (string, error) {
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return "/videos/talk.mp4", nil
}

func (f *fakePipeline) Transcribe(context.Context, string) ([]core.Segment, error) {
	return []core.Segment{{End: time.Second, Text: "hello"}}, nil
}

func (f *fakePipeline) Translate(_ context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	f.language = language
	return segments, nil
}

func (f *fakePipeline) Export(context.Context, string, []core.Segment, []core.Segment) (string, error) {
	return "/videos/talk.bilingual.srt", nil
}

// newTestApp queues background updates so the test applies them on its own goroutine, as the window's UI thread would.
func newTestApp(p media.Pipeline) (*app, *ui.Tester, chan func()) {
	updates := make(chan func(), 16)
	a := newApp(p, func(fn func()) { updates <- fn })
	a.edit(func(s *session) { s.LibraryDir = "/videos" })
	return a, ui.NewTester(a.view, 1100, 720), updates
}

func settle(t *testing.T, a *app, tt *ui.Tester, updates chan func()) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for a.running() {
		select {
		case fn := <-updates:
			fn()
		case <-timeout:
			t.Fatalf("job still running, status %q", a.status)
		}
	}
	tt.Frame()
}

func startJob(t *testing.T, tt *ui.Tester, source string) {
	t.Helper()
	if err := tt.Click("视频链接"); err != nil {
		t.Fatal(err)
	}
	tt.Type(source)
	if err := tt.Click("开始处理"); err != nil {
		t.Fatal(err)
	}
}

func TestViewRunsJobToCompletion(t *testing.T) {
	p := &fakePipeline{}
	a, tt, updates := newTestApp(p)
	startJob(t, tt, "https://example.com/video")
	if a.job == nil || a.status != "任务已加入队列" {
		t.Fatalf("texts %q after starting", tt.Texts())
	}

	settle(t, a, tt, updates)
	if !tt.HasText("已完成") || !tt.HasText("talk") {
		t.Fatalf("texts %q after the job", tt.Texts())
	}
	if p.language != "简体中文" {
		t.Fatalf("translated into %q", p.language)
	}
	if !tt.HasText("开始处理") {
		t.Fatal("start button did not come back")
	}
}

func TestViewCancelsRunningJob(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{block: true})
	startJob(t, tt, "https://example.com/video")
	tt.Frame()
	if err := tt.Click("取消"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}

	settle(t, a, tt, updates)
	if !tt.HasText("已取消") || !tt.HasText("开始处理") {
		t.Fatalf("texts %q after cancelling", tt.Texts())
	}
}

func TestRetryKeepsTheSameCard(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	a.library.Jobs = []core.Job{{
		ID: "same", Source: "https://www.youtube.com/watch?v=TRWgBxVAb1M", Language: "简体中文",
		Stage: core.StageFailed, VideoPath: "/videos/Colibri [TRWgBxVAb1M].mp4",
		Err: errors.New("translate: translation API returned 401"),
	}}
	tt.Frame()
	if err := tt.Click("重试"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].ID != "same" || a.library.Jobs[0].Stage == core.StageFailed {
		t.Fatalf("right after retry: %#v", a.library.Jobs)
	}

	settle(t, a, tt, updates)
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].ID != "same" || a.library.Jobs[0].Stage != core.StageComplete {
		t.Fatalf("after retry: %#v", a.library.Jobs)
	}
	if tt.HasText("失败") {
		t.Fatalf("failed card stayed: %q", tt.Texts())
	}
}

func TestLibraryShowsAndRestoresJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, tt, updates := newTestApp(&fakePipeline{})
	a.openLibrary(path)
	startJob(t, tt, "https://example.com/video")
	settle(t, a, tt, updates)
	if !tt.HasText("talk") || !tt.HasText("已完成") {
		t.Fatalf("texts %q after the job", tt.Texts())
	}
	if !tt.HasText("1 部影片") || !tt.HasText("1") {
		t.Fatalf("texts %q missing the count", tt.Texts())
	}

	reloaded, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Jobs) != 1 || reloaded.Jobs[0].SubtitlePath != "/videos/talk.bilingual.srt" || reloaded.Jobs[0].Language != "简体中文" {
		t.Fatalf("saved %#v", reloaded.Jobs)
	}
}

func TestItemVideoUsesTheFolderName(t *testing.T) {
	got := jobName(core.Job{VideoPath: "/Movies/影视库.lingocut/Learn Rust [abc]/video.mp4"})
	if got != "Learn Rust [abc]" {
		t.Fatalf("name = %q", got)
	}
}

func TestYouTubeJobNameUsesTheVideoID(t *testing.T) {
	got := jobName(core.Job{Source: "https://www.youtube.com/watch?v=TRWgBxVAb1M"})
	if got != "YouTube · TRWgBxVAb1M" {
		t.Fatalf("name = %q", got)
	}
}

func TestFilmColumnsFollowTheWindow(t *testing.T) {
	if films.Columns(900) != 2 || films.Columns(1100) != 2 || films.Columns(1680) != 4 || films.Columns(700) != 1 {
		t.Fatalf("columns 900=%d 1100=%d 1680=%d 700=%d", films.Columns(900), films.Columns(1100), films.Columns(1680), films.Columns(700))
	}
}

func TestFilmGridReflows(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one.mp4", "two.mp4", "three.mp4"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := newApp(&fakePipeline{}, func(fn func()) {})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.refreshLibrary()

	narrow := ui.NewTester(a.view, 900, 720)
	narrow.Frame()
	if got := filmRows(t, narrow); got != 2 {
		t.Fatalf("narrow rows = %d", got)
	}

	wide := ui.NewTester(a.view, 1680, 720)
	wide.Frame()
	if got := filmRows(t, wide); got != 1 {
		t.Fatalf("wide rows = %d", got)
	}
}

func filmRows(t *testing.T, tt *ui.Tester) int {
	t.Helper()
	tops := []float32{filmTop(t, tt, "one"), filmTop(t, tt, "two"), filmTop(t, tt, "three")}
	slices.Sort(tops)
	rows := 1
	for i := 1; i < len(tops); i++ {
		if tops[i]-tops[i-1] > 4 {
			rows++
		}
	}
	return rows
}

func filmTop(t *testing.T, tt *ui.Tester, title string) float32 {
	t.Helper()
	r, ok := tt.Find(title)
	if !ok {
		t.Fatalf("missing %q in %q", title, tt.Texts())
	}
	return r.Y
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func TestTitleBarClearsTheTrafficLights(t *testing.T) {
	_, tt, _ := newTestApp(&fakePipeline{})
	tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 70})
	title, ok := tt.Find("videos")
	if !ok {
		t.Fatalf("texts %q", tt.Texts())
	}
	if title.Y < 28 {
		t.Fatalf("title sits under the traffic lights at y=%v", title.Y)
	}

	_, plain, _ := newTestApp(&fakePipeline{})
	plain.Frame()
	title, ok = plain.Find("videos")
	if !ok || title.Y >= 28 {
		t.Fatalf("title y=%v without a title bar", title.Y)
	}
}

func TestViewShowsDownloadedVideosInLibrary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "interview.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.refreshLibrary()
	tt.Frame()
	if !tt.HasText("interview") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestDropImportsLocalVideo(t *testing.T) {
	src := filepath.Join(t.TempDir(), "interview.mp4")
	if err := os.WriteFile(src, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	r, ok := tt.Find("开始处理")
	if !ok {
		t.Fatal("missing start button")
	}
	if !tt.DropFiles(r.X+r.W/2, r.Y+r.H/2, src) {
		t.Fatal("the library did not take the dropped file")
	}
	tt.Frame()
	if a.source != "file://"+src || a.status != "已选择本地视频" {
		t.Fatalf("source %q status %q texts %q", a.source, a.status, tt.Texts())
	}
}

func TestImportIgnoresNonVideo(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(src, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.importLocal(src)
	if a.status != "没有可导入的视频" {
		t.Fatalf("status %q", a.status)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("non-video was copied")
	}
}

func TestViewRejectsInvalidSource(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	startJob(t, tt, "not-a-url")
	tt.Frame()
	if a.job != nil || !tt.HasText("video source must be a valid URL") {
		t.Fatalf("texts %q after an invalid source", tt.Texts())
	}
}

func TestShortVersion(t *testing.T) {
	if got := shortVersion("ffmpeg version 9.0.2 Copyright"); got != "9.0.2" {
		t.Fatalf("ffmpeg = %q", got)
	}
	if got := shortVersion("whisper.cpp version: 1.9.5"); got != "1.9.5" {
		t.Fatalf("whisper = %q", got)
	}
	if got := shortVersion("2026.08.19"); got != "2026.08.19" {
		t.Fatalf("yt-dlp = %q", got)
	}
}

func TestSettingsPages(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = filepath.Join(home, "Movies", "LingoCut") })
	tt.Frame()
	if err := tt.Click("通用"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, text := range []string{"资料库", "当前资料库", "~/Movies/LingoCut", "切换", "界面语言", "简体中文", "当前版本", appVersion, "组件", "yt-dlp", "FFmpeg", "whisper-cli", "未安装"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if tt.HasText("新建资料库") || tt.HasText("移出") {
		t.Fatalf("library actions are in the menu, texts %q", tt.Texts())
	}
	if err := tt.Click("切换"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"新建资料库...", "打开资料库...", "在 Finder 中显示"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if tt.HasText("重新检测") {
		t.Fatal("redetect is only for a real tool pipeline")
	}
	if err := tt.Click("语音识别"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, text := range []string{"Whisper", "Tiny", "Large v3 Turbo（压缩版）", "推荐", "下载"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("AI 服务"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("未配置") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("翻译"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, text := range []string{"默认翻译为", "简体中文", "智能翻译", "提示词", "正在使用默认提示词", "恢复默认", "{{language}}"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestMissingLibraryFollowsTheRenamedBundle(t *testing.T) {
	parent := t.TempDir()
	current := filepath.Join(parent, "影视库.lingocut")
	old := filepath.Join(parent, "影视库 2.lingocut")
	renamed := filepath.Join(parent, "missav.lingocut")
	if err := os.Mkdir(current, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(renamed, 0o755); err != nil {
		t.Fatal(err)
	}
	a := newApp(&fakePipeline{}, func(func()) {})
	a.edit(func(s *session) {
		s.LibraryDir = current
		s.Libraries = []string{current, old}
	})
	if !a.reconcileMissingLibraries() {
		t.Fatal("the renamed bundle was not picked up")
	}
	got := a.knownLibraries()
	if len(got) != 2 || !containsPath(got, renamed) || containsPath(got, old) {
		t.Fatalf("libraries = %v", got)
	}
	if a.snap().LibraryDir != current {
		t.Fatalf("open library = %q", a.snap().LibraryDir)
	}
}

func TestRenameMovesTheInodeWithTheLibrary(t *testing.T) {
	parent := t.TempDir()
	old := filepath.Join(parent, "影视库.lingocut")
	next := filepath.Join(parent, "missav.lingocut")
	a := newApp(&fakePipeline{}, func(func()) {})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) {
		s.LibraryDir = old
		s.Libraries = []string{old}
	})
	a.rememberID(old, media.MakeDirID(7, 99))
	a.renameLibraryPath(old, next)
	if _, ok := a.snap().Inodes[old]; ok {
		t.Fatal("old path kept its inode")
	}
	dev, ino, ok := a.inode(next).Parts()
	if !ok || dev != 7 || ino != 99 {
		t.Fatalf("inode = %d %d ok=%v", dev, ino, ok)
	}
	if a.snap().LibraryDir != next {
		t.Fatalf("open library = %q", a.snap().LibraryDir)
	}
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.LibraryDir != next {
		t.Fatalf("saved library = %q", saved.LibraryDir)
	}
	got := saved.LibraryInodes[next]
	if got.Dev != 7 || got.Ino != 99 {
		t.Fatalf("saved inode = %+v", got)
	}
	if _, ok := saved.LibraryInodes[old]; ok {
		t.Fatal("saved the old path")
	}
}

func containsPath(paths []string, want string) bool {
	want = filepath.Clean(want)
	for _, path := range paths {
		if filepath.Clean(path) == want {
			return true
		}
	}
	return false
}

func TestSwitchLibraryStopsTheJobAndRemembersTheDirectory(t *testing.T) {
	p := &fakePipeline{block: true}
	a, tt, updates := newTestApp(p)
	startJob(t, tt, "https://example.com/video")
	if !a.running() {
		t.Fatal("job did not start")
	}
	dir := filepath.Join(t.TempDir(), "library")
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.switchLibrary(dir)
	if a.snap().LibraryDir != dir {
		t.Fatalf("library = %q", a.snap().LibraryDir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.LibraryDir != dir || len(saved.Libraries) != 1 || saved.Libraries[0] != dir {
		t.Fatalf("saved %#v", saved)
	}
	settle(t, a, tt, updates)
	if a.running() {
		t.Fatal("switching the library left the job running")
	}
}

func TestSpeechModelSelectionIsSaved(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "ggml-base.bin")
	if err := os.WriteFile(model, []byte("ggml"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, _ := newTestApp(media.Tools{})
	a.modelsDir = dir
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt.Frame()
	if err := tt.Click("语音识别"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("已下载") || !tt.HasText("下载") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("Base"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tools, ok := a.pipeline.(media.Tools)
	if !ok || tools.WhisperModel != model {
		t.Fatalf("model %#v", a.pipeline)
	}
	if loadSavedSettings(a.settingsPath).WhisperModel != model {
		t.Fatalf("saved %#v", loadSavedSettings(a.settingsPath))
	}
}

func TestAISettingsSelectsAProvider(t *testing.T) {
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}}
	a, tt, _ := newTestApp(tools)
	a.syncLLM(tools)
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt.Frame()
	if err := tt.Click("AI 服务"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, text := range []string{"EveryAPI", "OpenAI 兼容", "Ollama", "Base URL", "API Key", "模型", "测试"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("OpenAI"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	got, ok := a.pipeline.(media.Tools)
	if !ok || got.Translator.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("base = %#v", a.pipeline)
	}
	if loadSavedSettings(a.settingsPath).LLMBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("saved %#v", loadSavedSettings(a.settingsPath))
	}
}

func TestWelcomeCreatesAndSwitchesLibraries(t *testing.T) {
	parent := t.TempDir()
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) {
		s.LibraryDir = ""
		s.Libraries = nil
		s.LibraryParent = parent
		s.LibraryName = "第一库"
	})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt.Frame()
	for _, text := range []string{"先准备一个资料库", "创建资料库", "导入已有资料库", "更改位置"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("创建资料库"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	first := filepath.Join(parent, "第一库.lingocut")
	if a.snap().LibraryDir != first {
		t.Fatalf("library = %q", a.snap().LibraryDir)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("先准备一个资料库") {
		t.Fatal("the welcome stayed after the library was created")
	}
	if err := tt.Click("资料库菜单"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("新建资料库..."); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	a.edit(func(s *session) { s.LibraryName = "第二库" })
	tt.Frame()
	if !tt.HasText("新建资料库") || !tt.HasText("创建资料库") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("创建资料库"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	second := filepath.Join(parent, "第二库.lingocut")
	if a.snap().LibraryDir != second || len(a.snap().Libraries) != 2 {
		t.Fatalf("library %q list %v", a.snap().LibraryDir, a.snap().Libraries)
	}
	if err := tt.Click("资料库菜单"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("第一库"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	if a.snap().LibraryDir != first {
		t.Fatalf("switched library = %q", a.snap().LibraryDir)
	}
	if err := tt.Click("通用"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("切换"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("从列表移出"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	if a.snap().LibraryDir != second {
		t.Fatalf("after forget, library = %q", a.snap().LibraryDir)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal(err)
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.LibraryDir != second || len(saved.Libraries) != 1 || saved.Libraries[0] != second {
		t.Fatalf("saved %#v", saved)
	}
}

func TestFilmsStayInsideTheOpenLibrary(t *testing.T) {
	a := newApp(&fakePipeline{}, func(func()) {})
	a.edit(func(s *session) { s.LibraryDir = "/libraries/one" })
	a.library.Jobs = []core.Job{
		{ID: "in", Stage: core.StageComplete, VideoPath: "/libraries/one/a/video.mp4"},
		{ID: "out", Stage: core.StageComplete, VideoPath: "/libraries/two/b/video.mp4"},
		{ID: "soon", Stage: core.StageDownloading, LibraryDir: "/libraries/one"},
	}
	a.edit(func(s *session) {
		s.Videos = []media.Video{{Path: "/libraries/one/c/video.mp4", Name: "c.mp4"}}
	})
	var names []string
	for _, item := range a.films() {
		names = append(names, item.Key)
	}
	if strings.Join(names, ",") != "in,soon,/libraries/one/c/video.mp4" {
		t.Fatalf("films = %v", names)
	}
	a.edit(func(s *session) { s.LibraryDir = "/libraries/empty" })
	if len(a.films()) != 0 {
		t.Fatalf("empty library films = %#v", a.films())
	}
}

func TestRefreshDropsAVideoRemovedFromTheLibrary(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "talk")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(folder, "video.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newApp(&fakePipeline{}, func(func()) {})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{ID: "talk", Stage: core.StageComplete, VideoPath: video}}
	a.openKey = "talk"
	a.refreshLibrary()
	if len(a.snap().Videos) != 1 || len(a.library.Jobs) != 1 {
		t.Fatalf("videos %d jobs %d", len(a.snap().Videos), len(a.library.Jobs))
	}
	if err := os.RemoveAll(folder); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	if len(a.snap().Videos) != 0 || len(a.library.Jobs) != 0 || a.openKey != "" {
		t.Fatalf("videos %d jobs %d open %q", len(a.snap().Videos), len(a.library.Jobs), a.openKey)
	}
}

func TestSwitchLibraryUpdatesOutputDir(t *testing.T) {
	a, _, _ := newTestApp(media.Tools{OutputDir: t.TempDir()})
	dir := filepath.Join(t.TempDir(), "movies")
	a.switchLibrary(dir)
	tools, ok := a.pipeline.(media.Tools)
	if !ok || tools.OutputDir != dir {
		t.Fatalf("output = %#v", a.pipeline)
	}
}

func TestOpenFilmShowsBilingualLines(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "talk.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	srt := filepath.Join(dir, "talk.bilingual.srt")
	body := "1\n00:00:01,000 --> 00:00:02,500\n你好。\nHello.\n\n2\n01:02:03,004 --> 01:02:05,000\n再见。\nBye.\n\n"
	if err := os.WriteFile(srt, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "job1", Source: "file://" + video, Language: "简体中文", Stage: core.StageComplete,
		VideoPath: video, SubtitlePath: srt,
	}}
	var revealed string
	prev := revealInFolder
	revealInFolder = func(path string) error {
		revealed = path
		return nil
	}
	t.Cleanup(func() { revealInFolder = prev })

	tt.Frame()
	if err := tt.Click("talk"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	waitUpdate(t, updates)
	tt.Frame()
	for _, text := range []string{"返回", "已完成", "你好。", "Hello.", "再见。", "0:01", "1:02:03", "2 句", "在访达中显示"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("在访达中显示"); err != nil {
		t.Fatal(err)
	}
	if revealed != srt {
		t.Fatalf("revealed %q", revealed)
	}
	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("你好。") || !tt.HasText("开始处理") || a.openKey != "" {
		t.Fatalf("open %q texts %q", a.openKey, tt.Texts())
	}
}

func TestOpenFilmPlaysTheVideo(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=black:s=160x90:d=1:r=10", "-c:v", "mpeg4", "-pix_fmt", "yuv420p", video).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg could not make a clip: %v %s", err, out)
	}
	srt := filepath.Join(dir, "clip.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,000\n画面上的字幕\nOn the picture.\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(media.Tools{FFmpeg: ffmpeg})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "clip", Source: "file://" + video, Language: "简体中文", Stage: core.StageComplete,
		VideoPath: video, SubtitlePath: srt,
	}}
	t.Cleanup(a.stopPlayback)
	tt.Frame()
	if err := tt.Click("clip"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	deadline := time.After(8 * time.Second)
	for !tt.HasText("播放") {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatalf("play button did not show: %q err %q", tt.Texts(), a.playErr)
		}
		tt.Frame()
	}
	if a.play == nil || a.play.player.Playing() {
		t.Fatal("video started before the play button")
	}
	deadline = time.After(2 * time.Second)
	for textCount(tt, "画面上的字幕") < 2 {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatalf("subtitle did not show on the picture: %q", tt.Texts())
		}
		tt.Frame()
	}
	if err := tt.Click("播放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.play == nil || !a.play.player.Playing() || !tt.HasText("暂停") {
		t.Fatalf("playing %v texts %q", a.play != nil && a.play.player.Playing(), tt.Texts())
	}
	if err := tt.Click("暂停"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.play == nil || a.play.player.Playing() || !tt.HasText("播放") {
		t.Fatalf("playing %v texts %q", a.play != nil && a.play.player.Playing(), tt.Texts())
	}
	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.play != nil || a.openKey != "" {
		t.Fatalf("open %q play %v", a.openKey, a.play != nil)
	}
}

func TestPlaybackAdvancesThePicture(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=160x90:d=1:r=10",
		"-f", "lavfi", "-i", "color=c=blue:s=160x90:d=1:r=10",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0",
		"-c:v", "mpeg4", "-pix_fmt", "yuv420p", video,
	).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg could not make a clip: %v %s", err, out)
	}
	a, tt, updates := newTestApp(media.Tools{FFmpeg: ffmpeg})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "clip", Source: "file://" + video, Stage: core.StageComplete, VideoPath: video,
	}}
	t.Cleanup(a.stopPlayback)
	tt.Frame()
	if err := tt.Click("clip"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(8 * time.Second)
	for a.play == nil {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatalf("player did not open: %q", a.playErr)
		}
		tt.Frame()
	}
	before := sampleStage(tt.Image())
	if err := tt.Click("播放"); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(4 * time.Second)
	after := before
	for time.Now().Before(until) {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
		after = sampleStage(tt.Image())
		if after != before {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if before == after {
		t.Fatalf("picture stayed %v playing %v held %v err %v", before, a.play.player.Playing(), a.play.paused, a.play.player.Err())
	}
}

func TestPlaybackStopsAtTheEnd(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=red:s=160x90:d=1:r=10", "-c:v", "mpeg4", "-pix_fmt", "yuv420p", video).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg could not make a clip: %v %s", err, out)
	}
	a, tt, updates := newTestApp(media.Tools{FFmpeg: ffmpeg})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "clip", Source: "file://" + video, Stage: core.StageComplete, VideoPath: video,
	}}
	t.Cleanup(a.stopPlayback)
	tt.Frame()
	if err := tt.Click("clip"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(8 * time.Second)
	for a.play == nil {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatalf("player did not open: %q", a.playErr)
		}
		tt.Frame()
	}
	if a.play.duration <= 0 {
		t.Fatal("clip has no duration")
	}
	if err := tt.Click("播放"); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(4 * time.Second)
	for time.Now().Before(until) && (a.play == nil || !a.play.finished) {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
		time.Sleep(20 * time.Millisecond)
	}
	if a.play == nil || !a.play.finished || a.play.player.Playing() {
		t.Fatalf("finished %v playing %v", a.play != nil && a.play.finished, a.play != nil && a.play.player.Playing())
	}
	head := a.play.playhead()
	if head > a.play.duration {
		t.Fatalf("playhead %s past %s", head, a.play.duration)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if a.play.player.Playing() || a.play.playhead() != head {
		t.Fatalf("kept playing at %s", a.play.playhead())
	}
}

func TestSeekJumpsIntoTheLaterPicture(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=160x90:d=1:r=10",
		"-f", "lavfi", "-i", "color=c=blue:s=160x90:d=1:r=10",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0",
		"-c:v", "mpeg4", "-pix_fmt", "yuv420p", video,
	).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg could not make a clip: %v %s", err, out)
	}
	a, tt, updates := newTestApp(media.Tools{FFmpeg: ffmpeg})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "clip", Source: "file://" + video, Stage: core.StageComplete, VideoPath: video,
	}}
	t.Cleanup(a.stopPlayback)
	tt.Frame()
	if err := tt.Click("clip"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(8 * time.Second)
	for a.play == nil {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatalf("player did not open: %q", a.playErr)
		}
		tt.Frame()
	}
	before := sampleStage(tt.Image())
	a.seekPlayback(time.Second + 200*time.Millisecond)
	until := time.Now().Add(4 * time.Second)
	after := before
	for time.Now().Before(until) {
		select {
		case fn := <-updates:
			fn()
		default:
		}
		tt.Frame()
		if a.play != nil && !a.play.paused && a.play.playhead() >= time.Second {
			after = sampleStage(tt.Image())
			if after != before {
				return
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	head := time.Duration(0)
	paused := false
	if a.play != nil {
		head = a.play.playhead()
		paused = a.play.paused
	}
	t.Fatalf("picture stayed %v head %s paused %v err %q", after, head, paused, a.playErr)
}

func TestFilmChaptersCoverTheTranscript(t *testing.T) {
	cues := []core.Segment{
		{Start: 0, End: 7 * time.Second, Text: "第一句\nFirst."},
		{Start: 7 * time.Second, End: 40 * time.Second, Text: "第二句\nSecond."},
		{Start: 40 * time.Second, End: 70 * time.Second, Text: "第三句\nThird."},
		{Start: 70 * time.Second, End: 100 * time.Second, Text: "第四句\nFourth."},
	}
	chapters := film.Chapters(cues)
	if len(chapters) != 4 {
		t.Fatalf("chapters %d", len(chapters))
	}
	if chapters[0].Title != "第一句" || chapters[0].Start != 0 || chapters[3].End != 100*time.Second {
		t.Fatalf("chapters %#v", chapters)
	}
}

func TestFilmSideShowsChaptersAndStyle(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "talk.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	srt := filepath.Join(dir, "talk.bilingual.srt")
	body := "1\n00:00:01,000 --> 00:00:02,500\n你好。\nHello.\n\n2\n01:02:03,004 --> 01:02:05,000\n再见。\nBye.\n\n"
	if err := os.WriteFile(srt, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "job1", Source: "file://" + video, Language: "简体中文", Stage: core.StageComplete,
		VideoPath: video, SubtitlePath: srt,
	}}
	tt.Frame()
	if err := tt.Click("talk"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	waitUpdate(t, updates)
	tt.Frame()
	if err := tt.Click("精彩片段"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	for _, text := range []string{"你好。", "2 段", "0:01"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("字幕样式"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("译文在上") || !tt.HasText("字号") || !tt.HasText("底框") || !tt.HasText("恢复默认样式") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func textCount(tt *ui.Tester, s string) int {
	n := 0
	for _, text := range tt.Texts() {
		if strings.Contains(text, s) {
			n++
		}
	}
	return n
}

func sampleStage(img *image.RGBA) [3]uint8 {
	// The film picture fills the page while it is open, so the middle of the window is on it.
	b := img.Bounds()
	x, y := (b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2
	if x >= b.Max.X {
		x = b.Max.X - 1
	}
	if y >= b.Max.Y {
		y = b.Max.Y - 1
	}
	r, g, b0, _ := img.At(x, y).RGBA()
	return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b0 >> 8)}
}

func TestCueAtFollowsThePlayhead(t *testing.T) {
	cues := []core.Segment{
		{Start: time.Second, End: 2500 * time.Millisecond, Text: "你好。"},
		{Start: time.Hour, End: time.Hour + time.Second, Text: "再见。"},
	}
	got, ok := film.CueAt(cues, 1500*time.Millisecond)
	if !ok || got.Text != "你好。" {
		t.Fatalf("cue %q ok %v", got.Text, ok)
	}
	if _, ok := film.CueAt(cues, 0); ok {
		t.Fatal("a cue before the first line")
	}
}

func TestOpenFilmStartsLocalVideo(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "interview.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.refreshLibrary()
	tt.Frame()
	if err := tt.Click("interview"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	if !tt.HasText("还没有字幕") || !tt.HasText("开始处理") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("开始处理"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	if a.job == nil || a.job.Stage != core.StageComplete || a.openKey != a.job.ID {
		t.Fatalf("open %q job %#v", a.openKey, a.job)
	}
	if !tt.HasText("已完成") || !tt.HasText("返回") {
		t.Fatalf("texts %q", tt.Texts())
	}
	waitUpdate(t, updates)
	tt.Frame()
	if !tt.HasText("字幕文件不在了") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestRemoveOnCardDoesNotOpenTheFilm(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.library.Jobs = []core.Job{{
		ID: "j", Source: "https://example.com/v", Stage: core.StageFailed, Err: errors.New("boom"),
		LibraryDir: a.snap().LibraryDir,
	}}
	tt.Frame()
	if err := tt.Click("移除"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	if a.openKey != "" || len(a.library.Jobs) != 0 {
		t.Fatalf("open %q jobs %#v", a.openKey, a.library.Jobs)
	}
}

func waitUpdate(t *testing.T, updates <-chan func()) {
	t.Helper()
	select {
	case fn := <-updates:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a background update")
	}
}

func TestImportLocalVideoSetsFileSource(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	a.importLocal("/Users/test/interview.mp4")
	if a.source != "file:///Users/test/interview.mp4" {
		t.Fatalf("source = %q", a.source)
	}
	if a.status != "已选择本地视频" {
		t.Fatalf("status = %q", a.status)
	}
}
