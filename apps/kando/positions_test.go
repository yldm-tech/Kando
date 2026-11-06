package main

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/packages/core"

	"github.com/egoist/mygo/ui"
)

// TestResumePositionsSurviveARestart checks that where a video stopped is saved with the shelf and read back when the shelf opens again, and that a video watched to the end is forgotten on disk too.
func TestResumePositionsSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(&fakePipeline{})
	a.openLibrary(path)
	a.rememberPosition(&filmPlay{path: "/v.mp4", duration: 10 * time.Second, frames: &frameDecoder{shown: 3 * time.Second}})
	a.rememberPosition(&filmPlay{path: "/w.mp4", duration: 10 * time.Second, frames: &frameDecoder{shown: 7 * time.Second}})

	again, _, _ := newTestApp(&fakePipeline{})
	again.openLibrary(path)
	if at := again.ResumeAt("/v.mp4"); at != 3*time.Second {
		t.Fatalf("ResumeAt(/v.mp4) after a restart = %v", at)
	}
	if at := again.ResumeAt("/w.mp4"); at != 7*time.Second {
		t.Fatalf("ResumeAt(/w.mp4) after a restart = %v", at)
	}

	again.rememberPosition(&filmPlay{path: "/v.mp4", duration: 10 * time.Second, finished: true})
	third, _, _ := newTestApp(&fakePipeline{})
	third.openLibrary(path)
	if at := third.ResumeAt("/v.mp4"); at != 0 {
		t.Fatalf("a finished video resumes at %v", at)
	}
	if at := third.ResumeAt("/w.mp4"); at != 7*time.Second {
		t.Fatalf("ResumeAt(/w.mp4) = %v", at)
	}
}

// stillFrames is a picture that shows one black frame over and over, for a player a test opens without ffmpeg.
type stillFrames struct{}

func (stillFrames) Next() (ui.VideoFrame, error) {
	return ui.VideoFrame{Image: image.NewRGBA(image.Rect(0, 0, 2, 2)), Duration: time.Second}, nil
}

func (stillFrames) Rewind() error { return nil }

// openStill makes the film page show job id with its picture at, as if it had played that far, in a library saved at path.
func openStill(t *testing.T, path, id, video string, at time.Duration) *app {
	t.Helper()
	a, _, _ := newTestApp(&fakePipeline{})
	a.openLibrary(path)
	a.library.Jobs = []core.Job{{ID: id, Source: "file://" + video, Language: "简体中文", Stage: core.StageComplete, VideoPath: video}}
	player, err := ui.NewVideoPlayer(stillFrames{})
	if err != nil {
		t.Fatal(err)
	}
	a.openKey = id
	a.player.play = &filmPlay{player: player, frames: &frameDecoder{shown: at}, path: video, duration: 10 * time.Second}
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
		again, _, _ := newTestApp(&fakePipeline{})
		again.openLibrary(path)
		if at := again.ResumeAt("/v.mp4"); at != 3*time.Second {
			t.Fatalf("%s: ResumeAt(/v.mp4) on disk = %v", name, at)
		}
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
	a, _, _ := newTestApp(&fakePipeline{})
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
