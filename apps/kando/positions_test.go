package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/packages/core"
)

// TestResumePositionsSurviveARestart checks that where a video stopped is saved with the shelf and read back when the shelf opens again, and that a video watched to the end is forgotten on disk too.
func TestResumePositionsSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	a.rememberPosition(&filmPlay{path: "/v.mp4", loaded: true, duration: 10 * time.Second, head: 3 * time.Second})
	a.rememberPosition(&filmPlay{path: "/w.mp4", loaded: true, duration: 10 * time.Second, head: 7 * time.Second})

	again, _, _ := newTestApp(t, &fakePipeline{})
	again.openLibrary(path)
	if at := again.ResumeAt("/v.mp4"); at != 3*time.Second {
		t.Fatalf("ResumeAt(/v.mp4) after a restart = %v", at)
	}
	if at := again.ResumeAt("/w.mp4"); at != 7*time.Second {
		t.Fatalf("ResumeAt(/w.mp4) after a restart = %v", at)
	}

	again.rememberPosition(&filmPlay{path: "/v.mp4", loaded: true, duration: 10 * time.Second, finished: true})
	third, _, _ := newTestApp(t, &fakePipeline{})
	third.openLibrary(path)
	if at := third.ResumeAt("/v.mp4"); at != 0 {
		t.Fatalf("a finished video resumes at %v", at)
	}
	if at := third.ResumeAt("/w.mp4"); at != 7*time.Second {
		t.Fatalf("ResumeAt(/w.mp4) = %v", at)
	}
}

// TestPlayedTimesSurviveARestart checks that when a video last played is kept as its position is, a video watched to the end included, saved with the shelf and read back when it opens again, and that a library.json written before it was kept reads with no time.
func TestPlayedTimesSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	if !a.PlayedAt("/v.mp4").IsZero() {
		t.Fatalf("a video never played played at %v", a.PlayedAt("/v.mp4"))
	}
	a.rememberPosition(&filmPlay{path: "/v.mp4", loaded: true, duration: 10 * time.Second, head: 3 * time.Second})
	first := a.PlayedAt("/v.mp4")
	if first.IsZero() {
		t.Fatal("rememberPosition kept no time")
	}
	tt.Advance(time.Hour)
	a.rememberPosition(&filmPlay{path: "/w.mp4", loaded: true, duration: 10 * time.Second, finished: true})
	if got := a.PlayedAt("/w.mp4"); got.Sub(first) != time.Hour {
		t.Fatalf("a video watched to the end played at %v, an hour after %v", got, first)
	}

	again, _, _ := newTestApp(t, &fakePipeline{})
	again.openLibrary(path)
	if got := again.PlayedAt("/v.mp4"); !got.Equal(first) {
		t.Fatalf("PlayedAt(/v.mp4) after a restart = %v, want %v", got, first)
	}
	if got := again.PlayedAt("/w.mp4"); !got.Equal(first.Add(time.Hour)) {
		t.Fatalf("PlayedAt(/w.mp4) after a restart = %v", got)
	}
}

// openStill makes the film page show job id with the system's player loaded and held at, as if it had played that far, in a library saved at path.
func openStill(t *testing.T, path, id, video string, at time.Duration) *app {
	t.Helper()
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	a.library.Jobs = []core.Job{{ID: id, Source: core.FileURL(video), Language: "简体中文", Stage: core.StageComplete, VideoPath: video}}
	a.openKey = id
	a.player.playPath = video
	a.player.play = &filmPlay{path: video, source: video, loaded: true, head: at, duration: 10 * time.Second, paused: true, played: true, clock: a.playerNow}
	t.Cleanup(a.stopPlayback)
	return a
}

// TestEventsThatTakeTheFilmAwayCloseIt checks that importing another video and removing the open film close its picture and save where it stopped themselves, with no frame drawn, so the library page never has to do that disk work while it is built.
func TestEventsThatTakeTheFilmAwayCloseIt(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "other.mp4")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, event := range map[string]func(a *app){
		"drop":   func(a *app) { a.dropFiles([]string{other}) },
		"remove": func(a *app) { a.remove("clip") },
	} {
		path := filepath.Join(t.TempDir(), "library.json")
		a := openStill(t, path, "clip", "/v.mp4", 3*time.Second)
		event(a)
		if a.openKey != "" || a.PlaybackOpen() {
			t.Fatalf("%s: open %q playback %v", name, a.openKey, a.PlaybackOpen())
		}
		again, _, _ := newTestApp(t, &fakePipeline{})
		again.openLibrary(path)
		if at := again.ResumeAt("/v.mp4"); at != 3*time.Second {
			t.Fatalf("%s: ResumeAt(/v.mp4) on disk = %v", name, at)
		}
	}
}

// TestResumePositionFollowsARenamedLibrary checks that a video of a library renamed in Finder still resumes where it stopped, in memory and in library.json.
func TestResumePositionFollowsARenamedLibrary(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "影视库.kando")
	video := filepath.Join(dir, "talk", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{ID: "talk", Source: core.FileURL(video), Stage: core.StageComplete, VideoPath: video, LibraryDir: dir}}
	a.setResumePositions(map[string]time.Duration{video: 3 * time.Second})
	a.savePositions()
	next := filepath.Join(parent, "看过.kando")
	if err := os.Rename(dir, next); err != nil {
		t.Fatal(err)
	}
	a.renameLibraryPath(dir, next)
	moved := filepath.Join(next, "talk", "video.mp4")
	if at := a.ResumeAt(moved); at != 3*time.Second {
		t.Fatalf("ResumeAt(%s) = %v, positions %v", moved, at, a.resumePositions())
	}
	again, _, _ := newTestApp(t, &fakePipeline{})
	again.openLibrary(path)
	if at := again.ResumeAt(moved); at != 3*time.Second {
		t.Fatalf("ResumeAt on disk = %v", at)
	}
}

// TestStartFilmKeepsThePicture checks that 开始处理 on an open library video keeps its picture playing while the page moves to the video's new job.
func TestStartFilmKeepsThePicture(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "talk.mp4")
	if err := os.WriteFile(video, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := openStill(t, filepath.Join(t.TempDir(), "library.json"), video, video, 3*time.Second)
	a.library.Jobs = nil
	// The video is on the shelf, as a library video is, so the player keeps its film open while the shelf changes.
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.refreshLibrary()
	play := a.player.play
	a.startFilm(films.Film{Key: video, VideoPath: video})
	if a.queue.job == nil || a.openKey != a.queue.job.ID || a.player.play != play {
		t.Fatalf("job %+v open %q picture kept %v", a.queue.job, a.openKey, a.player.play == play)
	}
}

// TestRefreshClosesAVideoThatLeftTheLibrary checks that when the open library video is gone from disk, the library refresh that notices closes its film, rather than the page while it is built.
func TestRefreshClosesAVideoThatLeftTheLibrary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "talk.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.refreshLibrary()
	videos := a.snap().Videos
	if len(videos) != 1 {
		t.Fatalf("videos %+v", videos)
	}
	a.openKey = videos[0].Path
	a.player.playPath = videos[0].Path
	a.refreshLibrary()
	if a.openKey != videos[0].Path || !a.PlaybackOpen() {
		t.Fatalf("a refresh with the video still there closed it: open %q", a.openKey)
	}
	if err := os.Remove(videos[0].Path); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	if a.openKey != "" || a.PlaybackOpen() {
		t.Fatalf("open %q playback %v after the video left", a.openKey, a.PlaybackOpen())
	}
}
