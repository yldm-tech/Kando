package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/shelf"
	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
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
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != "file://"+src || !tt.HasText("已加入，开始处理。") {
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
