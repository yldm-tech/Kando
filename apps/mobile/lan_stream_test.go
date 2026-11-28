package main

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/nsgotest"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
)

// A finished video on the paired computer streams directly over the local network from the film's page without downloading it first, plays immediately with its subtitles, serves HTTP Range requests through the loopback server, and can still be downloaded offline later.
func TestAComputerFilmStreamsOverTheLocalNetwork(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	row := talkRow(id)
	p.app.WaitFor(row, wait)

	p.app.Press(row)
	p.app.WaitFor("id:film-stream", wait)
	if !p.app.Exists("id:film-download") {
		t.Fatalf("expected both Play and Download on film page, got %q", p.app.Texts())
	}
	p.app.Press("id:film-stream")
	p.app.WaitFor("id:player-video", wait)
	vid := p.app.Find("id:player-video").Video
	if !strings.HasPrefix(vid.Source, "http://127.0.0.1:") || !strings.HasSuffix(vid.Source, "/video.mp4") {
		t.Fatalf("streamed film plays %q, want loopback video.mp4", vid.Source)
	}
	if vid.Paused {
		t.Fatal("streamed film opened paused, want playing immediately")
	}

	// The loopback server forwards byte-range requests to the computer and serves the video bytes.
	req, err := http.NewRequest(http.MethodGet, vid.Source, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-15")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("range request answered %d, want 206", resp.StatusCode)
	}
	if code, body := fetch(t, vid.Source); code != http.StatusOK || len(body) != len(talkVideo) {
		t.Fatalf("full fetch answered %d, %d bytes (want %d)", code, len(body), len(talkVideo))
	}

	// Subtitles are kept locally to study while no video file is downloaded to disk.
	kept, err := store.Open(dir).Films()
	if err != nil || len(kept) != 1 || !kept[0].Streamed || kept[0].Done() || kept[0].HasVideo() || kept[0].LibraryID != "lib" {
		t.Fatalf("kept %+v, %v", kept, err)
	}
	if cues, ok, _ := store.Open(dir).Cues(id, "f1"); !ok || len(cues) != len(talkCues) {
		t.Fatalf("cues kept: %v %d", ok, len(cues))
	}
	if _, err := os.Stat(filepath.Join(dir, "films", id, "f1", "video.mp4")); !os.IsNotExist(err) {
		t.Fatalf("streamed film unexpectedly wrote local video: %v", err)
	}
	p.app.WaitFor("id:player-lines-pane >> text:How are you?", wait)

	// Minimize and download offline: the local file replaces the stream.
	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:film-download", wait)
	p.app.Press("id:film-download")
	p.app.WaitFor("id:film-play", wait)
	data, err := os.ReadFile(filepath.Join(dir, "films", id, "f1", "video.mp4"))
	if err != nil || !bytes.Equal(data, talkVideo) {
		t.Fatalf("downloaded video is %d bytes, %v", len(data), err)
	}
}

// A computer film can also start streaming straight from its row's options sheet; once streamed, its options offer Play and Save for offline (not Resume/Cancel, nor deleting offline files), and pressing the row opens the player directly.
func TestAFilmStreamsFromOptionsAndReopensFromTheList(t *testing.T) {
	d := newDesktop(t)
	d.SetNoFilmOps(true)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	row := talkRow(id)
	options := "id:films-options-" + id + "-f1"
	p.app.WaitFor(options, wait)

	p.app.Press(options)
	p.choose(msgs.FilmPlay())
	p.app.WaitFor("id:player-video", wait)
	if src := p.playerSource(); !strings.HasPrefix(src, "http://127.0.0.1:") {
		t.Fatalf("playing from options opened %q", src)
	}
	p.app.Press("id:player-minimize")

	// Options for a streamed film offer Play and Save for offline, never Resume/Cancel, nor deleting offline files the phone does not have: what streaming kept is its lines, which its computer still lists the film with.
	p.app.Press(options)
	if got, want := p.optionLabels(), []string{msgs.FilmPlay(), msgs.FilmsSaveOffline()}; !slices.Equal(got, want) {
		t.Fatalf("options after streaming are %q, want %q", got, want)
	}
	p.app.Key("Escape")
	p.app.WaitForGone("id:films-options-sheet", wait)

	// Pressing the row directly opens the player again.
	p.app.Press(row)
	p.app.WaitFor("id:player-video", wait)
}

// A streamed episode on the computer's network automatically queues the next episode on the computer when it ends.
func TestAStreamedComputerFilmGoesOnToTheNextEpisode(t *testing.T) {
	d := newDesktop(t)
	d.AddFilm(synctest.Film{Library: "lib", Cues: talkCues, Video: talkVideo,
		Film: sync.Film{ID: "ep1", Title: "Episode 1", Source: "youtube", DurationMS: 10_000, VideoSize: int64(len(talkVideo)), Next: "ep2", Updated: now()}})
	d.AddFilm(synctest.Film{Library: "lib", Cues: talkCues, Video: talkVideo,
		Film: sync.Film{ID: "ep2", Title: "Episode 2", Source: "youtube", DurationMS: 10_000, VideoSize: int64(len(talkVideo)), Updated: now()}})
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)

	p.app.WaitFor(filmRow(id, "ep1"), wait)
	p.app.Press(filmRow(id, "ep1"))
	p.app.WaitFor("id:film-stream", wait)
	p.app.Press("id:film-stream")
	p.app.WaitFor("id:player-video", wait)
	firstSrc := p.playerSource()

	p.app.Report(nsgotest.Event{Kind: nsgotest.EventEnd, ID: p.app.Find("id:player-video").ID})
	p.app.WaitFor("id:player-up-next", wait)
	if got := p.texts("id:player-up-next-title"); got != "Episode 2" {
		t.Fatalf("up next shows %q, want Episode 2", got)
	}
	p.app.Press("id:player-up-next-play")
	p.app.WaitUntil(wait, func() bool {
		src := p.playerSource()
		return strings.HasPrefix(src, "http://127.0.0.1:") && src != firstSrc
	})
}

// A sound-only film on the computer streams as sound.m4a, while a subtitles-only film with no video does not offer streaming.
func TestSoundFilmStreamsAndSubtitlesOnlyDoesNot(t *testing.T) {
	d := newDesktop(t)
	d.AddFilm(synctest.Film{Library: "lib", Cues: talkCues, Video: []byte("audio-bytes"),
		Film: sync.Film{ID: "s1", Title: "Morning Podcast", Source: "youtube", DurationMS: 10_000, Sound: true, VideoSize: 11, Updated: now()}})
	d.AddFilm(linesFilm())
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)

	p.app.WaitFor(filmRow(id, "s1"), wait)
	p.app.Press(filmRow(id, "s1"))
	p.app.WaitFor("id:film-stream", wait)
	p.app.Press("id:film-stream")
	p.app.WaitUntil(wait, func() bool { return strings.HasSuffix(p.playerSource(), "/sound.m4a") })
	p.app.Press("id:player-minimize")
	p.app.HeaderBack("id:film-page")

	p.app.Press(filmRow(id, "f3"))
	p.app.WaitFor("id:film-download", wait)
	if p.app.Exists("id:film-stream") {
		t.Fatal("subtitles-only film unexpectedly offered streaming")
	}
}

// A computer's film streamed without its poster kept on the phone shows in the mini player the poster its computer gives, as its page and its row do.
func TestTheMiniPlayerShowsAStreamedFilmsPoster(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	row := talkRow(store.DesktopID(d.Fingerprint))
	p.app.WaitFor(row, wait)
	p.app.Press(row)
	p.app.WaitFor("id:film-stream", wait)
	p.app.Press("id:film-stream")
	p.app.WaitFor("id:player-video", wait)
	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:player-mini", wait)
	p.app.WaitUntil(wait, func() bool {
		return string(p.app.FindMounted("id:player-mini-poster").Image.Source.Data) == "jpeg"
	})
}
