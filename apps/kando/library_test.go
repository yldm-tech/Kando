package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

func TestLibraryShowsAndRestoresJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, tt, updates := newTestApp(&fakePipeline{})
	a.openLibrary(path)
	startJob(t, tt, "https://example.com/video")
	settle(t, a, tt, updates)
	if !tt.HasText("talk") || !tt.HasText("最近完成") {
		t.Fatalf("texts %q after the job", tt.Texts())
	}
	if n := sidebarCount(t, tt, "全部影片"); n != "1" {
		t.Fatalf("全部影片 shows %q films, texts %q", n, tt.Texts())
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
	got := jobName(core.Job{VideoPath: "/Movies/影视库.kando/Learn Rust [abc]/video.mp4"})
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
	if films.Columns(900) != 2 || films.Columns(1100) != 3 || films.Columns(1680) != 5 || films.Columns(700) != 1 {
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

func TestDropQueuesLocalVideo(t *testing.T) {
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
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != core.FileURL(src) || !tt.HasText("已加入，开始处理。") {
		t.Fatalf("jobs %#v texts %q", a.library.Jobs, tt.Texts())
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
	if a.queue.status != "没有可导入的视频" {
		t.Fatalf("status %q", a.queue.status)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("non-video was copied")
	}
}

func TestViewRejectsInvalidSource(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	startJob(t, tt, "not-a-url")
	tt.Frame()
	if a.queue.job != nil || !tt.HasText("这不像一个视频链接") || a.queue.source != "not-a-url" {
		t.Fatalf("texts %q source %q after an invalid source", tt.Texts(), a.queue.source)
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

// TestAVideoWithoutAJobShowsItsBilingualSubtitles checks that a video copied into the library with the bilingual .srt the pipeline exported for it, but no job record, plays with those subtitles and its card does not say it has none, as the CLI already reads them.
func TestAVideoWithoutAJobShowsItsBilingualSubtitles(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{
		"interview.mp4":           "video",
		"interview.bilingual.srt": "1\n00:00:00,500 --> 00:00:01,000\n早上好\nGood morning\n\n",
		"silent.mp4":              "video",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.refreshLibrary()
	tt.Frame()
	subs := map[string]string{}
	for _, item := range a.films() {
		subs[item.Name] = item.SubtitlePath
	}
	if want := filepath.Join(dir, "interview", "video.bilingual.srt"); subs["interview"] != want || subs["silent"] != "" || len(subs) != 2 {
		t.Fatalf("subtitles %q, want interview's %q", subs, want)
	}
	saying := 0
	for _, text := range tt.Texts() {
		if strings.Contains(text, locales.FilmsNoSubtitles()) {
			saying++
		}
	}
	if saying != 1 {
		t.Fatalf("%d cards say %q, want only silent's; texts %q", saying, locales.FilmsNoSubtitles(), tt.Texts())
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

func TestImportLocalVideoSetsFileSource(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	a.importLocal("/Users/test/interview.mp4")
	if a.queue.source != "file:///Users/test/interview.mp4" {
		t.Fatalf("source = %q", a.queue.source)
	}
	if a.queue.status != "已选择本地视频" {
		t.Fatalf("status = %q", a.queue.status)
	}
}

// libraryWith makes a library folder holding one film, name/video.mp4 with its bilingual subtitles, and an app showing it with the film's job finished.
func libraryWith(t *testing.T, name string) (a *app, tt *ui.Tester, video string) {
	t.Helper()
	lib := filepath.Join(t.TempDir(), "我的影片.kando")
	video = filepath.Join(lib, name, "video.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	srt := filepath.Join(lib, name, "video.bilingual.srt")
	for path, body := range map[string]string{video: "v", srt: "1\n00:00:01,000 --> 00:00:02,000\n你好。\nHello.\n\n"} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, tt, _ = newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = lib })
	a.library.Jobs = []core.Job{{ID: "film", Source: "https://youtu.be/TRWgBxVAb1M", Language: "简体中文", Stage: core.StageComplete, VideoPath: video, SubtitlePath: srt, LibraryDir: lib}}
	a.refreshLibrary()
	tt.Frame()
	return a, tt, video
}

// fakeTrash moves what goes to the Trash into a folder of the test's, and lists it.
func fakeTrash(t *testing.T) *[]string {
	t.Helper()
	bin := t.TempDir()
	var trashed []string
	prev := trashItem
	trashItem = func(path string) error {
		trashed = append(trashed, path)
		return os.Rename(path, filepath.Join(bin, filepath.Base(path)))
	}
	t.Cleanup(func() { trashItem = prev })
	return &trashed
}

func countText(tt *ui.Tester, s string) int {
	n := 0
	for _, text := range tt.Texts() {
		if text == s {
			n++
		}
	}
	return n
}

func TestDropQueuesEveryVideo(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{block: true})
	t.Cleanup(func() {
		// Cancel whatever runs until nothing does: once the queue keeps every dropped video waiting, stopping the running one starts the next.
		for a.RunningJob() != "" {
			a.Cancel(a.RunningJob())
			(<-updates)()
		}
	})
	src := t.TempDir()
	var paths []string
	for _, name := range []string{"one.mp4", "two.mov", "three.mkv", "notes.txt"} {
		path := filepath.Join(src, name)
		if err := os.WriteFile(path, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	tt.Frame()
	r, ok := tt.Find("开始处理")
	if !ok {
		t.Fatal("missing start button")
	}
	if !tt.DropFiles(r.X+r.W/2, r.Y+r.H/2, paths...) {
		t.Fatal("the library did not take the dropped files")
	}
	tt.Frame()
	if len(a.library.Jobs) == 0 {
		t.Fatalf("no job was queued: %q", tt.Texts())
	}
	// Each video either got a job, whose card shows, or the message bar says why the queue did not take it, as the queue of one job at a time does with errBusy.
	for _, path := range paths[:3] {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		queued := false
		for _, job := range a.library.Jobs {
			queued = queued || job.Source == core.FileURL(path)
		}
		if queued && !tt.HasText(name) {
			t.Fatalf("no card for %s: %q", name, tt.Texts())
		}
		if !queued && !tt.HasText(errBusy().Error()) {
			t.Fatalf("%s was dropped without a word: %q", name, tt.Texts())
		}
	}
	for _, job := range a.library.Jobs {
		if strings.HasSuffix(job.Source, "notes.txt") {
			t.Fatal("a text file was queued")
		}
	}
	if got := countText(tt, "取消处理"); got != len(a.library.Jobs) {
		t.Fatalf("%d cards for %d jobs: %q", got, len(a.library.Jobs), tt.Texts())
	}
}

func TestDropWithoutVideosSaysWhatCanBeAdded(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	note := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(note, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.dropFiles([]string{note})
	tt.Frame()
	if len(a.library.Jobs) != 0 || !tt.HasText(locales.PrecheckNoVideo()) {
		t.Fatalf("jobs %#v texts %q", a.library.Jobs, tt.Texts())
	}
	if err := tt.Click("关闭提示"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText(locales.PrecheckNoVideo()) {
		t.Fatal("the message bar stayed after ×")
	}
}

// TestDropElsewhereShowsTheLibrary drops files on a settings page and over an open film: neither page shows the library's message bar or queue, so the drop takes the window to the library, where both say what became of the files.
func TestDropElsewhereShowsTheLibrary(t *testing.T) {
	src := t.TempDir()
	video := filepath.Join(src, "talk.mp4")
	note := filepath.Join(src, "notes.txt")
	for _, path := range []string{video, note} {
		if err := os.WriteFile(path, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("settings", func(t *testing.T) {
		a, tt, updates := newTestApp(&fakePipeline{block: true})
		t.Cleanup(func() {
			a.Cancel(a.RunningJob())
			for a.RunningJob() != "" {
				(<-updates)()
			}
		})
		a.OpenSettings(settings.SectionTranslate)
		tt.Frame()
		wantSettings(t, a, settings.SectionTranslate)
		w, h := float32(1100), float32(720)
		if !tt.DropFiles(sidebarWidth+(w-sidebarWidth)/2, h/2, video) {
			t.Fatal("the settings page did not take the dropped file")
		}
		tt.Frame()
		if a.page != pageFilms || len(a.library.Jobs) != 1 {
			t.Fatalf("page %q jobs %#v", a.page, a.library.Jobs)
		}
		if !tt.HasText("已加入，开始处理。") || !tt.HasText("talk") {
			t.Fatalf("the drop was not shown: %q", tt.Texts())
		}
	})

	t.Run("open film", func(t *testing.T) {
		a, tt, _ := libraryWith(t, "Talk")
		a.Open(a.films()[0])
		tt.Frame()
		if a.openKey == "" || tt.HasText("开始处理") {
			t.Fatalf("the film did not open: %q", tt.Texts())
		}
		w, h := float32(1100), float32(720)
		if !tt.DropFiles(w/2, h/2, note) {
			t.Fatal("the open film did not take the dropped file")
		}
		tt.Frame()
		if a.page != pageFilms || a.openKey != "" || a.PlaybackOpen() {
			t.Fatalf("page %q open %q playback %v", a.page, a.openKey, a.PlaybackOpen())
		}
		if !tt.HasText(locales.PrecheckNoVideo()) {
			t.Fatalf("the message bar is not shown: %q", tt.Texts())
		}
	})
}

func TestQueueShowsEveryWaitingJob(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	for i := range 8 {
		a.library.Jobs = append(a.library.Jobs, core.Job{ID: string(rune('a' + i)), Source: "https://youtu.be/clip" + string(rune('a'+i)), Stage: core.StageQueued, LibraryDir: "/videos"})
	}
	tt.Frame()
	if got := countText(tt, "排队中 · 等待前面的任务"); got != 6 || !tt.HasText("显示全部 8 个") || !tt.HasText("8 个进行中") {
		t.Fatalf("%d cards: %q", got, tt.Texts())
	}
	if err := tt.Click("显示全部 8 个"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := countText(tt, "排队中 · 等待前面的任务"); got != 8 || !tt.HasText("收起") {
		t.Fatalf("%d cards after 显示全部: %q", got, tt.Texts())
	}
}

// TestLibraryControlsStayOpenWhileAJobRuns checks that nothing on the library page waits for the running job: the link field takes a link, 开始处理 and Enter pass it on, and the failed cards offer 重试 and 移除. Passing on means the queue added the link or the message bar says why it did not, as the queue of one job at a time does with errBusy; a field or button held shut while a job runs would do neither.
func TestLibraryControlsStayOpenWhileAJobRuns(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{block: true})
	t.Cleanup(func() {
		for a.RunningJob() != "" {
			a.Cancel(a.RunningJob())
			(<-updates)()
		}
	})
	a.library.Jobs = []core.Job{
		{ID: "gone", Source: "https://example.com/gone", Language: "简体中文", Stage: core.StageFailed, Err: errors.New("boom"), LibraryDir: "/videos"},
		{ID: "old", Source: "https://example.com/old", Language: "简体中文", Stage: core.StageFailed, Err: errors.New("boom"), LibraryDir: "/videos"},
	}
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	if a.RunningJob() == "" {
		t.Fatal("no job runs")
	}
	passedOn := func(source string) bool {
		for _, job := range a.library.Jobs {
			if job.Source == source {
				return true
			}
		}
		return tt.HasText(errBusy().Error())
	}
	tt.Frame()

	startJob(t, tt, "https://example.com/b")
	tt.Frame()
	if !passedOn("https://example.com/b") {
		t.Fatalf("开始处理 while a job runs did nothing: %q", tt.Texts())
	}

	a.queue.source, a.lib.notice = "", films.Notice{}
	tt.Frame()
	if err := tt.Click("视频链接"); err != nil {
		t.Fatal(err)
	}
	tt.Type("https://example.com/c")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !passedOn("https://example.com/c") {
		t.Fatalf("Enter in the link field while a job runs did nothing: %q", tt.Texts())
	}

	a.lib.notice = films.Notice{}
	tt.Frame()
	if countText(tt, "重试") != 2 || countText(tt, "移除") != 2 {
		t.Fatalf("the failed cards hide 重试 or 移除 while a job runs: %q", tt.Texts())
	}
	if err := tt.Click("移除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	left := ""
	for _, id := range []string{"gone", "old"} {
		if _, ok := a.library.Find(id); ok {
			left += id
		}
	}
	if left != "gone" && left != "old" {
		t.Fatalf("移除 while a job runs left %q", left)
	}
	if err := tt.Click("重试"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if job, _ := a.library.Find(left); job.Stage != core.StageQueued && !tt.HasText(errBusy().Error()) {
		t.Fatalf("重试 while a job runs did nothing: %+v %q", job, tt.Texts())
	}
}

func TestEnterKeepsTheFilmWhenRemoving(t *testing.T) {
	a, tt, video := libraryWith(t, "Talk")
	trashed := fakeTrash(t)
	if err := tt.Click("更多操作"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	if err := tt.ChooseMenuItem("从资料库移除…"); err != nil {
		t.Fatalf("%v menu %q", err, tt.Menu())
	}
	tt.Frame()
	if !tt.HasText("要从资料库移除「Talk」吗？") || !a.modalOpen() {
		t.Fatalf("no confirmation, or it does not hold the window (modal %v): %q", a.modalOpen(), tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(*trashed) != 0 || len(a.library.Jobs) != 1 || tt.HasText("要从资料库移除「Talk」吗？") {
		t.Fatalf("Enter removed the film: trashed %v jobs %d texts %q", *trashed, len(a.library.Jobs), tt.Texts())
	}
	if a.modalOpen() {
		t.Fatal("the closed confirmation still holds the window")
	}
	if _, err := os.Stat(video); err != nil {
		t.Fatal(err)
	}

	if err := tt.RightClick("Talk"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("从资料库移除…"); err != nil {
		t.Fatalf("%v menu %q", err, tt.Menu())
	}
	tt.Frame()
	if err := tt.Click("移除"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	if len(*trashed) != 1 || (*trashed)[0] != filepath.Dir(video) {
		t.Fatalf("trashed %v, want the film's folder", *trashed)
	}
	if len(a.library.Jobs) != 0 || len(a.snap().Videos) != 0 {
		t.Fatalf("jobs %#v videos %#v", a.library.Jobs, a.snap().Videos)
	}
	if _, ok := tt.Find("Talk"); ok || !tt.HasText("已把「Talk」移到废纸篓") {
		t.Fatalf("texts %q after removing", tt.Texts())
	}
}

func TestRemovingKeepsAVideoOutsideTheLibrary(t *testing.T) {
	lib := filepath.Join(t.TempDir(), "我的影片.kando")
	if err := os.Mkdir(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(t.TempDir(), "interview.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	trashed := fakeTrash(t)
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = lib })
	a.library.Jobs = []core.Job{{ID: "local", Source: core.FileURL(video), Language: "简体中文", Stage: core.StageComplete, VideoPath: video, LibraryDir: lib}}
	tt.Frame()
	if _, ok := tt.Find("interview"); !ok {
		t.Fatalf("a local video processed in this library is not on its shelf: %q", tt.Texts())
	}
	a.AskRemove(a.films()[0])
	tt.Frame()
	if !tt.HasText("会留在原处") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("移除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(*trashed) != 0 || len(a.library.Jobs) != 0 {
		t.Fatalf("trashed %v jobs %#v", *trashed, a.library.Jobs)
	}
	if _, err := os.Stat(video); err != nil {
		t.Fatal("the video outside the library was touched")
	}
}

func TestFilmFilesKeepsOtherVideosOfTheFolder(t *testing.T) {
	lib := t.TempDir()
	dir := filepath.Join(lib, "Talks")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.mp4", "one.bilingual.srt", "two.mp4"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := filmFiles(lib, filmOf(filepath.Join(dir, "one.mp4")))
	want := []string{filepath.Join(dir, "one.mp4"), filepath.Join(dir, "one.bilingual.srt")}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("filmFiles = %v, want %v", got, want)
	}
	if got := filmFiles(lib, filmOf(filepath.Join(lib, "loose.mp4"))); len(got) != 1 {
		t.Fatalf("a video at the library's root took more than itself: %v", got)
	}

	// A folder inside the film's folder is not on the shelf, and may hold videos the user never saw there, so the film's folder stays.
	course := filepath.Join(lib, "Course")
	if err := os.MkdirAll(filepath.Join(course, "extras"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"video.mp4", "video.bilingual.srt", filepath.Join("extras", "b.mp4")} {
		if err := os.WriteFile(filepath.Join(course, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got = filmFiles(lib, filmOf(filepath.Join(course, "video.mp4")))
	want = []string{filepath.Join(course, "video.mp4"), filepath.Join(course, "video.bilingual.srt")}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("filmFiles = %v, want %v", got, want)
	}
}

// TestRemoveSaysWhatGoesToTheTrash checks that the confirmation says the folder goes only when the folder is what TrashFilm moves, and that a folder inside the film's folder stays.
func TestRemoveSaysWhatGoesToTheTrash(t *testing.T) {
	const folderText = "影片文件夹会移到废纸篓"
	const filesText = "视频和它的字幕会移到废纸篓"

	a, tt, video := libraryWith(t, "Talk")
	a.AskRemove(a.films()[0])
	tt.Frame()
	if !tt.HasText(folderText + "，里面的视频、封面和字幕都可以从废纸篓找回。") {
		t.Fatalf("a film alone in its folder: %q", tt.Texts())
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	extra := filepath.Join(filepath.Dir(video), "extras", "b.mp4")
	if err := os.MkdirAll(filepath.Dir(extra), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extra, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	trashed := fakeTrash(t)
	a.AskRemove(a.films()[0])
	tt.Frame()
	texts := strings.Join(tt.Texts(), "\n")
	if strings.Contains(texts, folderText) || !strings.Contains(texts, filesText) {
		t.Fatalf("a film whose folder holds another folder: %q", tt.Texts())
	}
	if err := tt.Click("移除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	want := []string{video, filepath.Join(filepath.Dir(video), "video.bilingual.srt")}
	if strings.Join(*trashed, ",") != strings.Join(want, ",") {
		t.Fatalf("trashed %v, want %v", *trashed, want)
	}
	if _, err := os.Stat(extra); err != nil {
		t.Fatalf("the video in the folder inside went too: %v", err)
	}
}

func TestPrecheckSaysWhatToFix(t *testing.T) {
	ready := media.Tools{YTDLP: "yt-dlp", FFmpeg: "ffmpeg", Whisper: "whisper-cli", WhisperModel: "model.bin", Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: "m", APIKey: "k"}}
	link := []string{"https://youtu.be/TRWgBxVAb1M"}
	for _, tc := range []struct {
		name    string
		tools   func(*media.Tools)
		sources []string
		noLib   bool
		msg     string
		section string
	}{
		{name: "empty", sources: []string{"  "}, msg: locales.PrecheckEmpty()},
		{name: "not a link", sources: []string{"hello"}, msg: locales.PrecheckNotLink()},
		// yt-dlp would download every video of a channel or a list that names none of them into the library, under one card.
		{name: "a YouTube handle", sources: []string{"https://www.youtube.com/@veritasium"}, msg: locales.PrecheckChannel("YouTube")},
		{name: "a YouTube channel ID", sources: []string{"https://www.youtube.com/channel/UCHnyfMqiRRG1u-2MsSQLbXA"}, msg: locales.PrecheckChannel("YouTube")},
		{name: "a YouTube channel name", sources: []string{"https://www.youtube.com/c/veritasium/videos"}, msg: locales.PrecheckChannel("YouTube")},
		{name: "a bilibili channel", sources: []string{"https://space.bilibili.com/946974"}, msg: locales.PrecheckChannel("哔哩哔哩")},
		{name: "a playlist", sources: []string{"https://www.youtube.com/playlist?list=PLx0sYbCqOb8"}, msg: locales.PrecheckList("YouTube")},
		{name: "a list without a video", sources: []string{"https://www.youtube.com/watch?list=PLx0sYbCqOb8"}, msg: locales.PrecheckList("YouTube")},
		{name: "a bilibili collection", sources: []string{"https://space.bilibili.com/946974/channel/collectiondetail?sid=1"}, msg: locales.PrecheckList("哔哩哔哩")},
		{name: "a channel among videos", sources: []string{"https://youtu.be/TRWgBxVAb1M", "https://www.youtube.com/@veritasium"}, msg: locales.PrecheckChannel("YouTube")},
		{name: "a video in a list", sources: []string{"https://www.youtube.com/watch?v=TRWgBxVAb1M&list=PLx0sYbCqOb8"}},
		{name: "a part of a bilibili video", sources: []string{"https://www.bilibili.com/video/BV1GJ411x7h7?p=3"}},
		{name: "a local file", sources: []string{"file:///Users/me/Movies/clip.mp4"}},
		{name: "no library", sources: link, noLib: true, msg: locales.PrecheckNoLibrary()},
		{name: "no yt-dlp", sources: link, tools: func(t *media.Tools) { t.YTDLP = "" }, msg: locales.PrecheckToolsMissing("yt-dlp"), section: settings.SectionGeneral},
		{name: "a local file still needs yt-dlp", sources: []string{"file:///v.mp4"}, tools: func(t *media.Tools) { t.YTDLP = "" }, msg: locales.PrecheckToolsMissing("yt-dlp"), section: settings.SectionGeneral},
		{name: "no model", sources: link, tools: func(t *media.Tools) { t.WhisperModel = "" }, msg: locales.PrecheckNoSpeechModel(), section: settings.SectionSpeech},
		{name: "no LLM model", sources: link, tools: func(t *media.Tools) { t.Translator.Model = "" }, msg: locales.PrecheckNoLLMModel(), section: settings.SectionAI},
		{name: "no EveryAPI key", sources: link, tools: func(t *media.Tools) { t.Translator.APIKey = "" }, msg: locales.PrecheckEveryAPIMissing(), section: settings.SectionAI},
		{name: "ready", sources: link},
	} {
		tools := ready
		if tc.tools != nil {
			tc.tools(&tools)
		}
		a := newApp(tools, func(func()) {})
		if !tc.noLib {
			a.edit(func(s *session) { s.LibraryDir = "/videos" })
		}
		msg, section := a.precheck(tc.sources)
		if msg != tc.msg || section != tc.section {
			t.Errorf("%s: precheck = %q, %q; want %q, %q", tc.name, msg, section, tc.msg, tc.section)
		}
	}
}

// TestPrecheckAsksForWhatCheckAsks keeps precheck and the pipeline's own Check in step: a local video precheck lets through must not fail Check for a missing tool.
func TestPrecheckAsksForWhatCheckAsks(t *testing.T) {
	model := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(model, []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	ready := media.Tools{YTDLP: "yt-dlp", FFmpeg: "ffmpeg", Whisper: "whisper-cli", WhisperModel: model, Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: "m", APIKey: "k"}}
	local := []string{core.FileURL("/videos/v.mp4")}
	for _, name := range []string{"yt-dlp", "ffmpeg", "whisper-cli"} {
		tools := ready
		switch name {
		case "yt-dlp":
			tools.YTDLP = ""
		case "ffmpeg":
			tools.FFmpeg = ""
		case "whisper-cli":
			tools.Whisper = ""
		}
		a := newApp(tools, func(func()) {})
		a.edit(func(s *session) { s.LibraryDir = "/videos" })
		msg, _ := a.precheck(local)
		err := tools.Check()
		if err == nil || !strings.Contains(err.Error(), name+" is not installed") {
			t.Fatalf("without %s Check = %v", name, err)
		}
		if msg != locales.PrecheckToolsMissing(name) {
			t.Errorf("without %s precheck let a local video through to a Check that fails with %v", name, err)
		}
	}
	if err := ready.Check(); err != nil {
		t.Fatalf("ready tools fail Check: %v", err)
	}
}

func TestNoticeOpensTheSettingThatFixesIt(t *testing.T) {
	a, tt, _ := newTestApp(media.Tools{YTDLP: "yt-dlp", FFmpeg: "ffmpeg", Whisper: "whisper-cli", Translator: media.ChatTranslator{Model: "m", APIKey: "k"}})
	startJob(t, tt, "https://youtu.be/TRWgBxVAb1M")
	tt.Frame()
	if len(a.library.Jobs) != 0 || !tt.HasText(locales.PrecheckNoSpeechModel()) || a.queue.source == "" {
		t.Fatalf("jobs %d source %q texts %q", len(a.library.Jobs), a.queue.source, tt.Texts())
	}
	if err := tt.Click("打开设置"); err != nil {
		t.Fatal(err)
	}
	wantSettings(t, a, settings.SectionSpeech)
}

func TestFailedCardOpensTheSettingThatFixesIt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		reason  func() string
		section string
	}{
		{"speech model", errors.New("check: KANDO_WHISPER_MODEL is not set"), locales.PrecheckNoSpeechModel, settings.SectionSpeech},
		{"everyapi missing at check", fmt.Errorf("check: %w", media.ErrEveryAPIMissing), everyAPIMissing, settings.SectionAI},
		{"everyapi gone before translate", fmt.Errorf("translate: %w", media.ErrEveryAPIMissing), everyAPIMissing, settings.SectionAI},
		{"everyapi signed out", fmt.Errorf("translate: %w", media.ErrEveryAPISignedOut), everyAPISignedOut, settings.SectionAI},
		// What Kando said before it told a missing everyapi from a signed-out one, as a failed job in library.json still says it.
		{"everyapi legacy text", errors.New("translate: EveryAPI is not signed in; run `everyapi auth login` or set EVERYAPI_API_KEY"), everyAPISignedOut, settings.SectionAI},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, tt, _ := newTestApp(&fakePipeline{})
			a.library.Jobs = []core.Job{{ID: "j", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageFailed, Err: tc.err, LibraryDir: "/videos"}}
			tt.Frame()
			if !tt.HasText(tc.reason()) || !tt.HasText("1 个需要处理") {
				t.Fatalf("want %q, texts %q", tc.reason(), tt.Texts())
			}
			if err := tt.Click("打开设置"); err != nil {
				t.Fatal(err)
			}
			wantSettings(t, a, tc.section)
		})
	}
}

// A video without a sound track, as a silent screen recording posted on X, fails to transcribe; its card says so in the user's words, not ffmpeg's, also for a job that failed before Kando told this case apart and kept ffmpeg's words in library.json.
func TestFailedCardSaysTheVideoHasNoSound(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"no audio", fmt.Errorf("transcribe: %w", media.ErrNoAudio)},
		{"ffmpeg's words", errors.New("transcribe: ffmpeg: exit status 234: [out#0/wav @ 0x77a5028180] Output file does not contain any stream\nError opening output file /tmp/kando-1/audio.wav.\nError opening output files: Invalid argument")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, tt, _ := newTestApp(&fakePipeline{})
			a.library.Jobs = []core.Job{{ID: "j", Source: "https://x.com/seanaiux/status/2119830876703879366", Stage: core.StageFailed, Err: tc.err, LibraryDir: "/videos"}}
			tt.Frame()
			if !tt.HasText(locales.PrecheckNoAudio()) {
				t.Fatalf("want %q, texts %q", locales.PrecheckNoAudio(), tt.Texts())
			}
			for _, text := range tt.Texts() {
				if strings.Contains(text, "does not contain any stream") {
					t.Fatalf("the card shows ffmpeg's words: %q", text)
				}
			}
		})
	}
}

func TestNewLibrarySheetStaysOpenUntilEscape(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) {
		s.LibraryDir = ""
		s.LibraryParent = t.TempDir()
	})
	tt.Frame()
	if err := tt.Click("新建资料库…"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	if !a.MakingLibrary() || !tt.HasText("影视库.kando") || !a.modalOpen() {
		t.Fatalf("sheet did not open with the path, or does not hold the window (modal %v): %q", a.modalOpen(), tt.Texts())
	}
	tt.ClickAt(5, 5)
	tt.Frame()
	if !a.MakingLibrary() {
		t.Fatal("a click outside closed the sheet")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.MakingLibrary() || tt.HasText("影视库.kando") {
		t.Fatalf("Escape left the sheet open: %q", tt.Texts())
	}
	tt.Frame()
	if a.modalOpen() {
		t.Fatal("the closed sheet still holds the window")
	}
}

func TestPostersAreReadByTheShelfNotTheView(t *testing.T) {
	lib := t.TempDir()
	video := filepath.Join(lib, "Talk", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(media.Tools{FFmpeg: filepath.Join(t.TempDir(), "ffmpeg")})
	a.edit(func(s *session) {
		s.LibraryDir = lib
		s.Videos = []media.Video{{Path: video, Name: "Talk"}}
	})
	tt.Frame()
	if len(a.lib.posterBusy) != 0 || len(a.lib.posterMiss) != 0 {
		t.Fatalf("drawing the page asked for posters: busy %v miss %v", a.lib.posterBusy, a.lib.posterMiss)
	}
	a.refreshLibrary()
	if !a.lib.posterBusy[video] {
		t.Fatalf("the shelf did not ask for the poster: busy %v miss %v", a.lib.posterBusy, a.lib.posterMiss)
	}
	waitUpdate(t, updates)
	if a.lib.posterBusy[video] || !a.lib.posterMiss[video] {
		t.Fatalf("busy %v miss %v after the grab failed", a.lib.posterBusy, a.lib.posterMiss)
	}
}

func TestHeroOffersToContinue(t *testing.T) {
	a, tt, video := libraryWith(t, "Talk")
	if !tt.HasText("“你好。”") || !tt.HasText("Hello.") || !tt.HasText("播放") || !tt.HasText(locales.FilmsShowInFinder(platformName())) {
		t.Fatalf("hero %q", tt.Texts())
	}
	a.player.positions = map[string]time.Duration{video: 42 * time.Second}
	tt.Frame()
	if !tt.HasText("从 0:42 继续") || !tt.HasText("从头看") {
		t.Fatalf("hero %q", tt.Texts())
	}
	if err := tt.Click("从头看"); err != nil {
		t.Fatal(err)
	}
	if a.openKey != "film" {
		t.Fatalf("open %q", a.openKey)
	}
}

func TestEmptySourceSaysSo(t *testing.T) {
	a, tt, _ := libraryWith(t, "Talk")
	a.SetSourceFilter(sidebar.SourceBilibili)
	tt.Frame()
	if !tt.HasText("这个来源下还没有影片。") || !tt.HasText("哔哩哔哩") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestLocalJobNameUsesTheFileName(t *testing.T) {
	if got := jobName(core.Job{Source: core.FileURL("/Users/me/Downloads/Interview 2.mp4")}); got != "Interview 2" {
		t.Fatalf("name = %q", got)
	}
}

func filmOf(video string) films.Film { return films.Film{Key: video, VideoPath: video} }

// TestHeroShadowReachesPastTheShelf checks that the hero's shadow spreads past its sides into the page's padding, as the prototype's does, rather than stopping in a straight edge where the scroll view would cut it off, which drew a gray band under the hero.
func TestHeroShadowReachesPastTheShelf(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	a.openLibrary(filepath.Join(t.TempDir(), "library.json"))
	startJob(t, tt, "https://example.com/video")
	settle(t, a, tt, updates)
	hero, ok := tt.Find("talk")
	if !ok || hero.H < 300 {
		t.Fatalf("no hero in %q (found %v)", tt.Texts(), hero)
	}
	img := tt.Image()
	bg := img.RGBAAt(int(hero.X+hero.W/2), int(hero.Y+hero.H+60))
	for _, x := range []float32{hero.X - 4, hero.X + hero.W + 3} {
		at := img.RGBAAt(int(x), int(hero.Y+hero.H+4))
		if at.R >= bg.R {
			t.Fatalf("at x=%v just below the hero %v the page is %v, as light as the background %v: the shadow stops at the hero's side", x, hero, at, bg)
		}
	}
}

// TestLibraryPageOwnsTheTitleBar checks that the library page says it draws its own header bar under the title bar (libraryOwnsTitleBar), so contentBand leaves that room to it, on 全部影片 and YouTube, and that it stops saying so while a film is open over it. Without a title bar the header bar starts at the window's top, with no page padding above it: on macOS it is the prototype's toolbar, 65 points high with 10 above its controls and 18 below them, so its 34-point controls are centered 28.5 points down. Elsewhere the bar is the platform's own header bar (films/topbar.go), whose measures the platform's look sets, so the position is checked on macOS only.
func TestLibraryPageOwnsTheTitleBar(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.library.Jobs = []core.Job{{ID: "j", Source: "https://example.com/v", Stage: core.StageQueued}}
	tt.Frame()
	if !a.libraryOwnsTitleBar() {
		t.Fatalf("the library page on %q does not own the title bar", a.page)
	}
	if start, ok := tt.Find("开始处理"); !ok || (runtime.GOOS == "darwin" && abs32(start.Y+start.H/2-28.5) > 0.5) {
		t.Fatalf("开始处理 at %v, want it centered 28.5 points from the window's top", start)
	}
	a.Select(pageYouTube)
	tt.Frame()
	if !a.libraryOwnsTitleBar() {
		t.Fatalf("the library page on %q does not own the title bar", a.page)
	}
	a.Select(pageFilms)
	a.openKey = "j"
	tt.Frame()
	if a.libraryOwnsTitleBar() {
		t.Fatal("the library page owns the title bar under an open film")
	}
}

// The library watcher refreshes the shelf every second while yt-dlp downloads. "Me at the zoo" comes as separate video and sound tracks, and a refresh between the two must neither rename the finished video track to video.mp4 under yt-dlp, which then cannot merge it, nor show a track as a film. The download ends with one film, the merged video.
func TestRefreshDuringADownloadLeavesItsTracksAlone(t *testing.T) {
	home := t.TempDir()
	library := filepath.Join(home, "影视库.kando")
	if err := os.Mkdir(library, 0o755); err != nil {
		t.Fatal(err)
	}
	gate := t.TempDir()
	t.Setenv("FAKE_YTDLP_GATE", gate)
	tools := media.Tools{OutputDir: library, DownloadTemp: filepath.Join(home, "cache"), YTDLP: fakeTool(t, filepath.Join(home, "bin"), "yt-dlp")}
	a := newApp(tools, func(func()) {})
	a.edit(func(s *session) { s.LibraryDir = library })
	type result struct {
		path string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		path, err := tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw")
		done <- result{path, err}
	}()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(gate, "tracks")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("yt-dlp never wrote its tracks")
		}
	}
	a.refreshLibrary()
	if videos := a.snap().Videos; len(videos) != 0 {
		t.Errorf("films while downloading: %#v", videos)
	}
	if err := os.WriteFile(filepath.Join(gate, "go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var r result
	select {
	case r = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("download never finished")
	}
	if r.err != nil {
		t.Fatal(r.err)
	}
	a.refreshLibrary()
	want := filepath.Join(library, "Me at the zoo [jNQXAC9IVRw]", "video.mp4")
	videos := a.snap().Videos
	if r.path != want || len(videos) != 1 || videos[0].Path != want {
		t.Fatalf("downloaded %q, films %#v", r.path, videos)
	}
	if _, ok := readFakeClip(want); !ok {
		t.Fatalf("%s is not the merged video", want)
	}
	if entries, _ := os.ReadDir(filepath.Dir(want)); len(entries) != 1 {
		t.Fatalf("the film's folder holds %v", entries)
	}
}

// coverFilm puts a downloaded film in its own folder of a library on the shelf, with cover.jpg holding kept when kept is not nil and thumbnail as its site's picture, and returns the app, its updates and the film's video and cover. The pipeline names an ffmpeg that is not there, so a frame grab fails.
func coverFilm(t *testing.T, thumbnail string, kept []byte) (*app, chan func(), string, string) {
	t.Helper()
	lib := filepath.Join(t.TempDir(), "我的影片.kando")
	video := filepath.Join(lib, "Talk", "video.mp4")
	cover := filepath.Join(lib, "Talk", "cover.jpg")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	if kept != nil {
		if err := os.WriteFile(cover, kept, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, _, updates := newTestApp(media.Tools{FFmpeg: filepath.Join(t.TempDir(), "ffmpeg")})
	a.lib.posterDir = t.TempDir()
	a.edit(func(s *session) { s.LibraryDir = lib })
	a.library.Jobs = []core.Job{{ID: "film", Source: "https://www.youtube.com/watch?v=TRWgBxVAb1M", Thumbnail: thumbnail, Stage: core.StageComplete, VideoPath: video, LibraryDir: lib}}
	return a, updates, video, cover
}

// solidPicture is a w×h picture of one color, encoded by encode.
func solidPicture(t *testing.T, c color.RGBA, encode func(io.Writer, image.Image) error) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 18))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func encodeJPEG(w io.Writer, img image.Image) error { return jpeg.Encode(w, img, nil) }

// wantCoverColor fails t unless path is a JPEG whose middle is near c.
func wantCoverColor(t *testing.T, path string, c color.RGBA) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("%s is not a JPEG: %v", filepath.Base(path), err)
	}
	b := img.Bounds()
	r, g, bl, _ := img.At(b.Dx()/2, b.Dy()/2).RGBA()
	near := func(got uint32, want uint8) bool { d := int(got>>8) - int(want); return d > -24 && d < 24 }
	if !near(r, c.R) || !near(g, c.G) || !near(bl, c.B) {
		t.Fatalf("%s is %d,%d,%d, want %v", filepath.Base(path), r>>8, g>>8, bl>>8, c)
	}
}

var (
	coverRed  = color.RGBA{220, 30, 30, 255}
	coverGray = color.RGBA{128, 128, 128, 255}
)

// A film from a site shows the picture the site gives it, in place of the frame an earlier Kando grabbed into cover.jpg, which now holds the site's picture as JPEG though the site gave a PNG. The picture is fetched once and kept with the processing card's.
func TestPosterIsTheSitesCover(t *testing.T) {
	const address = "https://i.ytimg.com/vi/TRWgBxVAb1M/maxresdefault.png"
	var fetched []string
	prev := fetchSiteThumbnail
	fetchSiteThumbnail = func(_ context.Context, addr, dest string) error {
		fetched = append(fetched, addr)
		return os.WriteFile(dest, solidPicture(t, coverRed, png.Encode), 0o644)
	}
	t.Cleanup(func() { fetchSiteThumbnail = prev })
	a, updates, video, cover := coverFilm(t, address, solidPicture(t, coverGray, encodeJPEG))
	a.refreshLibrary()
	waitUpdate(t, updates)
	if a.posterOf(video) == nil || a.lib.posterMiss[video] {
		t.Fatalf("no poster: miss %v", a.lib.posterMiss[video])
	}
	wantCoverColor(t, cover, coverRed)
	if !slices.Equal(fetched, []string{address}) {
		t.Fatalf("fetched %q", fetched)
	}
	if !fileExists(siteThumbnailPath(a.lib.posterDir, address)) {
		t.Fatal("the site's picture was not kept")
	}
	// Found again, the kept picture serves without asking the site, and cover.jpg is left as it is.
	before, _ := os.Stat(cover)
	data, err := findCover(context.Background(), coverSources{site: address, siteCache: siteThumbnailPath(a.lib.posterDir, address), video: video, dest: cover})
	if err != nil || len(data) == 0 {
		t.Fatal(err)
	}
	if after, _ := os.Stat(cover); len(fetched) != 1 || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("fetched %q, cover.jpg rewritten %v", fetched, !after.ModTime().Equal(before.ModTime()))
	}
}

// When the site's picture cannot be had, as when the site no longer serves it, the film keeps the cover it has.
func TestPosterKeepsItsCoverWhenTheSiteHasNone(t *testing.T) {
	prev := fetchSiteThumbnail
	fetchSiteThumbnail = func(context.Context, string, string) error { return errors.New("thumbnail: 404 Not Found") }
	t.Cleanup(func() { fetchSiteThumbnail = prev })
	prevEmbedded := embeddedCover
	embeddedCover = func(context.Context, string, string) ([]byte, error) { return nil, errors.New("no cover art") }
	t.Cleanup(func() { embeddedCover = prevEmbedded })
	kept := solidPicture(t, coverGray, encodeJPEG)
	a, updates, video, cover := coverFilm(t, "https://p16-sign.tiktokcdn.com/expired.jpeg", kept)
	a.refreshLibrary()
	waitUpdate(t, updates)
	if a.posterOf(video) == nil {
		t.Fatal("no poster")
	}
	if data, _ := os.ReadFile(cover); !bytes.Equal(data, kept) {
		t.Fatal("cover.jpg changed")
	}
}

// A local video shows the cover art its file carries, kept as JPEG in cover.jpg, rather than a frame of it.
func TestPosterOfALocalVideoIsItsCoverArt(t *testing.T) {
	var asked []string
	prev := embeddedCover
	embeddedCover = func(_ context.Context, _, video string) ([]byte, error) {
		asked = append(asked, video)
		return solidPicture(t, coverRed, png.Encode), nil
	}
	t.Cleanup(func() { embeddedCover = prev })
	a, updates, video, cover := coverFilm(t, "", solidPicture(t, coverGray, encodeJPEG))
	a.refreshLibrary()
	waitUpdate(t, updates)
	if a.posterOf(video) == nil {
		t.Fatal("no poster")
	}
	if !slices.Equal(asked, []string{video}) {
		t.Fatalf("asked %q for cover art", asked)
	}
	wantCoverColor(t, cover, coverRed)
}

// A WebP picture, as YouTube often gives (testdata/cover.webp is golang.org/x/image's blue-purple-pink.lossy.webp), becomes a JPEG cover; a JPEG stays byte for byte.
func TestCoverJPEG(t *testing.T) {
	webp, err := os.ReadFile(filepath.Join("testdata", "cover.webp"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := coverJPEG(webp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("not a JPEG: %v", err)
	}
	kept := solidPicture(t, coverGray, encodeJPEG)
	if out, err := coverJPEG(kept); err != nil || !bytes.Equal(out, kept) {
		t.Fatalf("a JPEG was encoded again: %v", err)
	}
	if _, err := coverJPEG([]byte("<html>not found</html>")); err == nil {
		t.Fatal("a page passed for a picture")
	}
}

// TestLibraryInEnglish reads the library page in English: the hero of the film finished last and its poster, a waiting and a failed job, the top bar, and the dialog that removes a film.
func TestLibraryInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, tt, _ := libraryWith(t, "Talk")
	lib := a.snap().LibraryDir
	a.library.Jobs = append(a.library.Jobs,
		core.Job{ID: "wait", Source: "https://www.bilibili.com/video/BV1GJ411x7h7", Stage: core.StageQueued, LibraryDir: lib},
		core.Job{ID: "stop", Source: "https://vimeo.com/76979871", Stage: core.StageFailed, Err: context.Canceled, LibraryDir: lib},
	)
	tt.Frame()
	for _, want := range []string{
		"Recently Finished · Simplified Chinese Bilingual", "Play", locales.FilmsShowInFinder(platformName()),
		"Bilingual · CHS", "All Films", "1 film",
		"Processing", "1 in progress · 1 needs attention", "Queued · Waiting for the jobs ahead", "Canceled", "Retry", "Remove",
		"Start Processing", "Local Video",
	} {
		wantText(t, tt, want)
	}
	a.AskRemove(a.films()[0])
	tt.Frame()
	wantText(t, tt, "Remove “Talk” from the library?")
	wantText(t, tt, "The film's folder will be moved to the Trash. You can get its video, cover and subtitles back from there.")
	for _, chinese := range []string{"最近完成", "处理中", "开始处理", "已取消", "移除"} {
		if tt.HasText(chinese) {
			t.Fatalf("the library still reads %q: %q", chinese, tt.Texts())
		}
	}
}

// TestLibraryNamesTheSystemsFileManager checks that the library speaks of the file manager of the system it runs on: the hero's and the card menu's reveal, the shortcut in the tooltip of 本地视频, the page before any library and the new-library sheet, which promises Kando's folder icon only on macOS, where Finder shows it.
func TestLibraryNamesTheSystemsFileManager(t *testing.T) {
	for _, tc := range []struct {
		platform                 breeze.Platform
		reveal, tooltip, manager string
		icon                     bool
	}{
		{breeze.Mac, "在访达中显示", "添加本地视频（⌘O）", "可以直接在访达里管理。", true},
		{breeze.Windows, "在文件资源管理器中显示", "添加本地视频（Ctrl+O）", "可以直接在文件资源管理器里管理。", false},
		{breeze.Linux, "在文件管理器中显示", "添加本地视频（Ctrl+O）", "可以直接在文件管理器里管理。", false},
	} {
		t.Run(tc.platform.String(), func(t *testing.T) {
			usePlatform(t, tc.platform)
			var revealed []string
			prev := revealInFolder
			revealInFolder = func(path string) error {
				revealed = append(revealed, path)
				return nil
			}
			t.Cleanup(func() { revealInFolder = prev })
			a, tt, video := libraryWith(t, "Talk")
			wantText(t, tt, tc.reveal)
			if err := tt.Click(tc.reveal); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(revealed, []string{video}) {
				t.Fatalf("revealed %q, want the video %q", revealed, video)
			}
			if err := tt.RightClick("Talk"); err != nil {
				t.Fatalf("%v texts %q", err, tt.Texts())
			}
			if menu := tt.Menu(); !slices.Contains(menu, tc.reveal) {
				t.Fatalf("card menu %q lacks %q", menu, tc.reveal)
			}
			tt.CloseMenu()
			r, ok := tt.Find("本地视频")
			if !ok {
				t.Fatalf("no 本地视频 in %q", tt.Texts())
			}
			tt.Move(r.X+r.W/2, r.Y+r.H/2)
			tt.Advance(time.Second)
			tt.Frame()
			wantText(t, tt, tc.tooltip)

			a.edit(func(s *session) {
				s.LibraryDir = ""
				s.LibraryParent = t.TempDir()
			})
			tt.Frame()
			wantText(t, tt, tc.manager)
			a.edit(func(s *session) { s.MakingLibrary = true })
			tt.Frame()
			wantText(t, tt, "会创建一个 .kando 文件夹")
			if got := tt.HasText("看懂标志"); got != tc.icon {
				t.Fatalf("the sheet promises the Kando icon: %v, want %v: %q", got, tc.icon, tt.Texts())
			}
		})
	}
}

// libraryStopJobs takes every waiting job out of the queue and cancels the running one, applying updates until it has stopped, so a test whose pipeline blocks leaves nothing running.
func libraryStopJobs(a *app, updates chan func()) {
	for _, job := range slices.Clone(a.library.Jobs) {
		if job.ID != a.RunningJob() {
			a.Cancel(job.ID)
		}
	}
	if id := a.RunningJob(); id != "" {
		a.Cancel(id)
		for a.RunningJob() != "" {
			(<-updates)()
		}
	}
}

// libraryOpenedVideos writes a file for each name in a folder of its own and returns their paths.
func libraryOpenedVideos(t *testing.T, names ...string) []string {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join(dir, name)
		if err := os.WriteFile(paths[i], []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

// TestOpenedFilesWaitForTheWindow opens videos as Finder does when it launches Kando with them: they arrive before the window exists, when nothing may report through update, and are added to the queue together once it does.
func TestOpenedFilesWaitForTheWindow(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{block: true})
	t.Cleanup(func() { libraryStopJobs(a, updates) })
	paths := libraryOpenedVideos(t, "talk.mp4", "interview.mov")
	hooks := a.libraryHooks()
	hooks.restore(savedSettings{})
	a.libraryOpenFile(paths[0])
	a.libraryOpenFile(paths[0])
	a.libraryOpenFile(paths[1])
	if len(a.library.Jobs) != 0 || len(updates) != 0 {
		t.Fatalf("before the window: jobs %#v, %d updates posted", a.library.Jobs, len(updates))
	}
	hooks.ready()
	if len(updates) != 1 || len(a.library.Jobs) != 0 {
		t.Fatalf("once the window exists: %d updates posted, jobs %#v; want one report adding both after every area's ready", len(updates), a.library.Jobs)
	}
	(<-updates)()
	tt.Frame()
	if len(a.library.Jobs) != 2 || len(a.lib.opened) != 0 {
		t.Fatalf("jobs %#v still waiting %q", a.library.Jobs, a.lib.opened)
	}
	for _, path := range paths {
		if !libraryHasJobFor(a.library.Jobs, core.FileURL(path)) {
			t.Fatalf("no job for %s: %#v", path, a.library.Jobs)
		}
	}
	if !tt.HasText(locales.LibraryAddedStarting(2)) {
		t.Fatalf("the message bar does not say both were added: %q", tt.Texts())
	}
}

func libraryHasJobFor(jobs []core.Job, source string) bool {
	return slices.ContainsFunc(jobs, func(job core.Job) bool { return job.Source == source })
}

// TestOpenedFilesArriveTogether opens two videos once the window exists, as dropping them on the Dock icon does: MyGo reports them one by one, and they reach the queue in one report through update, on the library, with one message for both.
func TestOpenedFilesArriveTogether(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{block: true})
	t.Cleanup(func() { libraryStopJobs(a, updates) })
	paths := libraryOpenedVideos(t, "talk.mp4", "interview.mkv")
	a.Select(pageYouTube)
	a.libraryOpenFile(paths[0])
	a.libraryOpenFile(paths[1])
	if len(updates) != 1 || len(a.library.Jobs) != 0 {
		t.Fatalf("%d updates posted, jobs %#v; want one report adding both", len(updates), a.library.Jobs)
	}
	(<-updates)()
	tt.Frame()
	if a.page != pageFilms || len(a.library.Jobs) != 2 {
		t.Fatalf("page %q jobs %#v", a.page, a.library.Jobs)
	}
	if !tt.HasText(locales.LibraryAddedStarting(2)) {
		t.Fatalf("the message bar does not say both were added: %q", tt.Texts())
	}
	a.libraryOpenFile(paths[1] + ".srt")
	if len(updates) != 1 {
		t.Fatalf("%d updates posted after the first report ran; want one", len(updates))
	}
}

// TestOpenedFileThatIsNotAVideoWarns opens a file that is not a video, as Open With can when the user picks Kando for any file: nothing is queued and the message bar says what can be added, as a drop on the window does.
func TestOpenedFileThatIsNotAVideoWarns(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	note := libraryOpenedVideos(t, "notes.txt")[0]
	a.libraryOpenFile(note)
	(<-updates)()
	tt.Frame()
	if len(a.library.Jobs) != 0 || !tt.HasText(locales.PrecheckNoVideo()) {
		t.Fatalf("jobs %#v texts %q", a.library.Jobs, tt.Texts())
	}
}

// TestOpenedVideoAlreadyQueuedIsNotAddedAgain opens a video and a symbolic link to it in one batch, which queues it once, then opens it again while it is being processed, by the same path and by other ones, as macOS hands a video named on the command line to the app twice: it is not queued again. Once its job has stopped, opening it again queues it again, on the card it has (queueTake retries a stopped job rather than adding a second one).
func TestOpenedVideoAlreadyQueuedIsNotAddedAgain(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{block: true})
	t.Cleanup(func() { libraryStopJobs(a, updates) })
	video := libraryOpenedVideos(t, "talk.mp4")[0]
	link := filepath.Join(t.TempDir(), "link.mp4")
	if err := os.Symlink(video, link); err != nil {
		t.Fatal(err)
	}
	a.libraryOpenFile(video)
	a.libraryOpenFile(link)
	(<-updates)()
	if len(a.library.Jobs) != 1 || a.RunningJob() == "" {
		t.Fatalf("opening a video and a link to it together: jobs %#v running %q; want one job", a.library.Jobs, a.RunningJob())
	}
	for _, again := range []string{video, filepath.Join(filepath.Dir(video), ".", "talk.mp4"), link} {
		a.Select(pageYouTube)
		a.libraryOpenFile(again)
		(<-updates)()
		tt.Frame()
		if len(a.library.Jobs) != 1 {
			t.Fatalf("opening %s again queued it again: %#v", again, a.library.Jobs)
		}
		if a.page != pageFilms {
			t.Fatalf("opening %s again left the window on %q", again, a.page)
		}
	}
	libraryStopJobs(a, updates)
	a.libraryOpenFile(video)
	(<-updates)()
	if len(a.library.Jobs) != 1 || a.RunningJob() != a.library.Jobs[0].ID {
		t.Fatalf("a video whose job stopped was not queued again on its card: jobs %#v running %q", a.library.Jobs, a.RunningJob())
	}
}

// TestFileAssociationsAreTheVideos checks mygo.json's fileAssociations, which put Kando in Finder's Open With and let videos be dropped on its Dock icon: they list each video extension 本地视频 offers once, every one a video the library takes, and Kando only views them.
func TestFileAssociationsAreTheVideos(t *testing.T) {
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		FileAssociations []struct {
			Ext      []string `json:"ext"`
			Name     string   `json:"name"`
			MimeType string   `json:"mimeType"`
			Role     string   `json:"role"`
		} `json:"fileAssociations"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	var exts []string
	for _, fa := range config.FileAssociations {
		if fa.Name == "" || !strings.HasPrefix(fa.MimeType, "video/") || fa.Role != "Viewer" {
			t.Errorf("association %+v: want a name, a video MIME type and the Viewer role", fa)
		}
		for _, ext := range fa.Ext {
			if !media.IsVideoFile("clip." + ext) {
				t.Errorf("%q is associated but the library does not take it", ext)
			}
			if slices.Contains(exts, ext) {
				t.Errorf("%q is associated twice", ext)
			}
			exts = append(exts, ext)
		}
	}
	slices.Sort(exts)
	want := slices.Sorted(slices.Values(libraryVideoExtensions))
	if !slices.Equal(exts, want) {
		t.Fatalf("associated %q, want the extensions 本地视频 offers %q", exts, want)
	}
}

// librarySearchWith makes a library of two done films, Rivers of Europe and Linear Algebra, and a failed job, Morning Market, and an app showing it.
func librarySearchWith(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	lib := filepath.Join(t.TempDir(), "我的影片.kando")
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = lib })
	for i, name := range []string{"Rivers of Europe", "Linear Algebra"} {
		video := filepath.Join(lib, name, "video.mp4")
		if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
		a.library.Jobs = append(a.library.Jobs, core.Job{ID: "done" + string(rune('a'+i)), Source: "https://youtu.be/clip" + string(rune('a'+i)), Language: "简体中文", Stage: core.StageComplete, VideoPath: video, LibraryDir: lib})
	}
	a.library.Jobs = append(a.library.Jobs, core.Job{ID: "failed", Title: "Morning Market", Source: "https://youtu.be/market", Stage: core.StageFailed, Err: errors.New("network down"), LibraryDir: lib})
	a.refreshLibrary()
	tt.Frame()
	return a, tt
}

// TestSearchFiltersTheShelfByTitle checks that the sidebar's search field narrows the grid to the done films whose names hold the query, whatever its case, leaves the hero out while it does and keeps 处理中 whole; that a query no name holds says so, with 清除搜索 bringing every film back; and that it filters as `kando list [query]` does.
func TestSearchFiltersTheShelfByTitle(t *testing.T) {
	a, tt := librarySearchWith(t)
	if !tt.HasText("最近完成") || !tt.HasText("Rivers of Europe") || !tt.HasText("Linear Algebra") {
		t.Fatalf("library before the search: %q", tt.Texts())
	}
	if err := tt.Click("搜索影片"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Type("RIVERS")
	tt.Frame()
	if a.side.query != "RIVERS" {
		t.Fatalf("query %q", a.side.query)
	}
	if !tt.HasText("Rivers of Europe") || tt.HasText("Linear Algebra") || tt.HasText("最近完成") {
		t.Fatalf("searching RIVERS shows %q", tt.Texts())
	}
	if !tt.HasText("Morning Market") || !tt.HasText("1 部") {
		t.Fatalf("searching RIVERS left 处理中 or the count out: %q", tt.Texts())
	}
	listed := films.ByTitle(a.films(), a.side.query)
	if len(listed) != 1 || listed[0].Name != "Rivers of Europe" {
		t.Fatalf("films.ByTitle(RIVERS) = %+v", listed)
	}
	// The sidebar's sources go on counting the whole library, and a source narrows the search further.
	if n := sidebarCount(t, tt, "YouTube"); n != "3" {
		t.Fatalf("YouTube counts %q while searching", n)
	}
	a.SetSourceFilter(sidebar.SourceLocal)
	tt.Frame()
	if !tt.HasText("这个来源下还没有影片。") || a.side.query != "RIVERS" {
		t.Fatalf("本地文件 while searching: query %q, %q", a.side.query, tt.Texts())
	}
	a.SetSourceFilter(sidebar.SourceAll)

	a.side.query = "ocean"
	tt.Frame()
	if !tt.HasText("没有名称包含「ocean」的已完成影片。") || tt.HasText("Rivers of Europe") || !tt.HasText("Morning Market") {
		t.Fatalf("searching ocean shows %q", tt.Texts())
	}
	if err := tt.Click("清除搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.side.query != "" || !tt.HasText("最近完成") || !tt.HasText("Linear Algebra") || tt.HasText("清除搜索") {
		t.Fatalf("清除搜索 left query %q: %q", a.side.query, tt.Texts())
	}
}

// libraryForSort is a library of three finished films, the last added first as the shelf keeps them: Mango from Bilibili, apple from YouTube and Zebra from YouTube. Its settings are written to a folder of the test's.
func libraryForSort(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	lib := filepath.Join(t.TempDir(), "我的影片.kando")
	a, tt, _ := newTestApp(&fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) { s.LibraryDir = lib })
	for _, film := range []struct{ name, source string }{
		{"Mango", "https://www.bilibili.com/video/BV1GJ411x7h7"},
		{"apple", "https://youtu.be/TRWgBxVAb1M"},
		{"Zebra", "https://www.youtube.com/watch?v=k9Yt2LmQ0pX"},
	} {
		video := filepath.Join(lib, film.name, "video.mp4")
		if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
		a.library.Jobs = append(a.library.Jobs, core.Job{ID: film.name, Source: film.source, Language: "简体中文", Stage: core.StageComplete, VideoPath: video, LibraryDir: lib})
	}
	a.refreshLibrary()
	tt.Frame()
	return a, tt
}

// libraryGridOrder is the names of the films in the done grid, as it shows them: the names after its heading's count, each once, though a card shows its name both on its poster's label and under it.
func libraryGridOrder(t *testing.T, tt *ui.Tester, names ...string) string {
	t.Helper()
	_, grid := libraryNamesAroundGrid(t, tt, names...)
	return grid
}

// libraryHeroOrder is the names of the films drawn above the done grid's heading, each once: with nothing processing, that is only the hero's film.
func libraryHeroOrder(t *testing.T, tt *ui.Tester, names ...string) string {
	t.Helper()
	above, _ := libraryNamesAroundGrid(t, tt, names...)
	return above
}

// libraryNamesAroundGrid splits the names of the films the page shows at the done grid's heading: those above it and those in the grid, each list in the order drawn and each name once.
func libraryNamesAroundGrid(t *testing.T, tt *ui.Tester, names ...string) (above, grid string) {
	t.Helper()
	texts := tt.Texts()
	at := slices.Index(texts, locales.FilmsGridCount(len(names)))
	if at < 0 {
		t.Fatalf("no grid heading; texts %q", texts)
	}
	distinct := func(texts []string) string {
		var order []string
		for _, text := range texts {
			if slices.Contains(names, text) && !slices.Contains(order, text) {
				order = append(order, text)
			}
		}
		return strings.Join(order, ",")
	}
	return distinct(texts[:at]), distinct(texts[at+1:])
}

// TestLibrarySortOrdersTheGrid chooses each order from the button beside the grid's heading: the grid follows it, the hero stays the film finished last, and the order is kept in settings.json for the next launch.
func TestLibrarySortOrdersTheGrid(t *testing.T) {
	a, tt := libraryForSort(t)
	names := []string{"Mango", "apple", "Zebra"}
	if got := libraryGridOrder(t, tt, names...); got != "Mango,apple,Zebra" {
		t.Fatalf("recently added %s", got)
	}
	choose := func(button, item string) {
		t.Helper()
		if err := tt.Click(button); err != nil {
			t.Fatalf("%v; texts %q", err, tt.Texts())
		}
		if got, want := tt.Menu(), []string{"最近添加", "标题", "来源"}; !slices.Equal(got, want) {
			t.Fatalf("sort menu %q, want %q", got, want)
		}
		if err := tt.ChooseMenuItem(item); err != nil {
			t.Fatalf("%v; menu %q", err, tt.Menu())
		}
		tt.Frame()
	}
	choose("排序：最近添加", "标题")
	if got := libraryGridOrder(t, tt, names...); got != "apple,Mango,Zebra" {
		t.Fatalf("by title %s", got)
	}
	if a.LibrarySort() != films.SortTitle || !tt.HasText("排序：标题") {
		t.Fatalf("sort %q; texts %q", a.LibrarySort(), tt.Texts())
	}
	if got := libraryHeroOrder(t, tt, names...); got != "Mango" {
		t.Fatalf("by title the hero shows %q, not the film finished last", got)
	}
	if saved := loadSavedSettings(a.settingsPath); saved.LibrarySort != films.SortTitle {
		t.Fatalf("saved sort %q", saved.LibrarySort)
	}

	choose("排序：标题", "来源")
	if got := libraryGridOrder(t, tt, names...); got != "apple,Zebra,Mango" {
		t.Fatalf("by source %s", got)
	}
	if got := libraryHeroOrder(t, tt, names...); got != "Mango" {
		t.Fatalf("by source the hero shows %q, not the film finished last", got)
	}

	// The next launch shows the grid in the order chosen last.
	b, btt, _ := newTestApp(&fakePipeline{})
	b.restoreSettings(loadSavedSettings(a.settingsPath))
	b.library.Jobs = a.library.Jobs
	b.refreshLibrary()
	btt.Frame()
	if got := libraryGridOrder(t, btt, names...); b.LibrarySort() != films.SortSource || got != "apple,Zebra,Mango" {
		t.Fatalf("restored sort %q shows %s", b.LibrarySort(), got)
	}

	choose("排序：来源", "最近添加")
	if got := libraryGridOrder(t, tt, names...); got != "Mango,apple,Zebra" {
		t.Fatalf("back to recently added %s", got)
	}
	if saved := loadSavedSettings(a.settingsPath); saved.LibrarySort != films.SortAdded {
		t.Fatalf("saved sort %q", saved.LibrarySort)
	}
}

// TestLibrarySortReadsAnUnknownOrderAsRecentlyAdded: an order settings.json holds that Kando does not know, as one another version saved, shows the films as they were added.
func TestLibrarySortReadsAnUnknownOrderAsRecentlyAdded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"librarySort": "duration"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	saved := loadSavedSettings(path)
	if saved.LibrarySort != films.SortAdded {
		t.Fatalf("read %q", saved.LibrarySort)
	}
	a, _, _ := newTestApp(&fakePipeline{})
	a.restoreSettings(savedSettings{savedLibrary: savedLibrary{LibrarySort: "duration"}})
	if a.LibrarySort() != films.SortAdded {
		t.Fatalf("restored %q", a.LibrarySort())
	}
}

// TestLibrarySortSaysWhenItCannotBeKept: the grid takes the order even when settings.json cannot be written, and the message bar says it will not be kept.
func TestLibrarySortSaysWhenItCannotBeKept(t *testing.T) {
	a, tt := libraryForSort(t)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	a.settingsPath = filepath.Join(blocker, "settings.json")
	if err := tt.Click("排序：最近添加"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("标题"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Frame()
	if a.LibrarySort() != films.SortTitle || !tt.HasText("排序方式无法保存") {
		t.Fatalf("sort %q; texts %q", a.LibrarySort(), tt.Texts())
	}
}

// TestLibrarySortHidesWithASingleFilm: one film has no order to choose, so the grid's heading has no sort button.
func TestLibrarySortHidesWithASingleFilm(t *testing.T) {
	_, tt, _ := libraryWith(t, "Talk")
	if !tt.HasText("1 部") || tt.HasText("排序：最近添加") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// renamedLibrary is libraryWith with library.json kept in a file of the test's, so what renaming saves can be read back.
func renamedLibrary(t *testing.T, name string) (a *app, tt *ui.Tester, video, libraryPath string) {
	t.Helper()
	a, tt, video = libraryWith(t, name)
	jobs := a.library.Jobs
	libraryPath = filepath.Join(t.TempDir(), "library.json")
	a.openLibrary(libraryPath)
	a.library.Jobs = jobs
	if err := a.library.Save(); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	return a, tt, video, libraryPath
}

// renameFilm opens 重命名… from the poster's ⋯, puts title in the field in place of what it holds, and presses Enter.
func renameFilm(t *testing.T, a *app, tt *ui.Tester, title string) {
	t.Helper()
	if err := tt.Click("更多操作"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	if err := tt.ChooseMenuItem("重命名…"); err != nil {
		t.Fatalf("%v menu %q", err, tt.Menu())
	}
	tt.Frame()
	if !tt.HasText("重命名影片") || !a.modalOpen() {
		t.Fatalf("no rename dialog, or it does not hold the window (modal %v): %q", a.modalOpen(), tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type(title)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	if tt.HasText("重命名影片") || a.modalOpen() {
		t.Fatalf("the rename dialog stays open after Enter (modal %v): %q", a.modalOpen(), tt.Texts())
	}
}

// TestRenameGivesAFilmTheTitleKandoShows checks that 重命名… opens with the title the film shows, that the new title shows on the shelf and in the player and is kept in library.json, that nothing on disk is renamed, and that an empty title gives the film its own name back.
func TestRenameGivesAFilmTheTitleKandoShows(t *testing.T) {
	a, tt, video, libraryPath := renamedLibrary(t, "Talk")
	if err := tt.Click("更多操作"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("重命名…"); err != nil {
		t.Fatalf("%v menu %q", err, tt.Menu())
	}
	tt.Frame()
	if a.lib.renameName != "Talk" || a.lib.renameOriginal != "Talk" {
		t.Fatalf("the dialog opens with %q (original %q), want the film's title", a.lib.renameName, a.lib.renameOriginal)
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if len(a.library.Names) != 0 || a.modalOpen() {
		t.Fatalf("取消 renamed the film (%v) or still holds the window (modal %v)", a.library.Names, a.modalOpen())
	}
	if err := tt.Click("更多操作"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("重命名…"); err != nil {
		t.Fatalf("%v menu %q", err, tt.Menu())
	}
	tt.Frame()
	tt.Type("Escaped")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Frame()
	if tt.HasText("重命名影片") || len(a.library.Names) != 0 || a.modalOpen() {
		t.Fatalf("Escape renamed the film (%v), or left the dialog open or holding the window (modal %v): %q", a.library.Names, a.modalOpen(), tt.Texts())
	}

	renameFilm(t, a, tt, "  我的演讲  ")
	if _, ok := tt.Find("我的演讲"); !ok {
		t.Fatalf("the shelf does not show the new title: %q", tt.Texts())
	}
	if got := a.films()[0]; got.Name != "我的演讲" || got.Title != "我的演讲" {
		t.Fatalf("film %+v", got)
	}
	if job := a.remoteJobOf(a.library.Jobs[0]); job.Title != "我的演讲" {
		t.Fatalf("the socket names the job %q", job.Title)
	}
	lib, err := shelf.Load(libraryPath)
	if err != nil {
		t.Fatal(err)
	}
	if lib.Names[video] != "我的演讲" || len(lib.Names) != 1 {
		t.Fatalf("library.json names %v", lib.Names)
	}
	if _, err := os.Stat(video); err != nil {
		t.Fatalf("renaming touched the video on disk: %v", err)
	}

	a.Open(a.films()[0])
	tt.Frame()
	if !tt.HasText("我的演讲") {
		t.Fatalf("the player does not show the new title: %q", tt.Texts())
	}
	a.openKey = ""
	a.stopPlayback()
	tt.Frame()

	renameFilm(t, a, tt, "")
	if len(a.library.Names) != 0 {
		t.Fatalf("an empty title kept %v", a.library.Names)
	}
	if _, ok := tt.Find("Talk"); !ok {
		t.Fatalf("an empty title did not give the film its name back: %q", tt.Texts())
	}
	if lib, _ := shelf.Load(libraryPath); len(lib.Names) != 0 {
		t.Fatalf("library.json still names %v", lib.Names)
	}
}

// TestRenameTypingTheOriginalTitleKeepsNone checks that typing the title the film has anyway gives it no title of its own, so it keeps following its site's title.
func TestRenameTypingTheOriginalTitleKeepsNone(t *testing.T) {
	a, tt, _, _ := renamedLibrary(t, "Talk")
	renameFilm(t, a, tt, "Talk")
	if len(a.library.Names) != 0 {
		t.Fatalf("names %v", a.library.Names)
	}
}

// TestRenameSurvivesReprocessingAndAFinderRename checks that the title given in Kando stays over the title the site gives when the film is processed again, and stays with the film when its folder is renamed in Finder: Finder's name does not replace it.
func TestRenameSurvivesReprocessingAndAFinderRename(t *testing.T) {
	a, tt, video, libraryPath := renamedLibrary(t, "Talk")
	renameFilm(t, a, tt, "我的演讲")

	// Processing it again looks the video up, and the site's title comes back on the job.
	job := a.library.Jobs[0]
	job.Title = "Rust in 100 Seconds"
	a.show(job)
	tt.Frame()
	if got := a.films()[0].Name; got != "我的演讲" {
		t.Fatalf("after the site's title came back the film is named %q", got)
	}

	renamed := filepath.Join(filepath.Dir(filepath.Dir(video)), "Finder name")
	if err := os.Rename(filepath.Dir(video), renamed); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	tt.Frame()
	moved := filepath.Join(renamed, "video.mp4")
	if a.library.Jobs[0].VideoPath != moved {
		t.Fatalf("the job did not follow its folder: %q", a.library.Jobs[0].VideoPath)
	}
	if got := a.films(); len(got) != 1 || got[0].Name != "我的演讲" {
		t.Fatalf("after the folder was renamed in Finder: %+v", got)
	}
	lib, err := shelf.Load(libraryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Names) != 1 || lib.Names[moved] != "我的演讲" {
		t.Fatalf("library.json names %v", lib.Names)
	}
}

// TestRenameFollowsAReprocessToANewFolder checks that processing a renamed film again into another folder carries its title to the new video, on the shelf and in library.json.
func TestRenameFollowsAReprocessToANewFolder(t *testing.T) {
	a, tt, video, libraryPath := renamedLibrary(t, "Talk")
	renameFilm(t, a, tt, "我的演讲")

	next := filepath.Join(filepath.Dir(filepath.Dir(video)), "Rust in 100 Seconds [TRWgBxVAb1M]", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(next), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(next, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	job := a.library.Jobs[0]
	job.Title = "Rust in 100 Seconds"
	job.VideoPath = next
	a.show(job)
	tt.Frame()
	if got := a.films(); len(got) == 0 || got[0].VideoPath != next || got[0].Name != "我的演讲" {
		t.Fatalf("after processing again into a new folder: %+v", got)
	}
	lib, err := shelf.Load(libraryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Names) != 1 || lib.Names[next] != "我的演讲" {
		t.Fatalf("library.json names %v", lib.Names)
	}
}

// TestRenameKeepsTheVideoTheWatcherMoved checks that a title typed while the library watcher moves the film's video is kept for where the video is now.
func TestRenameKeepsTheVideoTheWatcherMoved(t *testing.T) {
	a, tt, video, _ := renamedLibrary(t, "Talk")
	if err := tt.Click("更多操作"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("重命名…"); err != nil {
		t.Fatalf("%v menu %q", err, tt.Menu())
	}
	tt.Frame()
	renamed := filepath.Join(filepath.Dir(filepath.Dir(video)), "Finder name")
	if err := os.Rename(filepath.Dir(video), renamed); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("我的演讲")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	moved := filepath.Join(renamed, "video.mp4")
	if len(a.library.Names) != 1 || a.library.Names[moved] != "我的演讲" {
		t.Fatalf("names %v, want the title kept for %q", a.library.Names, moved)
	}
}

// TestRenameDropsTheTitleOfAVideoDeletedInFinder checks that deleting a renamed film's folder in Finder forgets its title, for a downloaded film and for a video no job processed, so a video that turns up there later does not take it.
func TestRenameDropsTheTitleOfAVideoDeletedInFinder(t *testing.T) {
	for _, loose := range []bool{false, true} {
		a, tt, video, libraryPath := renamedLibrary(t, "Talk")
		if loose {
			a.library.Jobs = nil
			if err := a.library.Save(); err != nil {
				t.Fatal(err)
			}
			a.refreshLibrary()
			tt.Frame()
		}
		renameFilm(t, a, tt, "我的演讲")
		if err := os.RemoveAll(filepath.Dir(video)); err != nil {
			t.Fatal(err)
		}
		a.refreshLibrary()
		tt.Frame()
		if len(a.library.Names) != 0 || len(a.library.Jobs) != 0 {
			t.Fatalf("loose %v: names %v jobs %d after the folder was deleted", loose, a.library.Names, len(a.library.Jobs))
		}
		if lib, _ := shelf.Load(libraryPath); len(lib.Names) != 0 {
			t.Fatalf("loose %v: library.json keeps %v", loose, lib.Names)
		}
	}
}

// TestRenameFollowsAFinderRenameOfAVideoNoJobProcessed checks that the title given to a video no job processed stays with it when its folder is renamed in Finder, also beside another video the shelf already listed.
func TestRenameFollowsAFinderRenameOfAVideoNoJobProcessed(t *testing.T) {
	a, tt, video, libraryPath := renamedLibrary(t, "Talk")
	other := filepath.Join(filepath.Dir(filepath.Dir(video)), "Other", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.library.Jobs = nil
	if err := a.library.Save(); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	tt.Frame()
	a.AskRename(films.Film{VideoPath: video, Name: "Talk"})
	a.lib.renameName = "Loose talk"
	a.RenameFilm()
	tt.Frame()
	if a.library.Names[video] != "Loose talk" {
		t.Fatalf("names %v", a.library.Names)
	}

	renamed := filepath.Join(filepath.Dir(filepath.Dir(video)), "Finder name")
	if err := os.Rename(filepath.Dir(video), renamed); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	tt.Frame()
	moved := filepath.Join(renamed, "video.mp4")
	if len(a.library.Names) != 1 || a.library.Names[moved] != "Loose talk" {
		t.Fatalf("the title did not follow the renamed folder: %v", a.library.Names)
	}
	var names []string
	for _, film := range a.films() {
		names = append(names, film.Name)
	}
	if !slices.Contains(names, "Loose talk") || slices.Contains(names, "Finder name") {
		t.Fatalf("films %q", names)
	}
	if lib, _ := shelf.Load(libraryPath); len(lib.Names) != 1 || lib.Names[moved] != "Loose talk" {
		t.Fatalf("library.json names %v", lib.Names)
	}
}

// TestRenameAVideoNoJobProcessed checks that a video found in the library folder can be renamed too, and that moving it to the Trash forgets its title.
func TestRenameAVideoNoJobProcessed(t *testing.T) {
	a, tt, video, libraryPath := renamedLibrary(t, "Talk")
	a.library.Jobs = nil
	if err := a.library.Save(); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	tt.Frame()
	if got := a.films(); len(got) != 1 || got[0].JobID != "" {
		t.Fatalf("films %+v", got)
	}
	renameFilm(t, a, tt, "Loose talk")
	if got := a.films()[0]; got.Name != "Loose talk" || a.library.Names[video] != "Loose talk" {
		t.Fatalf("film %+v names %v", got, a.library.Names)
	}
	if a.lib.renameOriginal != "Talk" {
		t.Fatalf("the original title of a loose video is %q", a.lib.renameOriginal)
	}

	fakeTrash(t)
	a.TrashFilm(a.films()[0])
	if len(a.library.Names) != 0 {
		t.Fatalf("the trashed video keeps its title: %v", a.library.Names)
	}
	if lib, _ := shelf.Load(libraryPath); len(lib.Names) != 0 {
		t.Fatalf("library.json keeps %v", lib.Names)
	}
}
