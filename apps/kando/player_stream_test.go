package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// streamAddress is the address of the stream the fake yt-dlp of streamTools gives.
const streamAddress = "https://manifest.googlevideo.com/api/manifest/hls_playlist/id/1/index.m3u8"

// streamTools are tools whose yt-dlp gives streamAddress as the stream of any video, or fails when broken.
func streamTools(t *testing.T, broken bool) media.Tools {
	body := "echo " + streamAddress
	if broken {
		body = "echo 'ERROR: [youtube] jNQXAC9IVRw: Requested format is not available' >&2\nexit 1"
	}
	return media.Tools{YTDLP: writeFakeScript(t, t.TempDir(), "yt-dlp", body)}
}

// videoPipeline downloads a film's video into a folder named after the video's new title, as yt-dlp does once a site renamed it, or fails with err.
type videoPipeline struct {
	fakePipeline
	t    *testing.T
	root string
	err  error
}

func (p *videoPipeline) Download(context.Context, string) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	video := filepath.Join(p.root, "Talk, renamed [jNQXAC9IVRw]", "video.mp4")
	writeFakeClip(p.t, video, 12, "202020", "101010")
	return video, nil
}

// streamFilm starts the app with a done YouTube film that keeps no video in its library: its subtitles beside the path its sound was downloaded to, which is gone. streams says whether it streams on this computer.
func streamFilm(t *testing.T, p media.Pipeline, streams bool, opts ...nsgotest.Option) (*app, *nsgotest.App, chan func(), films.Film) {
	t.Helper()
	dir := t.TempDir()
	folder := filepath.Join(dir, "Talk [jNQXAC9IVRw]")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	srt := filepath.Join(folder, "video.bilingual.srt")
	if err := os.WriteFile(srt, []byte(talkSubtitles), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(t, p, opts...)
	a.player.streams = func(string) bool { return streams }
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{ID: "talk", Title: "Talk", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Language: "简体中文", Stage: core.StageComplete,
		VideoPath: filepath.Join(folder, "video.webm"), SubtitlePath: srt, LibraryDir: dir, Quality: core.QualitySubtitles, SubtitlesOnly: true}}
	refresh(tt, a)
	item, ok := films.Find(a.films(), "talk")
	if !ok {
		t.Fatalf("the film is not on the shelf: %+v", a.films())
	}
	return a, tt, updates, item
}

// TestSubtitlesOnlyFilmStreams checks that a film that keeps no video plays its site's stream, looked up as it opens, with its subtitles, and stays on the shelf though its sound is gone.
func TestSubtitlesOnlyFilmStreams(t *testing.T) {
	a, tt, updates, item := streamFilm(t, streamTools(t, false), true)
	a.refreshLibrary()
	if _, ok := films.Find(a.films(), "talk"); !ok {
		t.Fatal("the film left the shelf without its sound")
	}
	playerOpenFilm(t, a, tt, updates, item)
	wantText(t, tt, testlocale.Messages().FilmOpeningVideo())
	waitUntil(t, tt, a, updates, func() bool { return a.player.play != nil && a.player.play.source != "" })
	if got := playerVideoConfig(tt).Source; got != streamAddress {
		t.Fatalf("the player plays %q", got)
	}
	if a.player.play.path != item.VideoPath || tt.Exists("id:film-stream") || !a.player.subs.ready {
		t.Fatalf("play %+v, subtitles ready %v", a.player.play, a.player.subs.ready)
	}
	playerLoadFilm(tt, a, 12*time.Second)
	if a.Play() == nil {
		t.Fatal("the stream did not load")
	}

	// A stream the system's player cannot play says so, and is not copied as a file would be.
	apply(tt, a, func() { a.VideoFailed(errors.New("cannot open")) })
	if a.player.playErr != film.ErrStreamFailed || a.player.play.remux != 0 || !tt.Exists("id:film-stream-retry") {
		t.Fatalf("error %q, remux %v", a.player.playErr, a.player.play.remux)
	}
	tt.Press("id:film-stream-retry")
	waitUntil(t, tt, a, updates, func() bool { return a.player.play != nil && a.player.play.source == streamAddress })
	if a.player.playErr != "" || tt.Exists("id:film-stream") {
		t.Fatalf("after trying again: error %q", a.player.playErr)
	}
}

// TestSubtitlesOnlyFilmStreamThatFails checks the card of a stream yt-dlp cannot give: it offers to try again and to download the video.
func TestSubtitlesOnlyFilmStreamThatFails(t *testing.T) {
	a, tt, updates, item := streamFilm(t, streamTools(t, true), true)
	playerOpenFilm(t, a, tt, updates, item)
	waitUntil(t, tt, a, updates, func() bool { return a.player.playErr != "" })
	msgs := testlocale.Messages()
	if a.player.playErr != film.ErrStreamFailed || !hasText(tt, msgs.PlayerStreamFailed()) || !tt.Exists("id:film-stream-download") {
		t.Fatalf("error %q, texts %q", a.player.playErr, tt.Texts())
	}
}

// TestSubtitlesOnlyFilmOffline checks a film that keeps no video opened offline: the card says it cannot play offline, its download waits until offline mode is off and says it is scheduled, and turning offline mode off from the card streams it.
func TestSubtitlesOnlyFilmOffline(t *testing.T) {
	a, tt, updates, item := streamFilm(t, streamTools(t, false), true)
	apply(tt, a, func() { a.privacySetOffline(true) })
	playerOpenFilm(t, a, tt, updates, item)
	msgs := testlocale.Messages()
	if a.player.playErr != film.ErrStreamOffline || !hasText(tt, msgs.PlayerStreamOffline()) || !hasText(tt, msgs.PlayerStreamOfflineDetail("YouTube")) {
		t.Fatalf("error %q, texts %q", a.player.playErr, tt.Texts())
	}
	if a.player.play != nil || hasText(tt, msgs.FilmOpeningVideo()) {
		t.Fatal("the film opens a stream offline")
	}
	tt.Press("id:film-stream-download")
	job, _ := a.library.Find("talk")
	if job.Redo != core.RedoVideo || job.Stage != core.StageQueued || job.Quality != core.QualityBest || a.queue.active != "" {
		t.Fatalf("job %+v, running %q", job, a.queue.active)
	}
	if got := tt.Find("id:film-stream-download"); got.Props.Label != msgs.PlayerStreamQueued() || tt.Enabled("id:film-stream-download") {
		t.Fatalf("the download button once scheduled: %+v", got.Props)
	}
	// The film is still the done film it was while its video waits.
	if f, _ := films.Find(a.films(), "talk"); !f.SubtitlesOnly || !f.VideoComing {
		t.Fatalf("film %+v", f)
	}

	// Its download would start once offline mode is off, which this test does not want: the film's job is taken back, and the card's other button streams the film.
	apply(tt, a, func() {
		job.Redo, job.Stage = "", core.StageComplete
		a.library.Upsert(job)
	})
	tt.Press("id:film-stream-online")
	if a.privacy.offline {
		t.Fatal("offline mode stayed on")
	}
	waitUntil(t, tt, a, updates, func() bool { return a.player.play != nil && a.player.play.source == streamAddress })
}

// TestSubtitlesOnlyFilmDownloadsItsVideo checks a film that keeps no video where it cannot stream: the card says so, and its download puts the video beside the subtitles, after which the film plays it as a downloaded film.
func TestSubtitlesOnlyFilmDownloadsItsVideo(t *testing.T) {
	p := &videoPipeline{t: t}
	a, tt, updates, item := streamFilm(t, p, false)
	p.root = a.snap().LibraryDir
	playerOpenFilm(t, a, tt, updates, item)
	msgs := testlocale.Messages()
	if a.player.playErr != film.ErrStreamNone || !hasText(tt, msgs.PlayerStreamNone()) || tt.Exists("id:film-stream-online") {
		t.Fatalf("error %q, texts %q", a.player.playErr, tt.Texts())
	}
	// Where the film was left while it streamed is kept by the path its sound was downloaded to.
	apply(tt, a, func() { a.setResumePositions(map[string]time.Duration{item.VideoPath: 5 * time.Second}) })
	tt.Press("id:film-stream-download")
	settle(t, a, tt, updates)
	job, _ := a.library.Find("talk")
	want := filepath.Join(filepath.Dir(item.VideoPath), "video.mp4")
	// The video has the download's extension, and the film's place follows it there.
	if a.ResumeAt(want) != 5*time.Second || a.ResumeAt(item.VideoPath) != 0 {
		t.Fatalf("place %v at the video, %v at the sound", a.ResumeAt(want), a.ResumeAt(item.VideoPath))
	}
	// The open film plays its video once it is downloaded.
	if a.player.playErr != "" || tt.Exists("id:film-stream") || playerVideoConfig(tt).Source != want {
		t.Fatalf("error %q after the download", a.player.playErr)
	}
	if job.Stage != core.StageComplete || job.SubtitlesOnly || job.Redo != "" || job.VideoPath != want || job.SubtitlePath != item.SubtitlePath {
		t.Fatalf("job %+v", job)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
	}
	f, _ := films.Find(a.films(), "talk")
	playerOpenFilm(t, a, tt, updates, f)
	if a.player.playErr != "" || playerVideoConfig(tt).Source != want {
		t.Fatalf("error %q, plays %q", a.player.playErr, playerVideoConfig(tt).Source)
	}
}

// TestSubtitlesOnlyFilmVideoThatFails checks a video download that fails: the film keeps no video as before, and the message bar says it could not be downloaded.
func TestSubtitlesOnlyFilmVideoThatFails(t *testing.T) {
	p := &videoPipeline{t: t, err: errors.New("ERROR: [youtube] jNQXAC9IVRw: Video unavailable. This video is private")}
	a, tt, updates, item := streamFilm(t, p, false)
	playerOpenFilm(t, a, tt, updates, item)
	tt.Press("id:film-stream-download")
	settle(t, a, tt, updates)
	job, _ := a.library.Find("talk")
	// It keeps no video as the user chose, so processing it again later does not keep one either.
	if job.Stage != core.StageComplete || !job.SubtitlesOnly || job.Redo != "" || job.VideoPath != item.VideoPath || job.Quality != core.QualitySubtitles {
		t.Fatalf("job %+v", job)
	}
	if want := testlocale.Messages().QueueVideoFailed("Talk", ""); !a.lib.notice.Open || !strings.HasPrefix(a.lib.notice.Text, want) {
		t.Fatalf("notice %q, want %q…", a.lib.notice.Text, want)
	}
}

// TestSubtitlesOnlyFilmKeepsToItsSubtitles checks that a video that turns up in the library is not taken for the video of a film that keeps none, as a renamed film's would be, and that the film leaves the shelf once its subtitles are gone.
func TestSubtitlesOnlyFilmKeepsToItsSubtitles(t *testing.T) {
	a, _, _, item := streamFilm(t, &fakePipeline{}, true)
	writeFakeClip(t, filepath.Join(a.snap().LibraryDir, "other.mp4"), 12, "202020", "101010")
	a.refreshLibrary()
	if job, _ := a.library.Find("talk"); job.VideoPath != item.VideoPath || !job.SubtitlesOnly {
		t.Fatalf("job %+v", job)
	}
	if err := os.RemoveAll(filepath.Dir(item.VideoPath)); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	if _, ok := a.library.Find("talk"); ok {
		t.Fatal("the film stayed without its subtitles")
	}
}

// TestSubtitlesOnlyFilmFollowsItsRenamedFolder checks that a film that keeps no video follows its folder renamed in Finder, as a downloaded film does, with its place, rather than leaving the shelf because its subtitles are not where they were.
func TestSubtitlesOnlyFilmFollowsItsRenamedFolder(t *testing.T) {
	a, tt, _, item := streamFilm(t, &fakePipeline{}, true)
	apply(tt, a, func() { a.setResumePositions(map[string]time.Duration{item.VideoPath: 5 * time.Second}) })
	next := filepath.Join(a.snap().LibraryDir, "Talk renamed [jNQXAC9IVRw]")
	if err := os.Rename(filepath.Dir(item.VideoPath), next); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	job, ok := a.library.Find("talk")
	if !ok {
		t.Fatal("the film left the shelf when its folder was renamed")
	}
	video := filepath.Join(next, filepath.Base(item.VideoPath))
	if job.SubtitlePath != filepath.Join(next, filepath.Base(item.SubtitlePath)) || job.VideoPath != video || !job.SubtitlesOnly {
		t.Fatalf("job %+v", job)
	}
	if a.ResumeAt(video) != 5*time.Second {
		t.Fatalf("place %v", a.ResumeAt(video))
	}
}

// TestSubtitlesOnlyFilmFoundElsewhereInTheLibrary checks that the path the sound of a film that keeps no video was downloaded to goes with its subtitles when they are found under the same folder name elsewhere in the library (relocateMissing).
func TestSubtitlesOnlyFilmFoundElsewhereInTheLibrary(t *testing.T) {
	a, _, _, item := streamFilm(t, &fakePipeline{}, true)
	root := a.snap().LibraryDir
	away := filepath.Join(root, "Old", filepath.Base(filepath.Dir(item.VideoPath)))
	a.library.Jobs[0].VideoPath = filepath.Join(away, filepath.Base(item.VideoPath))
	a.library.Jobs[0].SubtitlePath = filepath.Join(away, filepath.Base(item.SubtitlePath))
	a.refreshLibrary()
	job, ok := a.library.Find("talk")
	if !ok || job.SubtitlePath != item.SubtitlePath || job.VideoPath != item.VideoPath {
		t.Fatalf("job %+v, found %v", job, ok)
	}
}
