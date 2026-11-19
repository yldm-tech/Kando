package main

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/nowplaying"
	cmdpalette "kando/apps/kando/palette"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	"kando/packages/media"
	"kando/packages/track"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The player's tests drive the film page as nsgotest runs it headless: the system's player (core.Video) mounts and records what it is asked to play, and a test reports for it what a platform's player reports, that it loaded (tt.Play) and where it got to (tt.Progress). No system player plays the fake clips, so the picture itself is the platform's and these tests check what the app asked of it.

// talkSubtitles are the bilingual lines of the test films: a line with a translation at 1 to 2.5 seconds, one without at 4 to 5.
const talkSubtitles = "1\n00:00:01,000 --> 00:00:02,500\n你好。\nHello.\n\n2\n00:00:04,000 --> 00:00:05,000\nBye.\n\n"

// playerFilm is a library at a temporary folder holding the film "talk": its video, a placeholder the fake ffmpeg plays as a clip of seconds, and its bilingual subtitles, as a finished job made them. It returns the app showing the library, its window and its background updates, and the film.
func playerFilm(t *testing.T, seconds int, p media.Pipeline, opts ...nsgotest.Option) (*app, *nsgotest.App, chan func(), films.Film) {
	t.Helper()
	dir := t.TempDir()
	video := filepath.Join(dir, "talk", "talk.mp4")
	writeFakeClip(t, video, seconds, "202020", "101010")
	srt := filepath.Join(dir, "talk", "talk.bilingual.srt")
	if err := os.WriteFile(srt, []byte(talkSubtitles), 0o644); err != nil {
		t.Fatal(err)
	}
	if p == nil {
		p = &fakePipeline{}
	}
	a, tt, updates := newTestApp(t, p, opts...)
	a.player.remuxDir = filepath.Join(t.TempDir(), "playback")
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{ID: "talk", Title: "talk", Source: core.FileURL(video), Language: "简体中文", Stage: core.StageComplete, VideoPath: video, SubtitlePath: srt, LibraryDir: dir}}
	refresh(tt, a)
	item, ok := films.Find(a.films(), "talk")
	if !ok {
		t.Fatalf("the film is not on the shelf: %+v", a.films())
	}
	return a, tt, updates, item
}

// playerOpenFilm opens item from the library as its poster does, held on its first frame, and waits for its subtitles and its folder to be read.
func playerOpenFilm(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), item films.Film) {
	t.Helper()
	apply(tt, a, func() { a.Open(item) })
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready && (item.VideoPath == "" || a.player.film.info.Folder != "") })
}

// playerLoadFilm reports that the system's player loaded the open video, length long, as a platform's does once it has opened it.
func playerLoadFilm(tt *nsgotest.App, a *app, length time.Duration) {
	tt.Play("id:film-video", ns.VideoInfo{Duration: length, Size: ns.Size{Width: 640, Height: 360}})
	refresh(tt, a)
}

// playerPlayTo reports that the system's player played the open video to at.
func playerPlayTo(tt *nsgotest.App, a *app, at time.Duration) {
	tt.Progress("id:film-video", at, a.PlayLength())
	refresh(tt, a)
}

// playerOpen is a library with "talk", 12 seconds long, opened and loaded by the system's player, held on its first frame.
func playerOpen(t *testing.T) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	a, tt, updates, item := playerFilm(t, 12, nil)
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	return a, tt, updates
}

// playerVideoConfig is what the page asks the system's player to play now.
func playerVideoConfig(tt *nsgotest.App) nsgotest.VideoConfig { return tt.Find("id:film-video").Video }

// playerSeeks are where the page asked the system's player to jump, oldest first.
func playerSeeks(tt *nsgotest.App) []time.Duration { return tt.Find("id:film-video").Seeks }

func TestOpenFilmShowsBilingualLines(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	var revealed string
	prev := showItemInFolder
	showItemInFolder = func(_ *ns.Scope, path string, done func(error)) { revealed = path; done(nil) }
	t.Cleanup(func() { showItemInFolder = prev })
	playerOpenFilm(t, a, tt, updates, item)
	msgs := testlocale.Messages()
	for _, text := range []string{"你好。", "Hello.", "Bye.", "0:01.0", "0:04.0", msgs.FilmAll()} {
		wantText(t, tt, text)
	}
	wantText(t, tt, msgs.FilmStatusTranslated(50))
	if got := tt.Find("id:film-back").Props.Label; got != msgs.FilmBackToLibrary() {
		t.Fatalf("the back button is named %q", got)
	}
	tt.Press("id:film-reveal")
	if revealed != item.SubtitlePath {
		t.Fatalf("revealed %q, want the subtitles %q", revealed, item.SubtitlePath)
	}
	tt.Press("id:film-back")
	if a.openKey != "" || a.player.play != nil || tt.Exists("id:film") {
		t.Fatalf("back left open %q play %v", a.openKey, a.player.play != nil)
	}
}

// TestOpenFilmReadsItsLinesThroughItsDocument checks that the open film's lines are read through its subtitle document (media.ReadTrack): a subtitle file rewritten by something else, as an older Kando or an editor, shows its new lines once read again, and reading writes nothing beside the video.
func TestOpenFilmReadsItsLinesThroughItsDocument(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	playerOpenFilm(t, a, tt, updates, item)
	data, err := os.ReadFile(item.SubtitlePath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "Bye.", "See you.", 1)
	if edited == string(data) {
		t.Fatalf("the film's subtitles hold no %q: %q", "Bye.", data)
	}
	if err := os.WriteFile(item.SubtitlePath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(item.SubtitlePath, later, later); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, a.reloadOpenSubtitles)
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	wantText(t, tt, "See you.")
	if logs := track.Logs(item.VideoPath); logs != nil {
		t.Fatalf("reading the film wrote %v", logs)
	}
}

// TestOpenFilmPlaysTheVideo opens a film from its poster: the system's player opens its video held, 播放 shows once it has loaded, and the play button starts and holds it; going back takes the player away.
func TestOpenFilmPlaysTheVideo(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	playerOpenFilm(t, a, tt, updates, item)
	msgs := testlocale.Messages()
	if v := playerVideoConfig(tt); v.Source != item.VideoPath || !v.Paused {
		t.Fatalf("before it loaded the player plays %+v", v)
	}
	if tt.Enabled("id:film-play") || tt.Find("id:film-play").Props.Label != msgs.FilmOpening() {
		t.Fatalf("the play button while the video opens: %+v", tt.Find("id:film-play").Props)
	}
	wantText(t, tt, msgs.FilmOpeningVideo())
	playerLoadFilm(tt, a, 12*time.Second)
	if !playerVideoConfig(tt).Paused || tt.Find("id:film-play").Props.Label != msgs.FilmPlay() || len(playerSeeks(tt)) != 0 {
		t.Fatalf("a film opened from its poster: %+v, label %q, seeks %v", playerVideoConfig(tt), tt.Find("id:film-play").Props.Label, playerSeeks(tt))
	}
	wantNoText(t, tt, msgs.FilmOpeningVideo())
	tt.Press("id:film-play")
	if playerVideoConfig(tt).Paused || !a.player.play.Playing() || tt.Find("id:film-play").Props.Label != msgs.FilmPause() {
		t.Fatalf("play: %+v playing %v", playerVideoConfig(tt), a.player.play.Playing())
	}
	tt.Press("id:film-play")
	if !playerVideoConfig(tt).Paused || a.player.play.Playing() {
		t.Fatalf("pause: %+v", playerVideoConfig(tt))
	}
	tt.Press("id:film-back")
	if a.player.play != nil || tt.Exists("id:film-video") {
		t.Fatal("the system's player stayed after going back")
	}
}

// TestPlaybackFollowsTheSystemsPlayer checks the playhead: it is where the system's player last said, and moves on with the clock while the film plays, at its speed; at the end it stays there, held, and play starts the film over.
func TestPlaybackFollowsTheSystemsPlayer(t *testing.T) {
	a, tt, _ := playerOpen(t)
	tt.Key("space")
	playerPlayTo(tt, a, 2*time.Second)
	if head := a.PlayHead(); head != 2*time.Second {
		t.Fatalf("head %v after the player said 2s", head)
	}
	tt.Advance(500 * time.Millisecond)
	if head := a.PlayHead(); head < 2400*time.Millisecond || head > 2600*time.Millisecond {
		t.Fatalf("head %v half a second on", head)
	}
	tt.Key("space")
	held := a.PlayHead()
	tt.Advance(time.Second)
	if a.PlayHead() != held {
		t.Fatalf("a held film moved from %v to %v", held, a.PlayHead())
	}
	tt.Key("space")
	tt.Progress("id:film-video", 12*time.Second, 12*time.Second)
	refresh(tt, a)
	p := a.player.play
	if !p.finished || p.Playing() || a.PlayHead() != 12*time.Second || !playerVideoConfig(tt).Paused {
		t.Fatalf("at the end: finished %v playing %v head %v paused %v", p.finished, p.Playing(), a.PlayHead(), playerVideoConfig(tt).Paused)
	}
	tt.Advance(time.Second)
	if a.PlayHead() != 12*time.Second {
		t.Fatalf("the film went on past its end to %v", a.PlayHead())
	}
	tt.Key("space")
	if !p.Playing() || a.PlayHead() != 0 || playerVideoConfig(tt).Paused || !slices.Contains(playerSeeks(tt), 0) {
		t.Fatalf("play after the end: playing %v head %v seeks %v", p.Playing(), a.PlayHead(), playerSeeks(tt))
	}
}

// TestPlayFromTheEndSeeksAsItPlays plays a film again once it has ended, with play and with a seek: the update that unpauses the system's player also has it jump, which a video that played to its end then plays from (core.Ref.Seek).
func TestPlayFromTheEndSeeksAsItPlays(t *testing.T) {
	for _, start := range []struct {
		name string
		do   func(a *app, tt *nsgotest.App)
		at   time.Duration
	}{
		{"play", func(_ *app, tt *nsgotest.App) { tt.Key("space") }, 0},
		{"seek", func(a *app, tt *nsgotest.App) { a.seekPlayback(3 * time.Second); refresh(tt, a) }, 3 * time.Second},
	} {
		t.Run(start.name, func(t *testing.T) {
			a, tt, _ := playerOpen(t)
			tt.Key("space")
			tt.Progress("id:film-video", 12*time.Second, 12*time.Second)
			refresh(tt, a)
			p := a.player.play
			if !p.finished {
				t.Fatal("the film did not end")
			}
			seeks := len(playerSeeks(tt))
			start.do(a, tt)
			got := playerSeeks(tt)
			if !p.Playing() || playerVideoConfig(tt).Paused || len(got) != seeks+1 || got[len(got)-1] != start.at {
				t.Fatalf("played from the end: playing %v paused %v seeks %v", p.Playing(), playerVideoConfig(tt).Paused, got)
			}
		})
	}
}

// TestSeekWaitsForItsLanding seeks the open film: the page asks the system's player to jump, the film plays from there, and a report the player sent before the seek landed does not take the playhead back.
func TestSeekWaitsForItsLanding(t *testing.T) {
	a, tt, _ := playerOpen(t)
	tt.Key("right")
	if got := playerSeeks(tt); len(got) != 1 || got[0] != 5*time.Second {
		t.Fatalf("→ seeked to %v", got)
	}
	if !a.player.play.Playing() || playerVideoConfig(tt).Paused || a.PlayHead() != 5*time.Second {
		t.Fatalf("after →: playing %v head %v", a.player.play.Playing(), a.PlayHead())
	}
	wantText(t, tt, testlocale.Messages().FilmSeconds("+5"))
	playerPlayTo(tt, a, 200*time.Millisecond)
	if head := a.PlayHead(); head < 5*time.Second {
		t.Fatalf("a report from before the seek took the playhead back to %v", head)
	}
	playerPlayTo(tt, a, 5100*time.Millisecond)
	if head := a.PlayHead(); head != 5100*time.Millisecond {
		t.Fatalf("head %v once the seek landed", head)
	}
	tt.Key("mod+left")
	if got := playerSeeks(tt); got[len(got)-1] != 0 {
		t.Fatalf("⌘← from 5.1s seeked to %v, want the start", got[len(got)-1])
	}
}

// TestOpenAtSeeksOnceLoaded opens a film playing from 0:06, as the hero's 从 0:06 继续 does: the system's player opens it held and jumps there once it has loaded, then plays.
func TestOpenAtSeeksOnceLoaded(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	apply(tt, a, func() { a.OpenAt(item, 6*time.Second) })
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	if !playerVideoConfig(tt).Paused || len(playerSeeks(tt)) != 0 || a.PlayHead() != 6*time.Second {
		t.Fatalf("before it loaded: %+v seeks %v head %v", playerVideoConfig(tt), playerSeeks(tt), a.PlayHead())
	}
	playerLoadFilm(tt, a, 12*time.Second)
	if got := playerSeeks(tt); !slices.Equal(got, []time.Duration{6 * time.Second}) || playerVideoConfig(tt).Paused || !a.player.play.Playing() {
		t.Fatalf("once loaded: seeks %v paused %v", got, playerVideoConfig(tt).Paused)
	}
}

// TestUnsupportedVideoPlaysFromACopy has the system's player fail on the open video, as AVFoundation fails on an MKV: the page plays a copy ffmpeg makes in place of the video, first with its streams as they are, then encoded anew when that fails too, and says the video cannot be played once both have.
func TestUnsupportedVideoPlaysFromACopy(t *testing.T) {
	bin := t.TempDir()
	// The stand-in writes its output file, the last argument, as ffmpeg does; asked for a poster on its standard output (pipe:1) it has none, rather than writing a file of that name where the test runs.
	ffmpeg := writeFakeScript(t, bin, "ffmpeg", "for last; do :; done\n[ \"$last\" = pipe:1 ] && exit 1\nprintf copy > \"$last\"\n")
	a, tt, updates, item := playerFilm(t, 12, media.Tools{FFmpeg: ffmpeg})
	playerOpenFilm(t, a, tt, updates, item)
	apply(tt, a, func() { a.VideoFailed(errors.New("cannot open")) })
	if a.player.play == nil || a.player.play.source != "" || tt.Exists("id:film-video") {
		t.Fatalf("while the copy is made the player plays %q", a.player.play.source)
	}
	wantText(t, tt, testlocale.Messages().FilmOpeningVideo())
	waitUntil(t, tt, a, updates, func() bool { return a.player.play.source != "" })
	copied := a.player.play.source
	if filepath.Dir(copied) != a.player.remuxDir || playerVideoConfig(tt).Source != copied {
		t.Fatalf("the player plays %q, want the copy in %s", playerVideoConfig(tt).Source, a.player.remuxDir)
	}
	apply(tt, a, func() { a.VideoFailed(errors.New("cannot decode")) })
	waitUntil(t, tt, a, updates, func() bool { return a.player.play.source != "" })
	if encoded := a.player.play.source; encoded == copied || a.player.play.remux != film.RemuxEncode {
		t.Fatalf("the second copy is %q, the first %q", encoded, copied)
	}
	apply(tt, a, func() { a.VideoFailed(errors.New("cannot decode")) })
	if a.player.playErr != playerErrUnplayable || tt.Exists("id:film-video") {
		t.Fatalf("after both copies failed: error %q", a.player.playErr)
	}
	wantText(t, tt, testlocale.Messages().PlayerUnplayable())
}

// TestUnsupportedVideoEncodedWhenTheCopyFails has ffmpeg fail to copy the open video's streams into an MP4 as they are, as it does for a WebM's VP8 and Vorbis: the page plays a copy encoded anew instead of saying the video cannot be played.
func TestUnsupportedVideoEncodedWhenTheCopyFails(t *testing.T) {
	bin := t.TempDir()
	// The stand-in fails to copy the streams as they are and writes its output file, the last argument, when it encodes them; asked for a poster on its standard output (pipe:1) it has none.
	ffmpeg := writeFakeScript(t, bin, "ffmpeg", "for arg; do [ \"$arg\" = copy ] && exit 1; done\nfor last; do :; done\n[ \"$last\" = pipe:1 ] && exit 1\nprintf encoded > \"$last\"\n")
	a, tt, updates, item := playerFilm(t, 12, media.Tools{FFmpeg: ffmpeg})
	playerOpenFilm(t, a, tt, updates, item)
	apply(tt, a, func() { a.VideoFailed(errors.New("cannot open")) })
	waitUntil(t, tt, a, updates, func() bool { return a.player.play.source != "" || a.player.playErr != "" })
	if p := a.player.play; a.player.playErr != "" || p.remux != film.RemuxEncode || playerVideoConfig(tt).Source != p.source {
		t.Fatalf("after the copy failed: error %q, step %v, the player plays %q", a.player.playErr, p.remux, playerVideoConfig(tt).Source)
	}
	if data, err := os.ReadFile(a.player.play.source); err != nil || string(data) != "encoded" {
		t.Fatalf("the player plays %q, %v", data, err)
	}
}

// TestVideoWithoutGStreamerPlugins has the system's player fail as it does on Linux without the GStreamer plug-ins the video needs: no copy is made, as none would play, and the page says which to install, with the command that installs them to copy.
func TestVideoWithoutGStreamerPlugins(t *testing.T) {
	bin := t.TempDir()
	ffmpeg := writeFakeScript(t, bin, "ffmpeg", "for last; do :; done\n[ \"$last\" = pipe:1 ] && exit 1\nprintf copy > \"$last\"\n")
	a, tt, updates, item := playerFilm(t, 12, media.Tools{FFmpeg: ffmpeg})
	playerOpenFilm(t, a, tt, updates, item)
	apply(tt, a, func() {
		a.VideoFailed(errors.New("Your GStreamer installation is missing a plug-in. No installed GStreamer plug-in can play this format (H.264 / AAC need gst-libav or gst-plugins-ugly / -bad)."))
	})
	if a.player.playErr != playerErrPlugins || a.player.play.remux != 0 || a.player.remuxStop != nil || tt.Exists("id:film-video") {
		t.Fatalf("error %q, copy %v", a.player.playErr, a.player.play.remux)
	}
	wantText(t, tt, testlocale.Messages().PlayerMissingPlugins())
	wantText(t, tt, film.GStreamerInstall)
	tt.Press("id:film-stage-copy-command")
	if got := tt.Clipboard; got != film.GStreamerInstall {
		t.Fatalf("copied %q, want %q", got, film.GStreamerInstall)
	}
	wantText(t, tt, testlocale.Messages().FilmCommandCopied())
}

// TestUnsupportedVideoWithoutFFmpeg: without ffmpeg there is no copy to make, and the page says the video cannot be played.
func TestUnsupportedVideoWithoutFFmpeg(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	playerOpenFilm(t, a, tt, updates, item)
	apply(tt, a, func() { a.VideoFailed(errors.New("cannot open")) })
	if a.player.playErr != playerErrUnplayable {
		t.Fatalf("error %q", a.player.playErr)
	}
	wantText(t, tt, testlocale.Messages().PlayerUnplayable())
}

// TestPlayerInEnglish checks the player in English: the title bar, the subtitle list, the 信息 tab with the saved target language named in English, the transport, the mode menu and the style view.
func TestPlayerInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, tt, updates, item := playerFilm(t, 12, nil)
	playerOpenFilm(t, a, tt, updates, item)
	for _, want := range []string{"50% translated", "All", "Subtitles", "Clips", "Info", "Style", "Opening video…"} {
		wantText(t, tt, want)
	}
	tt.Press("id:film-tab-info")
	for _, want := range []string{"Film Folder", "Files", "Source", "Local file", "Job record", "Simplified Chinese", "Bilingual subtitles · ", "Video · "} {
		wantText(t, tt, want)
	}
	playerLoadFilm(tt, a, 12*time.Second)
	for id, want := range map[string]string{"film-back5": "Back 5 Seconds", "film-forward5": "Forward 5 Seconds", "film-mute": "Mute", "film-full": "Full Screen", "film-panel-toggle": "Sidebar", "film-play": "Play", "film-back": "Back to Library"} {
		if got := tt.Find("id:" + id).Props.Label; got != want {
			t.Fatalf("%s is named %q, want %q", id, got, want)
		}
	}
	tt.Press("id:film-mode")
	for _, want := range []string{"Display", "Off", "Hides only the subtitles", "Original Only", "Translation Only", "Subtitle Delay", "Style…"} {
		wantText(t, tt, want)
	}
	tt.Press("id:film-style")
	for _, want := range []string{"Playback", "Restore Default Style", "Show Subtitles", "Outline", "Box", "Large", "Font", "Bilingual Order", "Translation on Top", "Original on Top", "Position", "Higher", "Bottom", "Translation", "Original", "Size", "Color"} {
		wantText(t, tt, want)
	}
	tt.Press("id:film-style-preset-box")
	if look := a.player.look; look.Preset != "box" || look.Translation.Family != "默认" {
		t.Fatalf("look %+v after Box", look)
	}
	for _, chinese := range []string{"字幕", "简体中文", locales.In("zh-Hans").FilmShowInFinder(platformName()), "恢复默认样式"} {
		if hasText(tt, chinese) {
			t.Fatalf("the player still reads %q: %q", chinese, tt.Texts())
		}
	}
}

// TestFilmSideShowsChaptersAndStyle checks the side panel's 片段, 信息 and the style view on a film, and that Escape closes the style view, keeping the film open.
func TestFilmSideShowsChaptersAndStyle(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 65, nil)
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 65*time.Second)
	tt.Press("id:film-tab-clips")
	// The film's two lines start in its first 5 seconds, so the even cuts of its 65 seconds have no line to start on: one 片段 covers the film, named after its first line.
	for _, text := range []string{"片段 1 · 0:00–1:05", "你好。", "Hello."} {
		wantText(t, tt, text)
	}
	tt.Press("id:film-tab-info")
	for _, text := range []string{"影片文件夹", films.DisplayPath(testlocale.Messages(), filepath.Dir(item.VideoPath)), "talk.mp4", "talk.bilingual.srt", "本地文件"} {
		wantText(t, tt, text)
	}
	apply(tt, a, func() { a.SetStyleView(true) })
	for _, text := range []string{"译文在上", "字号", "底框", "恢复默认样式"} {
		wantText(t, tt, text)
	}
	tt.Press("id:film-style-preset-box")
	if look := a.player.look; look.Preset != "box" || !look.Box || look.Shadow {
		t.Fatalf("look %+v after 底框", look)
	}
	tt.Press(whole("原文在上"))
	if a.player.subOrder != film.SubOriginalFirst {
		t.Fatalf("order %q after 原文在上", a.player.subOrder)
	}
	tt.Press("id:film-style-reset")
	if a.player.look != film.DefaultLook() {
		t.Fatalf("look %+v after reset", a.player.look)
	}
	tt.Key("escape")
	if a.player.film.styleView || a.openKey == "" || !tt.Exists("id:film-tab-clips") {
		t.Fatalf("Escape left style %v open %q", a.player.film.styleView, a.openKey)
	}
}

// TestOpenFilmStartsLocalVideo opens a video no job has processed: it shows 还没有字幕 and 生成字幕, which makes its subtitles, the film staying open on its job.
func TestOpenFilmStartsLocalVideo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "interview.mp4"), []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.refreshLibrary()
	refresh(tt, a)
	// The refresh gathers the video into a folder of its own, so the film is found by its title, as the poster's click found it
	at := slices.IndexFunc(a.films(), func(f films.Film) bool { return f.Title == "interview" })
	if at < 0 {
		t.Fatalf("films %+v", a.films())
	}
	item := a.films()[at]
	playerOpenFilm(t, a, tt, updates, item)
	wantText(t, tt, testlocale.Messages().FilmNoSubtitlesYet())
	wantText(t, tt, testlocale.Messages().FilmMakeSubtitles())
	tt.Press("id:film-start")
	settle(t, a, tt, updates)
	if a.queue.job == nil || a.queue.job.Stage != core.StageComplete || a.openKey != a.queue.job.ID {
		t.Fatalf("open %q job %#v", a.openKey, a.queue.job)
	}
	// Its subtitles, written beside it as they were made, show.
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	wantText(t, tt, "hello")
}

// TestOpenFilmClearsTheTitleBar checks that an open film hides the sidebar and fills the window up to its top edge, its header bar putting the back button after the traffic lights.
func TestOpenFilmClearsTheTitleBar(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	playerOpenFilm(t, a, tt, updates, item)
	plain := pageFrame(tt.Find("id:film-back"))
	useTitleBar(tt, ns.Edges{Top: 28, Left: 80})
	barred := pageFrame(tt.Find("id:film-back"))
	if plain.Origin.X >= 80 || barred.Origin.X < 120 || abs32(barred.Origin.Y-plain.Origin.Y) > 0.5 {
		t.Fatalf("back button at %v without the traffic lights and %v beside them", plain, barred)
	}
	if tt.Exists("id:sidebar") {
		t.Fatal("sidebar shown over an open film")
	}
	if f := pageFrame(tt.Find("id:film")); f.Origin.Y != 0 || f.Origin.X != 0 {
		t.Fatalf("the film starts at %v, want the window's corner", f.Origin)
	}
}

// TestFilmInTheColumnStartsAtItsEdge checks that a film opened from the YouTube page, which shows in the content column beside the sidebar, puts its back button the prototype's padding of 20 from the column's edge rather than leaving room for the traffic lights, which are over the sidebar.
func TestFilmInTheColumnStartsAtItsEdge(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	useTitleBar(tt, ns.Edges{Top: 28, Left: 80})
	apply(tt, a, func() {
		a.page = pageYouTube
		a.library.Jobs = []core.Job{{ID: "j", Source: "https://www.youtube.com/watch?v=abc", Stage: core.StageQueued}}
		a.openKey = "j"
	})
	if !tt.Exists("id:sidebar") {
		t.Fatal("sidebar hidden beside a film from the YouTube page")
	}
	back := pageFrame(tt.Find("id:film-back"))
	edge := sidebarWidth + pagePadX
	if back.Origin.X < edge || back.Origin.X-edge > 20 {
		t.Fatalf("back button at %v, want it within 20 of the column's edge at %v", back, edge)
	}
}

// TestRefreshClosesAFilmThatLeftTheShelf checks that an open video closes, and its playback stops, when a refresh of the shelf no longer finds it.
func TestRefreshClosesAFilmThatLeftTheShelf(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	dir := t.TempDir()
	video := filepath.Join(dir, "clip", "clip.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.refreshLibrary()
	items := a.films()
	if len(items) != 1 {
		t.Fatalf("films %+v", items)
	}
	a.Open(items[0])
	a.refreshLibrary()
	if a.openKey == "" || a.player.playPath == "" {
		t.Fatal("the film closed while it is still on the shelf")
	}
	if err := os.Remove(items[0].VideoPath); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	if a.openKey != "" || a.player.playPath != "" || a.player.play != nil {
		t.Fatalf("open %q playing %q after the film left the shelf", a.openKey, a.player.playPath)
	}
}

// TestSubtitleKeys checks V, S, Z and X on an open film: V hides the subtitles and brings back the mode they had, S steps original, translation, bilingual, and Z and X move them a tenth of a second, each saying so on the picture for a moment.
func TestSubtitleKeys(t *testing.T) {
	a, tt, _ := playerOpen(t)
	key := func(k, mode, osd string) {
		t.Helper()
		tt.Key(k)
		if a.player.subMode != mode || !hasText(tt, osd) {
			t.Fatalf("after %s: mode %q, want %q; texts %q", k, a.player.subMode, mode, tt.Texts())
		}
	}
	key("s", film.SubOriginal, "字幕：仅原文")
	key("v", film.SubOff, "字幕：关闭")
	key("v", film.SubOriginal, "字幕：仅原文")
	key("s", film.SubTranslation, "字幕：仅译文")
	key("s", film.SubBilingual, "字幕：双语")
	key("z", film.SubBilingual, "字幕 -0.1 秒")
	key("z", film.SubBilingual, "字幕 -0.2 秒")
	key("x", film.SubBilingual, "字幕 -0.1 秒")
	if a.player.film.delay != -100*time.Millisecond {
		t.Fatalf("delay %v", a.player.film.delay)
	}
	tt.Advance(1500 * time.Millisecond)
	if hasText(tt, "字幕 -0.1 秒") || tt.Exists("id:film-osd") {
		t.Fatalf("the message stayed on the picture: %q", tt.Texts())
	}
}

// TestModeMenuPicksTheMode opens the transport's subtitle menu, picks a mode in it, and checks that Escape closes the menu without leaving the film, and that 样式… opens the style view on the side panel.
func TestModeMenuPicksTheMode(t *testing.T) {
	a, tt, _ := playerOpen(t)
	tt.Press("id:film-mode")
	if !a.player.film.modeMenu || !hasText(tt, "字幕延迟") {
		t.Fatalf("menu not open: %q", tt.Texts())
	}
	tt.Press("id:film-mode-" + film.SubOriginal)
	if a.player.subMode != film.SubOriginal || a.player.film.modeMenu || hasText(tt, "字幕延迟") {
		t.Fatalf("mode %q menu %v", a.player.subMode, a.player.film.modeMenu)
	}
	tt.Press("id:film-mode")
	tt.Key("escape")
	if a.player.film.modeMenu || a.openKey == "" {
		t.Fatalf("Escape: menu %v open %q", a.player.film.modeMenu, a.openKey)
	}
	apply(tt, a, func() { a.player.panel = false })
	tt.Press("id:film-mode")
	tt.Press("id:film-style")
	if !a.player.film.styleView || !a.player.panel || !hasText(tt, "恢复默认样式") {
		t.Fatalf("style view %v panel %v", a.player.film.styleView, a.player.panel)
	}
}

// TestFindFocusesTheSubtitleSearch checks that ⌘F opens the side panel on its subtitle list, with the keyboard in its search field, and that ⇧⌘T hides and shows the panel.
func TestFindFocusesTheSubtitleSearch(t *testing.T) {
	a, tt, _ := playerOpen(t)
	tt.Key("mod+shift+t")
	tt.Advance(400 * time.Millisecond)
	if a.player.panel || tt.Exists("id:film-search") {
		t.Fatal("⇧⌘T left the panel")
	}
	apply(tt, a, func() { a.player.filmSide = "info" })
	tt.Key("mod+f")
	tt.Settle()
	if !a.player.panel || a.player.filmSide != "" || !tt.Find("id:film-search").Focused {
		t.Fatalf("panel %v side %q focused %v", a.player.panel, a.player.filmSide, tt.Find("id:film-search").Focused)
	}
	typeFocused(t, tt, "river")
	if a.player.film.search != "river" {
		t.Fatalf("search %q", a.player.film.search)
	}
	if !hasText(tt, testlocale.Messages().FilmNoMatch()) {
		t.Fatalf("a search no line holds lists %q", tt.Texts())
	}
}

// TestPanelLeavesThePictureAlone checks that the open side panel sits beside the picture rather than over it, so nothing of a film (a screencast's text, say) is hidden under it, and that the picture takes the whole window again once the panel is hidden.
func TestPanelLeavesThePictureAlone(t *testing.T) {
	a, tt, _ := playerOpen(t)
	picture := pageFrame(tt.Find("id:film-stage"))
	if !a.player.panel {
		t.Fatal("the panel is off on opening")
	}
	tt.Key("mod+shift+t")
	tt.Advance(400 * time.Millisecond)
	whole := pageFrame(tt.Find("id:film-stage"))
	if a.player.panel || whole.Origin.X != 0 || whole.Size.Width <= picture.Size.Width {
		t.Fatalf("panel %v: picture %+v, with the panel %+v", a.player.panel, whole, picture)
	}
	// The panel is 340 points wide, 16 in from the window's right edge.
	if under := picture.Origin.X + picture.Size.Width - (whole.Size.Width - 16 - 340); under > 0 {
		t.Fatalf("with the panel open the picture %+v reaches %v points under it", picture, under)
	}
}

// TestTransportTogglesThePanel checks the one control that folds the side panel away and brings it back, the transport's toggle, lit while the panel shows: the title bar has none of its own, as it fades with the transport.
func TestTransportTogglesThePanel(t *testing.T) {
	a, tt, _ := playerOpen(t)
	if tt.Exists("id:film-top-panel") {
		t.Fatal("the title bar has a panel button besides the transport's")
	}
	if !tt.Find("id:film-panel-toggle").Props.Selected {
		t.Fatal("the transport's toggle is not lit with the panel showing")
	}
	tt.Press("id:film-panel-toggle")
	tt.Advance(400 * time.Millisecond)
	if a.player.panel || tt.Exists("id:film-panel") || tt.Find("id:film-panel-toggle").Props.Selected {
		t.Fatal("the toggle left the panel")
	}
	tt.Press("id:film-panel-toggle")
	if !a.player.panel || !tt.Exists("id:film-search") {
		t.Fatal("the toggle did not bring the panel back")
	}
}

// TestPanelShowsWithoutAPicture checks that a film with no video, as one still downloading, shows its side panel even when the panel was hidden on the film before, since it has no transport to bring the panel back from.
func TestPanelShowsWithoutAPicture(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	apply(tt, a, func() {
		a.library.Jobs = []core.Job{{ID: "j", Source: "https://example.com/v", Stage: core.StageQueued}}
		a.player.panel = false
		a.openKey = "j"
	})
	if !tt.Exists("id:film-panel") || !tt.Exists("id:film-search") || tt.Exists("id:film-transport") {
		t.Fatalf("a film without a picture: %q", tt.Texts())
	}
}

// TestControlsFadeWhileTheFilmPlays checks that the controls and the pointer hide once the pointer rests for 2.5 seconds on a playing film, and come back when it moves; a held film keeps them.
func TestControlsFadeWhileTheFilmPlays(t *testing.T) {
	a, tt, _ := playerOpen(t)
	hidden := func() bool {
		return tt.Find("id:film-transport").Style.Opacity == 0 && tt.Find("id:film-stage").Style.Cursor == ns.CursorNone
	}
	tt.HoverAt("id:film", ns.Point{X: 400, Y: 300})
	tt.Key("space")
	if !a.player.play.Playing() {
		t.Fatal("Space did not start the film")
	}
	tt.Advance(time.Second)
	if hidden() {
		t.Fatal("the controls hid a second after the pointer moved")
	}
	tt.Advance(2 * time.Second)
	if !hidden() {
		t.Fatalf("the controls show after the pointer rested: opacity %v", tt.Find("id:film-transport").Style.Opacity)
	}
	tt.HoverAt("id:film", ns.Point{X: 420, Y: 310})
	if hidden() {
		t.Fatal("moving the pointer did not bring the controls back")
	}
	tt.Key("space")
	tt.Advance(3 * time.Second)
	if hidden() {
		t.Fatal("the controls hid while the film was held")
	}
}

// TestControlsShowWhileTheKeyboardIsOnThem checks that Tab brings back the controls a playing film hid, keeps them while the keyboard is on the title bar or the transport however long the pointer rests, and lets them fade again idleAfter after it leaves them.
func TestControlsShowWhileTheKeyboardIsOnThem(t *testing.T) {
	a, tt, _ := playerOpen(t)
	opacity := func() float32 { return tt.Find("id:film-transport").Style.Opacity }
	tt.HoverAt("id:film", ns.Point{X: 400, Y: 300})
	tt.Key("space")
	tt.Advance(3 * time.Second)
	if opacity() != 0 {
		t.Fatalf("the controls show after the pointer rested: opacity %v", opacity())
	}
	tt.Focus("id:film-back")
	if opacity() == 0 || tt.Find("id:film-top").Style.Opacity == 0 {
		t.Fatal("Tab onto 返回 left the controls hidden")
	}
	tt.Focus("id:film-full")
	tt.Advance(10 * time.Second)
	if !a.player.play.Playing() || opacity() == 0 {
		t.Fatalf("the controls hid with the keyboard on 全屏 (playing %v)", a.player.play.Playing())
	}
	// The next stop after the transport is in the side panel, out of the controls.
	tt.Key("tab")
	if tt.Find("id:film-transport").Style.Opacity == 0 {
		t.Fatal("the controls hid at once as the keyboard left them")
	}
	tt.Advance(3 * time.Second)
	if opacity() != 0 {
		t.Fatalf("the controls stayed after the keyboard left them: opacity %v", opacity())
	}
}

// TestScrubberSeeksOnRelease drags the scrubber and checks that the film seeks only once the pointer lets go, while the time shown follows the pointer meanwhile.
func TestScrubberSeeksOnRelease(t *testing.T) {
	a, tt, _ := playerOpen(t)
	bar := tt.Find("id:film-scrubber")
	w, y := bar.Frame.Size.Width, bar.Frame.Size.Height/2
	// nsgotest's path is offsets from where the drag starts, so the pointer goes from a quarter to three quarters of the bar
	tt.DragAt("id:film-scrubber", ns.Point{X: w * 0.25, Y: y}, ns.Point{X: w * 0.5})
	got := playerSeeks(tt)
	if len(got) != 1 {
		t.Fatalf("the drag seeked %v times, want once as it ended", got)
	}
	if at := got[0]; at < 8*time.Second || at > 10*time.Second {
		t.Fatalf("the drag to three quarters seeked to %v", at)
	}
	if head := a.PlayHead(); head != got[0] {
		t.Fatalf("head %v after the drag, want %v", head, got[0])
	}
}

// TestOpeningAFilmKeepsWhereItStopped opens a film that stopped at 0:06 last time from its poster, which holds it there, and goes back without playing it: the film still continues from 0:06, in the app and in library.json.
func TestOpeningAFilmKeepsWhereItStopped(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	path := filepath.Join(t.TempDir(), "library.json")
	// The library.json at path starts empty, so the film's job comes along: the library column shows only a film the shelf or the queue has (libraryOpenFilm)
	jobs := a.library.Jobs
	a.openLibrary(path)
	a.library.Jobs = jobs
	a.player.positions = map[string]time.Duration{item.VideoPath: 6 * time.Second}
	a.areasPositionsChanged()
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 0)
	tt.Press("id:film-back")
	if at := a.ResumeAt(item.VideoPath); at != 6*time.Second {
		t.Fatalf("ResumeAt after opening and going back = %v, want 6s", at)
	}
	if at := onDisk(t, path).Positions[item.VideoPath]; at != 6*time.Second {
		t.Fatalf("library.json keeps %v for the film, want 6s", at)
	}
}

// TestOpeningAFilmFromItsPosterResumesWhereItStopped opens a film that stopped at 0:06 last time from its poster in the grid, plays it on for two seconds, pauses and goes back: it opened held at 0:06, so it now resumes at 0:08 rather than at the 0:02 a start from 0:00 would have put in place of 0:06. A film stopped within 3 seconds of its start opens at the start, as the hero's 继续 does.
func TestOpeningAFilmFromItsPosterResumesWhereItStopped(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	apply(tt, a, func() { a.player.positions = map[string]time.Duration{item.VideoPath: 6 * time.Second} })
	tt.Press("id:film-" + item.Key)
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready && a.player.film.info.Folder != "" })
	if a.openKey != item.Key {
		t.Fatalf("pressing the poster opened %q", a.openKey)
	}
	if at := a.PlayHead(); at != 6*time.Second {
		t.Errorf("the film opened at %v, want where it stopped, 0:06", at)
	}
	playerLoadFilm(tt, a, 12*time.Second)
	if a.player.play.Playing() {
		t.Fatal("opening the film from its poster played it")
	}
	// The system's player reports where it plays from where it was asked to start, once there.
	start := a.PlayHead()
	playerPlayTo(tt, a, start)
	tt.Key("space")
	playerPlayTo(tt, a, start+2*time.Second)
	tt.Key("space")
	tt.Press("id:film-back")
	if at := a.ResumeAt(item.VideoPath); at != 8*time.Second {
		t.Fatalf("after opening from the poster and playing two seconds the film resumes at %v, want 0:08: the 0:06 it stopped at was lost", at)
	}

	apply(tt, a, func() { a.player.positions[item.VideoPath] = 2 * time.Second })
	tt.Press("id:film-" + item.Key)
	if at := a.PlayHead(); at != 0 {
		t.Fatalf("a film stopped at 0:02 opened at %v, want its start", at)
	}
}

// TestContinuingAFilmKeepsWhereItGotTo continues a film from 0:06, which opens it playing, lets it play on and goes back: where it got to past 0:06 is kept, in the app and in library.json.
func TestContinuingAFilmKeepsWhereItGotTo(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	path := filepath.Join(t.TempDir(), "library.json")
	// The library.json at path starts empty, so the film's job comes along: the library column shows only a film the shelf or the queue has (libraryOpenFilm)
	jobs := a.library.Jobs
	a.openLibrary(path)
	a.library.Jobs = jobs
	apply(tt, a, func() { a.OpenAt(item, 6*time.Second) })
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 6*time.Second)
	playerPlayTo(tt, a, 7*time.Second)
	if !a.player.play.Playing() {
		t.Fatal("the film opened at 0:06 is not playing")
	}
	tt.Press("id:film-back")
	if at := a.ResumeAt(item.VideoPath); at < 7*time.Second {
		t.Fatalf("ResumeAt after continuing and going back = %v, want past 0:07", at)
	}
	if at := onDisk(t, path).Positions[item.VideoPath]; at != a.ResumeAt(item.VideoPath) {
		t.Fatalf("library.json keeps %v for the film, want %v", at, a.ResumeAt(item.VideoPath))
	}
}

// TestQuittingStopsTheOpenFilm: when the app quits with a film playing, the film stops and where it stopped is kept for next time.
func TestQuittingStopsTheOpenFilm(t *testing.T) {
	a, tt, _ := playerOpen(t)
	path := a.player.play.path
	tt.Key("space")
	playerPlayTo(tt, a, 3*time.Second)
	a.areasQuitting()
	if a.player.play != nil {
		t.Fatal("the film still plays after the app quit")
	}
	if a.ResumeAt(path) < 3*time.Second {
		t.Fatalf("where the film stopped was not kept: %v", a.player.positions)
	}
}

// TestThePlayerKeepsTheDisplayAwakeWhileAFilmPlays: a film held on its first frame takes no hold; playing it keeps the display awake, and pausing it, playing it to its end, going back to the library and quitting each let the display sleep again.
func TestThePlayerKeepsTheDisplayAwakeWhileAFilmPlays(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	awake := &fakeAwake{}
	a.player.keepAwake = awake.keepAwake
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	if len(awake.holds) != 0 {
		t.Fatalf("a film held on its first frame took holds %+v", awake.holds)
	}
	play := func(when string) {
		t.Helper()
		tt.Key("space")
		if !a.player.play.Playing() || awake.held != 1 {
			t.Fatalf("%s: playing %v, holds %+v, %d held", when, a.player.play.Playing(), awake.holds, awake.held)
		}
	}
	play("pressing Space")
	if hold := awake.holds[0]; hold.reason != "正在播放视频" || !hold.display {
		t.Fatalf("the hold is %+v, want the reason 正在播放视频 and the display kept on", hold)
	}
	playerPlayTo(tt, a, 2*time.Second)
	if len(awake.holds) != 1 {
		t.Fatalf("the playing film took another hold: %+v", awake.holds)
	}
	tt.Key("space")
	if awake.held != 0 {
		t.Fatalf("after pausing: holds %+v, %d held", awake.holds, awake.held)
	}
	play("playing again")
	tt.Progress("id:film-video", 12*time.Second, 12*time.Second)
	refresh(tt, a)
	if a.player.play.Playing() || awake.held != 0 {
		t.Fatalf("after the film's end: holds %+v, %d held", awake.holds, awake.held)
	}
	play("playing from the start")
	tt.Press("id:film-back")
	if a.player.play != nil || awake.held != 0 {
		t.Fatalf("after going back to the library: holds %+v, %d held", awake.holds, awake.held)
	}
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	play("playing the film opened again")
	a.areasQuitting()
	if awake.held != 0 {
		t.Fatalf("after quitting: holds %+v, %d held", awake.holds, awake.held)
	}
}

// TestTheDisplayHoldFollowsTheWindow: a playing film lets the display sleep while none of the app shows or the main window is put away, as only its sound is heard, and keeps it awake again once the window is back; a seek keeps the one hold.
func TestTheDisplayHoldFollowsTheWindow(t *testing.T) {
	a, tt, _ := playerOpen(t)
	awake := &fakeAwake{}
	a.player.keepAwake = awake.keepAwake
	tt.Key("space")
	if awake.held != 1 {
		t.Fatalf("holds %+v, %d held", awake.holds, awake.held)
	}
	apply(tt, a, func() { a.playerWindowAway(true) })
	if awake.held != 0 || len(awake.holds) != 1 {
		t.Fatalf("with the window away: holds %+v, %d held", awake.holds, awake.held)
	}
	apply(tt, a, func() { a.playerWindowAway(false) })
	if awake.held != 1 || len(awake.holds) != 2 {
		t.Fatalf("with the window back: holds %+v, %d held", awake.holds, awake.held)
	}
	apply(tt, a, func() { a.areasWindowShown(false) })
	if awake.held != 0 {
		t.Fatalf("with the window put away: holds %+v, %d held", awake.holds, awake.held)
	}
	apply(tt, a, func() { a.areasWindowShown(true) })
	tt.Key("right")
	if awake.held != 1 || len(awake.holds) != 3 {
		t.Fatalf("after the seek: holds %+v, %d held", awake.holds, awake.held)
	}
}

// TestVolumeKeys checks ↑, ↓ and M on an open film: ↑ and ↓ move the volume a tenth within 0 and 100%, M mutes and gives the volume back, and each says so on the picture. The system's player plays at the new volume at once, muted for a volume of 0, and once the keys rest the settings keep it.
func TestVolumeKeys(t *testing.T) {
	a, tt, updates := playerOpen(t)
	key := func(k string, volume float64, muted bool, osd string) {
		t.Helper()
		tt.Key(k)
		if math.Abs(a.player.volume-volume) > 1e-9 || a.player.muted != muted || !hasText(tt, osd) {
			t.Fatalf("after %s: volume %v muted %v, want %v %v; texts %q", k, a.player.volume, a.player.muted, volume, muted, tt.Texts())
		}
		if v := playerVideoConfig(tt); v.Muted != (muted || volume == 0) || !muted && volume > 0 && math.Abs(float64(v.Volume)-volume) > 1e-6 {
			t.Fatalf("after %s the player plays %+v", k, v)
		}
	}
	key("up", 1, false, "音量 100%")
	key("down", 0.9, false, "音量 90%")
	key("down", 0.8, false, "音量 80%")
	key("m", 0.8, true, "静音")
	if got := tt.Find("id:film-mute").Props.Label; got != "取消静音" {
		t.Fatalf("the speaker is named %q while muted", got)
	}
	key("m", 0.8, false, "音量 80%")
	key("m", 0.8, true, "静音")
	// ↑ while muted unmutes, a step above the volume the mute kept.
	key("up", 0.9, false, "音量 90%")
	waitUntil(t, tt, a, updates, func() bool { return !a.player.volumeUnsaved })
	saved := loadSavedSettings(a.settingsPath)
	if saved.Volume == nil || math.Abs(*saved.Volume-0.9) > 1e-9 || saved.Muted {
		t.Fatalf("saved volume %v muted %v", saved.Volume, saved.Muted)
	}
	for range 12 {
		tt.Key("down")
	}
	if a.player.volume != 0 || !hasText(tt, "音量 0%") || !playerVideoConfig(tt).Muted {
		t.Fatalf("volume %v after ↓ to the bottom; player %+v", a.player.volume, playerVideoConfig(tt))
	}
	// M on a volume of 0 makes it heard rather than muting it.
	key("m", 0.5, false, "音量 50%")
}

// TestMuteButtonMutes clicks the transport's speaker: it mutes the film and turns into 取消静音, which gives the volume back.
func TestMuteButtonMutes(t *testing.T) {
	a, tt, _ := playerOpen(t)
	tt.Press("id:film-mute")
	if !a.player.muted || !playerVideoConfig(tt).Muted {
		t.Fatal("the speaker did not mute the film")
	}
	tt.Press("id:film-mute")
	if a.player.muted || a.player.volume != 1 || playerVideoConfig(tt).Muted || playerVideoConfig(tt).Volume != 1 {
		t.Fatalf("muted %v volume %v after 取消静音", a.player.muted, a.player.volume)
	}
}

// TestVolumeSettingsRoundTrip checks that the volume and mute are kept in the settings, and that a volume out of range is brought back within 0 and 1.
func TestVolumeSettingsRoundTrip(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.player.volume = 0.3
	a.player.muted = true
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}
	b, _, _ := newTestApp(t, &fakePipeline{})
	b.restorePlayer(loadSavedSettings(a.settingsPath))
	if b.player.volume != 0.3 || !b.player.muted || b.playerVolume() != 0 {
		t.Fatalf("restored volume %v muted %v", b.player.volume, b.player.muted)
	}
	loud := 3.0
	c, _, _ := newTestApp(t, &fakePipeline{})
	c.restorePlayer(savedSettings{savedPlayer: savedPlayer{Volume: &loud}})
	if c.player.volume != 1 || c.player.muted {
		t.Fatalf("volume %v muted %v from a saved 3", c.player.volume, c.player.muted)
	}
	d, _, _ := newTestApp(t, &fakePipeline{})
	d.restorePlayer(savedSettings{})
	if d.player.volume != 1 || d.player.muted {
		t.Fatalf("default volume %v muted %v", d.player.volume, d.player.muted)
	}
}

// TestQuittingKeepsTheVolume: a volume changed too shortly before quitting for the change to have settled is still kept in the settings.
func TestQuittingKeepsTheVolume(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.ToggleMute()
	a.areasQuitting()
	if saved := loadSavedSettings(a.settingsPath); !saved.Muted {
		t.Fatalf("the mute was not kept: %+v", saved.savedPlayer)
	}
}

// TestQuittingKeepsTheSpeed: a speed changed too shortly before quitting for the change to have settled is still kept in the settings.
func TestQuittingKeepsTheSpeed(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.SetRate(0.75)
	a.areasQuitting()
	if saved := loadSavedSettings(a.settingsPath); saved.Rate == nil || *saved.Rate != 0.75 {
		t.Fatalf("the speed was not kept: %v", saved.Rate)
	}
}

// fakeScreen stands in for the main window's full screen, which tests have no window for.
type fakeScreen struct{ full bool }

func (s *fakeScreen) IsFullScreen() bool   { return s.full }
func (s *fakeScreen) SetFullScreen(v bool) { s.full = v }
func (s *fakeScreen) ToggleFullScreen()    { s.full = !s.full }

// TestEscapeLeavesFullScreenFirst checks that Esc on a film in full screen leaves full screen and keeps the film open, and that the next Esc goes back to the library.
func TestEscapeLeavesFullScreenFirst(t *testing.T) {
	a, tt, _ := playerOpen(t)
	screen := &fakeScreen{}
	a.player.screen = screen
	tt.Key("f")
	if !screen.full {
		t.Fatal("F did not enter full screen")
	}
	tt.Key("escape")
	if screen.full || a.openKey == "" {
		t.Fatalf("first Escape: full screen %v open %q", screen.full, a.openKey)
	}
	tt.Key("escape")
	if screen.full || a.openKey != "" {
		t.Fatalf("second Escape: full screen %v open %q", screen.full, a.openKey)
	}
}

// TestSubtitlesDragOffBurnedInLines drags the lines on the picture up, as off a film's own burned-in subtitles, and checks that they stay where the pointer lets them go, as a share of the picture's height kept for the film in library.json and not in the settings; that a click on them starts or holds the film; and that a double click puts them back, after which a double click on them enters full screen, as on the picture.
func TestSubtitlesDragOffBurnedInLines(t *testing.T) {
	a, tt, _ := playerOpen(t)
	screen := &fakeScreen{}
	a.player.screen = screen
	playerPlayTo(tt, a, 1500*time.Millisecond)
	lines := func() ns.Bounds { return pageFrame(tt.Find("id:film-subtitle-lines")) }
	place := lines()
	// The held film shows its controls, so the lines yield 60 points to them, and the drag takes them from there.
	tt.Pan("id:film-subtitle-lines", ns.Point{Y: -20}, ns.Point{Y: -60}, ns.Point{Y: -100})
	if want := float32(160) / 720; math.Abs(float64(a.SubLift()-want)) > 0.002 || a.player.play.Playing() || screen.full {
		t.Fatalf("lift %v, want %v; playing %v full screen %v", a.SubLift(), want, a.player.play.Playing(), screen.full)
	}
	a.playerSaveDelay()
	if got := a.library.Lifts[a.player.film.video]; got != a.SubLift() {
		t.Fatalf("the shelf keeps the film's lift at %v, want %v", got, a.SubLift())
	}
	if data, err := os.ReadFile(a.settingsPath); err == nil && strings.Contains(string(data), "Lift") {
		t.Fatalf("the settings keep a place for every film: %s", data)
	}
	// They were 60 points up, out of the controls' way, and are 160 up now.
	if moved := place.Origin.Y - lines().Origin.Y; math.Abs(float64(moved-100)) > 0.5 {
		t.Fatalf("the lines moved %v points up, want 100 more than the 60 they yielded", moved)
	}
	tt.Press("id:film-subtitle-lines")
	if !a.player.play.Playing() {
		t.Fatal("a click on the lines did not start the film")
	}
	tt.Key("space")
	double := func() {
		t.Helper()
		tt.Advance(time.Second)
		tt.Press("id:film-subtitle-lines")
		tt.Press("id:film-subtitle-lines")
	}
	double()
	if a.SubLift() != 0 || screen.full || a.player.play.Playing() {
		t.Fatalf("a double click left lift %v; full screen %v playing %v", a.SubLift(), screen.full, a.player.play.Playing())
	}
	a.playerSaveDelay()
	if got, ok := a.library.Lifts[a.player.film.video]; ok {
		t.Fatalf("lines put back in their place are still kept at %v", got)
	}
	double()
	if !screen.full {
		t.Fatalf("a double click on lines in their place did not enter full screen; lines at %v", place)
	}
}

// TestSubtitlePlaceStaysWithItsFilm moves the subtitles of one film and checks that they are there again when it opens again, in this launch and the next, that another film opens with its own at the look's place, that a run of ⌥↑ presses is saved once it rests, and that ⌥0's reset is forgotten on disk.
func TestSubtitlePlaceStaysWithItsFilm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	filmA := films.Film{Key: "a", VideoPath: "/films/a.mp4"}
	filmB := films.Film{Key: "b", VideoPath: "/films/b.mp4"}
	near := func(got, want float32) bool { return math.Abs(float64(got-want)) < 1e-4 }

	a.Open(filmA)
	a.player.subRoom = film.SubRoom{Lo: -0.05, Hi: 0.6}
	start := time.Date(2026, 11, 16, 20, 0, 0, 0, time.UTC)
	a.playerNudgeLift(1)
	a.playerNudgeLift(1)
	a.playerKeep(start)
	a.playerNudgeLift(1)
	a.playerKeep(start.Add(500 * time.Millisecond))
	if got, ok := onDisk(t, path).Lifts[filmA.VideoPath]; ok {
		t.Fatalf("the place was saved at %v before it rested", got)
	}
	a.playerKeep(start.Add(1500 * time.Millisecond))
	if got := onDisk(t, path).Lifts[filmA.VideoPath]; !near(got, 0.06) {
		t.Fatalf("library.json keeps film A's subtitles at %v once they rested, want 0.06", got)
	}
	a.closeFilm()

	a.Open(filmB)
	if got := a.SubLift(); got != 0 {
		t.Fatalf("film B opens with film A's place %v", got)
	}
	a.closeFilm()
	if lib := onDisk(t, path); len(lib.Lifts) != 1 {
		t.Fatalf("library.json lifts %v, want film A's alone", lib.Lifts)
	}
	a.Open(filmA)
	if got := a.SubLift(); !near(got, 0.06) {
		t.Fatalf("film A opens again with its subtitles at %v, want 0.06", got)
	}

	again, _, _ := newTestApp(t, &fakePipeline{})
	again.openLibrary(path)
	again.Open(filmA)
	if got := again.SubLift(); !near(got, 0.06) {
		t.Fatalf("after a restart film A opens with its subtitles at %v, want 0.06", got)
	}
	again.keyRun(again.cx, "alt+0")()
	if got := again.SubLift(); got != 0 {
		t.Fatalf("⌥0 left the subtitles at %v", got)
	}
	again.closeFilm()
	if got, ok := onDisk(t, path).Lifts[filmA.VideoPath]; ok {
		t.Fatalf("subtitles put back in their place are still kept at %v", got)
	}
}

// TestSubtitlePlaceFollowsAMovedVideo checks that the place of the subtitles of a video that moved on disk is kept by its new path, with the film open and in library.json.
func TestSubtitlePlaceFollowsAMovedVideo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	a.Open(films.Film{Key: "a", VideoPath: "/old/a.mp4"})
	a.SetSubLift(0.2)
	a.playerSaveDelay()
	moved := func(p string) (string, bool) {
		if p == "/old/a.mp4" {
			return "/new/a.mp4", true
		}
		return "", false
	}
	if !a.moveVideos(moved) {
		t.Fatal("moving the video changed nothing in library.json")
	}
	if got := a.SubLift(); got != 0.2 {
		t.Fatalf("the open film's subtitles are at %v after its video moved, want 0.2", got)
	}
	a.SetSubLift(0.3)
	a.closeFilm()
	lib := onDisk(t, path)
	if len(lib.Lifts) != 1 || lib.Lifts["/new/a.mp4"] != 0.3 {
		t.Fatalf("library.json lifts %v", lib.Lifts)
	}
}

// TestSubtitlePlaceKeys checks ⌥↑, ⌥↓ and ⌥0 on the lines of a held film: the first ⌥↑ moves them on from where they yield to the controls, each press a fiftieth of the picture, ⌥↓ brings them back down, a run of ⌥↑ stops where a drag stops, 8 points inside the picture, and ⌥0 puts them back. The keys are listed in the menu bar's 字幕 menu and on the cheat sheet.
func TestSubtitlePlaceKeys(t *testing.T) {
	a, tt, _ := playerOpen(t)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	// shown is how far up from their place the lines show, the held film's yielding 60 points to the controls at first.
	yielded := pageFrame(tt.Find("id:film-subtitle-lines")).Origin.Y
	shown := func() ns.Points { return yielded + 60 - pageFrame(tt.Find("id:film-subtitle-lines")).Origin.Y }
	tt.Key("alt+up")
	if got, want := shown(), ns.Points(60+0.02*720); math.Abs(float64(got-want)) > 0.5 {
		t.Fatalf("⌥↑ shows the lines %v points up, want %v", got, want)
	}
	tt.Key("alt+down")
	tt.Key("alt+down")
	if got, want := shown(), ns.Points(60-0.02*720); math.Abs(float64(got-want)) > 0.5 {
		t.Fatalf("⌥↑ then ⌥↓ twice shows the lines %v points up, want %v", got, want)
	}
	for range 60 {
		tt.Key("alt+up")
	}
	if top := pageFrame(tt.Find("id:film-subtitle-lines")).Origin.Y - pageFrame(tt.Find("id:film")).Origin.Y; math.Abs(float64(top-8)) > 0.5 {
		t.Fatalf("a run of ⌥↑ took the lines to %v points from the picture's top, want the 8 a drag keeps", top)
	}
	if got := a.SubLift(); got != a.player.subRoom.Hi {
		t.Fatalf("a run of ⌥↑ left the lift at %v, want the room's top %v", got, a.player.subRoom.Hi)
	}
	tt.Key("alt+0")
	if got := shown(); a.SubLift() != 0 || math.Abs(float64(got-60)) > 0.5 {
		t.Fatalf("⌥0 left lift %v, the lines %v points up", a.SubLift(), got)
	}

	a.installMenu(a.cx)
	refresh(tt, a)
	msgs := testlocale.Messages()
	for _, label := range []string{msgs.PlayerKeySubtitlesUp(), msgs.PlayerKeySubtitlesDown(), msgs.PlayerKeySubtitlesReset()} {
		findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupSubtitles(), label)
	}
	var sheet []string
	for _, g := range a.cheatsheetGroups(a.cx) {
		for _, k := range g.Keys {
			sheet = append(sheet, k.Label+" "+strings.Join(k.Combos, ","))
		}
	}
	for _, want := range []string{msgs.CheatsheetSubtitlesPosition() + " alt+up,alt+down", msgs.PlayerKeySubtitlesReset() + " alt+0"} {
		if !slices.Contains(sheet, want) {
			t.Fatalf("the cheat sheet lacks %q: %q", want, sheet)
		}
	}
}

// TestMovedSubtitlesTakeThePointerWhereTheyShow moves the lines up a quarter of the picture and checks that the pointer finds them where they show, as the platforms' hit tests go by frames: a double click there puts them back rather than entering full screen, as one on the picture would.
func TestMovedSubtitlesTakeThePointerWhereTheyShow(t *testing.T) {
	a, tt, _ := playerOpen(t)
	screen := &fakeScreen{}
	a.player.screen = screen
	playerPlayTo(tt, a, 1500*time.Millisecond)
	apply(tt, a, func() { a.SetSubLift(0.25) })
	lines := tt.Find("id:film-subtitle-lines")
	box, film := pageFrame(lines), pageFrame(tt.Find("id:film"))
	at := ns.Point{X: box.Origin.X + box.Size.Width/2 - film.Origin.X, Y: box.Origin.Y + lines.Style.Transform.TranslateY + box.Size.Height/2 - film.Origin.Y}
	tt.PressAt("id:film", at)
	tt.PressAt("id:film", at)
	if a.SubLift() != 0 || screen.full {
		t.Fatalf("a double click where the moved lines show left lift %v, full screen %v", a.SubLift(), screen.full)
	}
}

// TestSubtitlesShowTheyCanBeGrabbed checks that the pointer over the lines puts a backdrop of the theme's behind them, without moving them, and that it goes once the pointer leaves.
func TestSubtitlesShowTheyCanBeGrabbed(t *testing.T) {
	a, tt, _ := playerOpen(t)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	before := pageFrame(tt.Find("id:film-subtitle-lines"))
	if bg := tt.Find("id:film-subtitle-lines").Style.Background; bg != 0 {
		t.Fatalf("the lines show a backdrop %#x before the pointer is over them", uint32(bg))
	}
	tt.Hover("id:film-subtitle-lines")
	lines := tt.Find("id:film-subtitle-lines")
	if lines.Style.Background == 0 || lines.Style.Cursor != ns.CursorGrab {
		t.Fatalf("the lines under the pointer show backdrop %#x and cursor %v, want a backdrop and the grab cursor", uint32(lines.Style.Background), lines.Style.Cursor)
	}
	if after := pageFrame(lines); after != before {
		t.Fatalf("the backdrop moved the lines from %v to %v", before, after)
	}
	tt.HoverAt("id:film", ns.Point{X: 20, Y: 200})
	if bg := tt.Find("id:film-subtitle-lines").Style.Background; bg != 0 {
		t.Fatalf("the backdrop %#x stays once the pointer left the lines", uint32(bg))
	}
}

// TestSubtitleTipShowsOnce checks that the tip that the subtitles can be dragged does not show while the first film is held, shows once it plays with subtitles, goes after a moment, and is kept in the settings as seen, so it shows neither when the film plays again nor after a restart; and that dragging the lines takes it away at once.
func TestSubtitleTipShowsOnce(t *testing.T) {
	a, tt, _ := playerOpen(t)
	msgs := testlocale.Messages()
	if tt.Exists("id:film-subtitle-tip") {
		t.Fatal("the tip shows while the film is held")
	}
	tt.Key("space")
	if !tt.Exists("id:film-subtitle-tip") || !hasText(tt, msgs.FilmSubtitleTip()) {
		t.Fatalf("the tip does not show as the film plays: %q", tt.Texts())
	}
	if saved := loadSavedSettings(a.settingsPath); !saved.SubtitleTipSeen {
		t.Fatal("the settings do not keep that the tip was seen")
	}
	tt.Advance(playerTipFor)
	refresh(tt, a)
	if tt.Exists("id:film-subtitle-tip") {
		t.Fatal("the tip stays")
	}
	tt.Key("space")
	tt.Key("space")
	if tt.Exists("id:film-subtitle-tip") {
		t.Fatal("the tip shows again as the film plays again")
	}

	b, btt, bupdates, item := playerFilm(t, 12, nil)
	b.settingsPath = a.settingsPath
	b.restoreSettings(loadSavedSettings(b.settingsPath))
	playerOpenFilm(t, b, btt, bupdates, item)
	playerLoadFilm(btt, b, 12*time.Second)
	btt.Key("space")
	if btt.Exists("id:film-subtitle-tip") {
		t.Fatal("the tip shows again after a restart")
	}

	c, ctt, _ := playerOpen(t)
	ctt.Key("space")
	playerPlayTo(ctt, c, 1500*time.Millisecond)
	if !ctt.Exists("id:film-subtitle-tip") {
		t.Fatal("the tip does not show for a first film")
	}
	ctt.Pan("id:film-subtitle-lines", ns.Point{Y: -20}, ns.Point{Y: -40})
	if ctt.Exists("id:film-subtitle-tip") {
		t.Fatal("the tip stays once the lines were dragged")
	}
}

// TestDoubleClickOnThePictureTogglesFullScreen checks that a double click on the picture enters full screen and the next one leaves it, while the film plays on: the double click's two clicks start and hold it, which cancel out.
func TestDoubleClickOnThePictureTogglesFullScreen(t *testing.T) {
	a, tt, _ := playerOpen(t)
	screen := &fakeScreen{}
	a.player.screen = screen
	at := ns.Point{X: 200, Y: 250}
	tt.Advance(time.Second)
	tt.PressAt("id:film-stage", at)
	if !a.player.play.Playing() || screen.full {
		t.Fatalf("a click: playing %v full screen %v", a.player.play.Playing(), screen.full)
	}
	double := func(want bool) {
		t.Helper()
		tt.Advance(time.Second)
		tt.PressAt("id:film-stage", at)
		tt.PressAt("id:film-stage", at)
		if screen.full != want || !a.player.play.Playing() {
			t.Fatalf("double click: full screen %v, want %v; playing %v", screen.full, want, a.player.play.Playing())
		}
	}
	double(true)
	double(false)
}

// onDisk is the library.json at path as the next launch would read it.
func onDisk(t *testing.T, path string) shelf.Library {
	t.Helper()
	lib, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

// TestPlayingFilmIsKeptOnDisk plays a film without closing it and checks that where it got to reaches library.json now and then, so a crash or a force quit does not lose it: not on every report of where it is, but once it has played on for playerKeepEvery, at once when it pauses, and at once where the user's seek lands. A film held on its first frame is not kept, nor a looped line's return.
func TestPlayingFilmIsKeptOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a := openStill(t, path, "clip", "/v.mp4", 0)
	play := a.player.play
	play.duration = 10 * time.Minute
	play.paused = false
	start := time.Date(2026, 11, 10, 20, 0, 0, 0, time.UTC)
	at := func(head, since time.Duration) {
		t.Helper()
		play.head, play.headAt = head, start.Add(since)
		play.clock = func() time.Time { return start.Add(since) }
		a.playerKeep(start.Add(since))
	}
	want := func(head time.Duration, why string) {
		t.Helper()
		if got := onDisk(t, path).Positions["/v.mp4"]; got != head {
			t.Fatalf("%s: library.json keeps %v, want %v", why, got, head)
		}
	}
	at(0, 0)
	at(5*time.Second, 5*time.Second)
	at(19*time.Second, 19*time.Second)
	want(0, "before playerKeepEvery")
	at(20*time.Second, 20*time.Second)
	want(20*time.Second, "after playerKeepEvery")
	at(21*time.Second, 21*time.Second)
	at(39*time.Second, 39*time.Second)
	want(20*time.Second, "a report after a save")

	play.toggle()
	at(40*time.Second, 40*time.Second)
	want(40*time.Second, "paused")
	at(40*time.Second, 80*time.Second)
	want(40*time.Second, "held")

	a.seekPlayback(5 * time.Minute)
	a.playerKeep(start.Add(81 * time.Second))
	want(5*time.Minute, "a seek")

	a.seekTo(time.Minute, false)
	a.playerKeep(start.Add(82 * time.Second))
	want(5*time.Minute, "a looped line's return")

	poster := &filmPlay{path: "/v.mp4", loaded: true, head: 3 * time.Second, duration: 10 * time.Minute, paused: true}
	a.player.play = poster
	a.playerKeep(start.Add(2 * time.Minute))
	a.playerKeep(start.Add(5 * time.Minute))
	want(5*time.Minute, "a film held on its first frame")

	ending := &filmPlay{path: "/v.mp4", loaded: true, head: 9 * time.Minute, duration: 10 * time.Minute, played: true}
	a.player.play = ending
	a.playerKeep(start.Add(6 * time.Minute))
	want(9*time.Minute, "a film opened in its last minute")
	ending.finish()
	a.playerKeep(start.Add(7 * time.Minute))
	if got, ok := onDisk(t, path).Positions["/v.mp4"]; ok {
		t.Fatalf("a film played to its end is kept at %v, want it to start over", got)
	}
}

// TestSubtitleDelayStaysWithItsFilm moves the subtitles of one film and checks that they are moved again when it opens again, in this launch and the next, that another film opens with its own in place, and that a run of Z presses is saved once it rests rather than on every press.
func TestSubtitleDelayStaysWithItsFilm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	filmA := films.Film{Key: "a", VideoPath: "/films/a.mp4"}
	filmB := films.Film{Key: "b", VideoPath: "/films/b.mp4"}

	a.Open(filmA)
	start := time.Date(2026, 11, 10, 20, 0, 0, 0, time.UTC)
	a.nudgeDelay(-100 * time.Millisecond)
	a.nudgeDelay(-100 * time.Millisecond)
	a.playerKeep(start)
	a.nudgeDelay(-100 * time.Millisecond)
	a.playerKeep(start.Add(500 * time.Millisecond))
	a.playerKeep(start.Add(1400 * time.Millisecond))
	if d, ok := onDisk(t, path).Delays[filmA.VideoPath]; ok {
		t.Fatalf("the delay was saved at %v before it rested", d)
	}
	a.playerKeep(start.Add(1500 * time.Millisecond))
	if d := onDisk(t, path).Delays[filmA.VideoPath]; d != -300*time.Millisecond {
		t.Fatalf("library.json keeps a delay of %v once it rested, want -0.3s", d)
	}

	a.SetDelay(-500 * time.Millisecond)
	a.closeFilm()
	if d := onDisk(t, path).Delays[filmA.VideoPath]; d != -500*time.Millisecond {
		t.Fatalf("library.json keeps a delay of %v after the film closed, want -0.5s", d)
	}
	a.Open(filmB)
	if d := a.Delay(); d != 0 {
		t.Fatalf("film B opens with film A's delay %v", d)
	}
	a.Open(filmA)
	if d := a.Delay(); d != -500*time.Millisecond {
		t.Fatalf("film A opens again with a delay of %v, want -0.5s", d)
	}

	again, _, _ := newTestApp(t, &fakePipeline{})
	again.openLibrary(path)
	again.Open(filmA)
	if d := again.Delay(); d != -500*time.Millisecond {
		t.Fatalf("after a restart film A opens with a delay of %v, want -0.5s", d)
	}
	again.SetDelay(0)
	again.closeFilm()
	if d, ok := onDisk(t, path).Delays[filmA.VideoPath]; ok {
		t.Fatalf("a delay moved back in place is still kept: %v", d)
	}
}

// TestSubtitleDelaySavesOnceItRests: a delay changed while the film is held, which reports nothing, is saved once it has rested, by a timer of its own.
func TestSubtitleDelaySavesOnceItRests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	a.Open(films.Film{Key: "a", VideoPath: "/films/a.mp4"})
	a.nudgeDelay(200 * time.Millisecond)
	a.nudgeDelay(200 * time.Millisecond)
	waitUntil(t, tt, a, updates, func() bool { return !a.player.saves.delayDirty })
	if d := onDisk(t, path).Delays["/films/a.mp4"]; d != 400*time.Millisecond {
		t.Fatalf("library.json keeps a delay of %v, want 0.4s", d)
	}
}

// TestSubtitleDelayFollowsAMovedVideo checks that the delay of a video that moved on disk is kept by its new path, with the film open and in library.json.
func TestSubtitleDelayFollowsAMovedVideo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	a.Open(films.Film{Key: "a", VideoPath: "/old/a.mp4"})
	a.SetDelay(700 * time.Millisecond)
	a.playerSaveDelay()
	moved := func(p string) (string, bool) {
		if p == "/old/a.mp4" {
			return "/new/a.mp4", true
		}
		return "", false
	}
	if !a.moveVideos(moved) {
		t.Fatal("moving the video changed nothing in library.json")
	}
	a.SetDelay(800 * time.Millisecond)
	a.closeFilm()
	lib := onDisk(t, path)
	if len(lib.Delays) != 1 || lib.Delays["/new/a.mp4"] != 800*time.Millisecond {
		t.Fatalf("library.json delays %v", lib.Delays)
	}
}

// TestSubtitleDelayStaysWithAFilmOpenedPlaying opens films playing (OpenAt), and checks that each opens with its own subtitle delay and keeps a change by its own video, while a jump to a line of the open film keeps the film as it is, as a seek does.
func TestSubtitleDelayStaysWithAFilmOpenedPlaying(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.openLibrary(path)
	filmA := films.Film{Key: "a", VideoPath: "/films/a.mp4"}
	filmB := films.Film{Key: "b", VideoPath: "/films/b.mp4"}

	a.Open(filmB)
	a.SetDelay(700 * time.Millisecond)
	a.closeFilm()
	for _, at := range []time.Duration{0, 30 * time.Second} {
		a.OpenAt(filmA, at)
		if d := a.Delay(); d != 0 {
			t.Fatalf("film A opened at %v with film B's delay %v", at, d)
		}
		if a.player.film.video != filmA.VideoPath {
			t.Fatalf("film A opened at %v keeps its delay by %q", at, a.player.film.video)
		}
		a.closeFilm()
	}

	a.OpenAt(filmA, 0)
	a.SetDelay(-200 * time.Millisecond)
	a.closeFilm()
	lib := onDisk(t, path)
	if lib.Delays[filmA.VideoPath] != -200*time.Millisecond || lib.Delays[filmB.VideoPath] != 700*time.Millisecond {
		t.Fatalf("library.json delays %v, want A -0.2s and B 0.7s", lib.Delays)
	}

	a.OpenAt(filmB, 30*time.Second)
	if d := a.Delay(); d != 700*time.Millisecond {
		t.Fatalf("film B opened playing with a delay of %v, want 0.7s", d)
	}
	*a.Search() = "hello"
	a.OpenAt(filmB, 40*time.Second)
	if d, q := a.Delay(), *a.Search(); d != 700*time.Millisecond || q != "hello" {
		t.Fatalf("a jump within film B left delay %v and search %q", d, q)
	}
	a.OpenAt(filmA, 30*time.Second)
	if d, q := a.Delay(), *a.Search(); d != -200*time.Millisecond || q != "" {
		t.Fatalf("film A opened from film B with delay %v and search %q", d, q)
	}
}

// TestPlayerNamesTheSystemsFileManager opens a film on each system and checks that its 在访达中显示 names that system's file manager, in the title bar and on the 信息 tab, and that the style view's font menu offers that system's fonts.
func TestPlayerNamesTheSystemsFileManager(t *testing.T) {
	for _, tc := range []struct {
		os     ns.OS
		reveal string
		fonts  []string
	}{
		{ns.OSMacOS, "在访达中显示", []string{"默认", "苹方", "冬青黑体"}},
		{ns.OSWindows, "在文件资源管理器中显示", []string{"默认", "黑体", "宋体"}},
		{ns.OSLinux, "在文件管理器中显示", []string{"默认", "黑体", "宋体"}},
	} {
		t.Run(platformNameOf(tc.os), func(t *testing.T) {
			usePlatform(t, tc.os)
			a, tt, updates, item := playerFilm(t, 12, nil)
			playerOpenFilm(t, a, tt, updates, item)
			if got := tt.Find("id:film-reveal").Props.Label; got != tc.reveal {
				t.Fatalf("the title bar's reveal is named %q, want %q", got, tc.reveal)
			}
			tt.Press("id:film-tab-info")
			wantText(t, tt, tc.reveal)
			apply(tt, a, func() { a.SetStyleView(true) })
			tt.Press("id:film-style-font")
			for _, font := range tc.fonts {
				wantText(t, tt, font)
			}
			pick := tc.fonts[len(tc.fonts)-1]
			tt.Press(whole(pick))
			if look := a.player.look; look.Translation.Family != pick || look.Original.Family != pick {
				t.Fatalf("look %+v after %s", look, pick)
			}
		})
	}
}

// platformNameOf is the catalogs' name of os, for a subtest.
func platformNameOf(os ns.OS) string {
	switch os {
	case ns.OSWindows:
		return "Windows"
	case ns.OSLinux:
		return "Linux"
	}
	return "macOS"
}

// nowPlayingRecorder stands in for the system's center (nowplaying.System): it keeps what the player shows the system, in order, and sends commands as the media keys and the system's controls would.
type nowPlayingRecorder struct {
	events  []nowPlayingEvent
	command func(nowplaying.Command, time.Duration)
}

// nowPlayingEvent is one call the player made: "set" with info, "playback" with state and at, or "clear".
type nowPlayingEvent struct {
	kind  string
	info  nowplaying.Info
	state nowplaying.State
	at    time.Duration
}

func (r *nowPlayingRecorder) Set(info nowplaying.Info) {
	r.events = append(r.events, nowPlayingEvent{kind: "set", info: info})
}

func (r *nowPlayingRecorder) SetPlayback(state nowplaying.State, at time.Duration) {
	r.events = append(r.events, nowPlayingEvent{kind: "playback", state: state, at: at})
}

func (r *nowPlayingRecorder) Clear() { r.events = append(r.events, nowPlayingEvent{kind: "clear"}) }

func (r *nowPlayingRecorder) OnCommand(fn func(nowplaying.Command, time.Duration)) func() {
	r.command = fn
	return func() { r.command = nil }
}

func (r *nowPlayingRecorder) last(t *testing.T) nowPlayingEvent {
	t.Helper()
	if len(r.events) == 0 {
		t.Fatal("the player told the system nothing")
	}
	return r.events[len(r.events)-1]
}

func (r *nowPlayingRecorder) count(kind string) int {
	n := 0
	for _, e := range r.events {
		if e.kind == kind {
			n++
		}
	}
	return n
}

// TestNowPlayingFollowsTheOpenFilm checks that a film the system's player loaded shows where the system shows what plays, with its library name and length, held; that Space, the media keys and the system's scrubber play, pause and move it and the system learns each; that a seek keeps the film shown; and that closing the film takes it off.
func TestNowPlayingFollowsTheOpenFilm(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 5, nil)
	rec := &nowPlayingRecorder{}
	a.playerNowPlayingConnect(rec)
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 5*time.Second)
	shown := rec.last(t)
	if shown.kind != "set" || shown.info.Title != "talk" || shown.info.Duration != 5*time.Second || shown.info.State != nowplaying.Paused || shown.info.Position != 0 || shown.info.Commands != nowplaying.Play|nowplaying.Pause|nowplaying.Seek {
		t.Fatalf("opening showed %+v", shown)
	}
	tt.Key("space")
	if e := rec.last(t); e.kind != "playback" || e.state != nowplaying.Playing {
		t.Fatalf("Space told the system %+v", e)
	}
	press := func(cmd nowplaying.Command, at time.Duration) {
		t.Helper()
		before := len(rec.events)
		rec.command(cmd, at)
		waitUntil(t, tt, a, updates, func() bool { return len(rec.events) > before })
	}
	press(nowplaying.Pause, time.Second)
	if a.player.play.Playing() || playerVideoConfig(tt).Paused != true {
		t.Fatal("the pause key did not pause the film")
	}
	press(nowplaying.Pause, time.Second)
	if e := rec.last(t); a.player.play.Playing() || e.kind != "playback" || e.state != nowplaying.Paused {
		t.Fatalf("a second pause told the system %+v", e)
	}
	press(nowplaying.Play, time.Second)
	if e := rec.last(t); !a.player.play.Playing() || e.state != nowplaying.Playing {
		t.Fatalf("playing told the system %+v", e)
	}
	sets := rec.count("set")
	press(nowplaying.Seek, 3*time.Second)
	if e := rec.last(t); e.kind != "playback" || e.state != nowplaying.Playing || e.at != 3*time.Second || !slices.Contains(playerSeeks(tt), 3*time.Second) {
		t.Fatalf("the scrubber told the system %+v, seeks %v", e, playerSeeks(tt))
	}
	if rec.count("clear") != 0 || rec.count("set") != sets {
		t.Fatalf("the seek took the film off or showed it again: %+v", rec.events)
	}
	tt.Key("escape")
	if a.player.play != nil || rec.last(t).kind != "clear" {
		t.Fatalf("closing the film left the system %+v", rec.last(t))
	}
}

// TestNowPlayingShowsThePosterAndTheEnd checks that a film shows with its poster once read, that its end shows paused at its length so the play key starts it over, that a media key while the film opens decides whether it opens playing, and that another video opening takes the film off.
func TestNowPlayingShowsThePosterAndTheEnd(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 10, nil)
	a.lib.posterDir = t.TempDir()
	poster := []byte("poster of the clip")
	if err := os.WriteFile(posterDest(a.lib.posterDir, item.VideoPath), poster, 0o644); err != nil {
		t.Fatal(err)
	}
	rec := &nowPlayingRecorder{}
	a.playerNowPlayingConnect(rec)
	playerOpenFilm(t, a, tt, updates, item)
	rec.command(nowplaying.Play, 0)
	waitUntil(t, tt, a, updates, func() bool { return a.player.play.played })
	// The system is shown the film only once the system's player has loaded it (playerNowPlayingShow), so a key before that may tell it nothing yet; whatever it was told must not say the film holds
	if len(rec.events) > 0 {
		if e := rec.last(t); e.kind != "clear" && (e.kind != "playback" || e.state != nowplaying.Playing) {
			t.Fatalf("play while the film opens told the system %+v", e)
		}
	}
	playerLoadFilm(tt, a, 10*time.Second)
	if !a.player.play.Playing() || playerVideoConfig(tt).Paused {
		t.Fatal("play while the film opened did not have it open playing")
	}
	waitUntil(t, tt, a, updates, func() bool { return rec.count("set") >= 2 })
	if e := rec.last(t); string(e.info.Artwork) != string(poster) || e.info.Title != "talk" {
		t.Fatalf("the poster showed as %+v", e)
	}
	tt.Progress("id:film-video", 10*time.Second, 10*time.Second)
	refresh(tt, a)
	if e := rec.last(t); e.kind != "playback" || e.state != nowplaying.Paused || e.at != 10*time.Second {
		t.Fatalf("the end told the system %+v", e)
	}
	apply(tt, a, func() { a.playerMediaCommand(nowplaying.Play, 10*time.Second) })
	if p := a.player.play; p.finished || !p.Playing() {
		t.Fatal("the play key did not start the film over")
	}
	if e := rec.last(t); e.kind != "playback" || e.state != nowplaying.Playing || e.at != 0 {
		t.Fatalf("starting over told the system %+v", e)
	}
	apply(tt, a, func() { a.startPlayback(item.SubtitlePath) })
	if e := rec.last(t); e.kind != "clear" {
		t.Fatalf("another video opening left the system %+v", e)
	}
	apply(tt, a, a.stopPlayback)
	before := len(rec.events)
	a.playerMediaCommand(nowplaying.Pause, 0)
	a.playerMediaCommand(nowplaying.Seek, time.Second)
	if len(rec.events) != before || a.player.play != nil {
		t.Fatalf("a media key with no film open did %+v", rec.events[before:])
	}
}

// TestMediaKeysWhileTheFilmOpens presses the media keys while the system's player opens the film: pause has a film opened playing at a point open held there, and play has it play once open. While the system shows the film, as when the player opens a copy of it after failing on it, the system learns each key at once, at where the film is.
func TestMediaKeysWhileTheFilmOpens(t *testing.T) {
	bin := t.TempDir()
	// The stand-in writes its output file, the last argument, as ffmpeg does; asked for a poster on its standard output (pipe:1) it has none, rather than writing a file of that name where the test runs.
	ffmpeg := writeFakeScript(t, bin, "ffmpeg", "for last; do :; done\n[ \"$last\" = pipe:1 ] && exit 1\nprintf copy > \"$last\"\n")
	a, tt, updates, item := playerFilm(t, 12, media.Tools{FFmpeg: ffmpeg})
	rec := &nowPlayingRecorder{}
	a.playerNowPlayingConnect(rec)
	apply(tt, a, func() { a.OpenAt(item, 4*time.Second) })
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	apply(tt, a, func() { a.playerMediaCommand(nowplaying.Pause, 0) })
	playerLoadFilm(tt, a, 12*time.Second)
	p := a.player.play
	if !playerVideoConfig(tt).Paused || p.Playing() || !slices.Equal(playerSeeks(tt), []time.Duration{4 * time.Second}) {
		t.Fatalf("pause while the film opened: paused %v playing %v seeks %v", playerVideoConfig(tt).Paused, p.Playing(), playerSeeks(tt))
	}
	if e := rec.last(t); e.kind != "set" || e.info.State != nowplaying.Paused || e.info.Position != 4*time.Second {
		t.Fatalf("the film held as it opened showed as %+v", e)
	}
	apply(tt, a, func() { a.playerMediaCommand(nowplaying.Play, 0) })
	playerPlayTo(tt, a, 4500*time.Millisecond)
	if !p.Playing() || playerVideoConfig(tt).Paused || a.PlayHead() != 4500*time.Millisecond {
		t.Fatalf("play: playing %v head %v", p.Playing(), a.PlayHead())
	}
	// The player fails on the film it showed, and opens a copy of it.
	apply(tt, a, func() { a.VideoFailed(errors.New("cannot decode")) })
	if p.loaded || p.Playing() {
		t.Fatal("the film plays while its copy is made")
	}
	apply(tt, a, func() { a.playerMediaCommand(nowplaying.Pause, 0) })
	if e := rec.last(t); e.kind != "playback" || e.state != nowplaying.Paused || e.at != 4500*time.Millisecond {
		t.Fatalf("pause while the copy opens told the system %+v", e)
	}
	waitUntil(t, tt, a, updates, func() bool { return p.source != "" })
	playerLoadFilm(tt, a, 12*time.Second)
	if !playerVideoConfig(tt).Paused || p.Playing() || !slices.Equal(playerSeeks(tt), []time.Duration{4500 * time.Millisecond}) {
		t.Fatalf("the copy opened: paused %v playing %v seeks %v", playerVideoConfig(tt).Paused, p.Playing(), playerSeeks(tt))
	}
	apply(tt, a, func() { a.playerMediaCommand(nowplaying.Play, 0) })
	if e := rec.last(t); !p.Playing() || playerVideoConfig(tt).Paused || e.kind != "playback" || e.state != nowplaying.Playing {
		t.Fatalf("play once the copy opened: playing %v, told the system %+v", p.Playing(), e)
	}
}

// TestSpeedOnLinuxStaysAtOne opens a film on Linux with 2× kept in the settings, where the system's player plays every film at the speed it was recorded at: the transport has no speed button, ⇧. and ⇧, change nothing and say nothing on the picture, and the settings keep 2×; the player is asked for 1×, and the playhead, which the subtitles and the loop follow, moves at it.
func TestSpeedOnLinuxStaysAtOne(t *testing.T) {
	usePlatform(t, ns.OSLinux)
	a, tt, _ := playerOpen(t)
	apply(tt, a, func() { a.player.rate = 2; a.playerApplyRate() })
	tt.Key("shift+,")
	tt.Key("shift+.")
	m := testlocale.Messages()
	if a.player.rate != 2 || tt.Exists("id:film-speed") || hasText(tt, m.PlayerOsdSpeed(film.RateTextIn(m, 1.5))) || hasText(tt, film.RateTextIn(m, 2)) || playerVideoConfig(tt).Rate != 1 {
		t.Fatalf("rate %v, the player asked for %v; texts %q", a.player.rate, playerVideoConfig(tt).Rate, tt.Texts())
	}
	tt.Key("space")
	playerPlayTo(tt, a, 2*time.Second)
	tt.Advance(time.Second)
	if head := a.PlayHead(); head != 3*time.Second {
		t.Fatalf("a second from 2s is at %v, want 3s", head)
	}
	if playerListsSpeedKeys(a) {
		t.Fatal("the cheat sheet lists the speed keys on Linux")
	}
}

// TestSpeedKeysAreListedWhereTheyWork checks the cheat sheet lists ⇧, and ⇧. on a Mac, where the speed keys change the film's speed.
func TestSpeedKeysAreListedWhereTheyWork(t *testing.T) {
	a, _, _ := playerOpen(t)
	if !playerListsSpeedKeys(a) {
		t.Fatal("the cheat sheet does not list the speed keys")
	}
}

// playerListsSpeedKeys reports whether the cheat sheet lists ⇧, or ⇧..
func playerListsSpeedKeys(a *app) bool {
	for _, g := range a.cheatsheetGroups(a.cx) {
		for _, k := range g.Keys {
			if slices.Contains(k.Combos, "shift+,") || slices.Contains(k.Combos, "shift+.") {
				return true
			}
		}
	}
	return false
}

// TestSpeedKeysStepThroughTheRates presses ⇧. and ⇧, on an open film: each steps to the next speed of film.Rates, stays at the fastest and the slowest, and says so on the picture and on the transport's speed button at once; the system's player plays at it at once, and once the keys rest the settings keep it.
func TestSpeedKeysStepThroughTheRates(t *testing.T) {
	a, tt, updates := playerOpen(t)
	m := testlocale.Messages()
	key := func(k string, rate float64, osd string) {
		t.Helper()
		tt.Key(k)
		if a.player.rate != rate || !hasText(tt, osd) || !hasText(tt, film.RateTextIn(m, rate)) || playerVideoConfig(tt).Rate != float32(rate) {
			t.Fatalf("after %s: rate %v, want %v; player %+v; texts %q", k, a.player.rate, rate, playerVideoConfig(tt), tt.Texts())
		}
	}
	key("shift+.", 1.1, "速度 1.1×")
	key("shift+.", 1.25, "速度 1.25×")
	key("shift+.", 1.5, "速度 1.5×")
	key("shift+.", 2, "速度 2×")
	waitUntil(t, tt, a, updates, func() bool { return !a.player.rateSettling })
	if saved := loadSavedSettings(a.settingsPath); saved.Rate == nil || *saved.Rate != 2 {
		t.Fatalf("saved rate %v, want 2", saved.Rate)
	}
	key("shift+.", 2, "速度 2×")
	key("shift+,", 1.5, "速度 1.5×")
	for range 4 {
		tt.Key("shift+,")
	}
	key("shift+,", 0.8, "速度 0.8×")
	key("shift+,", 0.75, "速度 0.75×")
	key("shift+,", 0.5, "速度 0.5×")
	// The playhead moves on at the film's speed.
	tt.Key("space")
	playerPlayTo(tt, a, 2*time.Second)
	tt.Advance(time.Second)
	if head := a.PlayHead(); head != 2500*time.Millisecond {
		t.Fatalf("a second at 0.5× from 2s is at %v, want 2.5s", head)
	}
}

// TestSpeedSettingsRoundTrip checks that the speed is kept in the settings, that a saved speed that is not one of film.Rates comes back as the nearest of them, and that one that is not a speed, 0 or below, comes back as 1.
func TestSpeedSettingsRoundTrip(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.SetRate(1.5)
	waitUntil(t, tt, a, updates, func() bool { return !a.player.rateSettling })
	b, _, _ := newTestApp(t, &fakePipeline{})
	b.restorePlayer(loadSavedSettings(a.settingsPath))
	if b.player.rate != 1.5 {
		t.Fatalf("restored rate %v, want 1.5", b.player.rate)
	}
	for _, tc := range []struct {
		saved string
		want  float64
	}{{"3", 2}, {"0.1", 0.5}, {"0.6", 0.5}, {"0.93", 0.9}, {"0.97", 1}, {"1.3", 1.25}, {"0", 1}, {"-1", 1}} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(`{"rate": `+tc.saved+`}`), 0o600); err != nil {
			t.Fatal(err)
		}
		saved := loadSavedSettings(path)
		if saved.Rate == nil || *saved.Rate != tc.want {
			t.Fatalf("saved rate %s read as %v, want %v", tc.saved, saved.Rate, tc.want)
		}
		c, _, _ := newTestApp(t, &fakePipeline{})
		c.restorePlayer(saved)
		if c.player.rate != tc.want {
			t.Fatalf("saved rate %s restored as %v, want %v", tc.saved, c.player.rate, tc.want)
		}
	}
	d, _, _ := newTestApp(t, &fakePipeline{})
	d.restorePlayer(savedSettings{})
	if d.player.rate != 1 {
		t.Fatalf("default rate %v", d.player.rate)
	}
}

// TestCueNearFindsTheLinesAround checks the lines before, at and after a moment: on a line, at a line's very start, between lines, before the first and after the last, which has no end and so lasts to the video's.
func TestCueNearFindsTheLinesAround(t *testing.T) {
	for _, tc := range []struct {
		at              time.Duration
		prev, cur, next int
	}{
		{2 * time.Second, -1, 0, 1},
		{3 * time.Second, 0, 1, 2},
		{6 * time.Second, 1, -1, 2},
		{0, -1, -1, 0},
		{20 * time.Second, 2, 3, -1},
	} {
		if prev, cur, next := film.CueNear(lineCues, tc.at); prev != tc.prev || cur != tc.cur || next != tc.next {
			t.Errorf("at %v: %d %d %d, want %d %d %d", tc.at, prev, cur, next, tc.prev, tc.cur, tc.next)
		}
	}
}

var lineCues = []core.Segment{
	{Start: time.Second, End: 3 * time.Second, Text: "第一句\nOne."},
	{Start: 3 * time.Second, End: 5 * time.Second, Text: "第二句\nTwo."},
	{Start: 7 * time.Second, End: 9 * time.Second, Text: "第三句\nThree."},
	{Start: 10 * time.Second, Text: "第四句\nFour."},
}

// linesApp is an app with film key open and lineCues as its loaded subtitles, without a picture.
func linesApp(t *testing.T, key string) *app {
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.openKey = key
	a.player.subs = subtitleView{key: key, ready: true, cues: lineCues}
	return a
}

// TestLoopLineTakesTheLinePlaying checks which line L loops: the one playing, between lines the one just heard, as R replays it, before the first line the one coming, and the last line, whose loop runs to the video's end; a line without an end loops to the next line's start. It goes off again, and with no lines there is nothing to loop.
func TestLoopLineTakesTheLinePlaying(t *testing.T) {
	a := linesApp(t, "film")
	for _, tc := range []struct {
		at, start, end time.Duration
	}{
		{2 * time.Second, time.Second, 3 * time.Second},
		{6 * time.Second, 3 * time.Second, 5 * time.Second},
		{0, time.Second, 3 * time.Second},
		{20 * time.Second, 10 * time.Second, 0},
	} {
		a.player.opening = tc.at
		a.ToggleLoopLine()
		if loop := a.player.film.loop; !loop.on || loop.start != tc.start || loop.end != tc.end || a.player.film.osd != "循环：开" {
			t.Fatalf("at %v: loop %+v osd %q, want %v to %v", tc.at, loop, a.player.film.osd, tc.start, tc.end)
		}
		a.ToggleLoopLine()
		if a.player.film.loop.on || a.player.film.osd != "循环：关" {
			t.Fatalf("at %v: loop %+v osd %q after L again", tc.at, a.player.film.loop, a.player.film.osd)
		}
	}
	if loop := playerLoopOf("film", []core.Segment{{Start: time.Second}, {Start: 4 * time.Second, End: 5 * time.Second}}, 0); loop.end != 4*time.Second {
		t.Fatalf("a line without an end loops to %v, want the next line's start", loop.end)
	}
	a.player.subs.cues = nil
	a.ToggleLoopLine()
	if a.player.film.loop.on || a.player.film.osd != "还没有字幕" {
		t.Fatalf("loop %+v osd %q without lines", a.player.film.loop, a.player.film.osd)
	}
}

// TestLoopFollowsASeek checks that a seek out of the looped line moves the loop to the line it lands on, or the one coming, and stops it after the last line, while a seek within the line keeps it. The subtitle delay is counted: the picture is the delay later than its lines. Closing the film stops looping.
func TestLoopFollowsASeek(t *testing.T) {
	a := linesApp(t, "film")
	a.player.film.delay = 500 * time.Millisecond
	a.player.film.loop = playerLoopOf("film", lineCues, 0)
	a.Seek(3 * time.Second)
	if loop := a.player.film.loop; !loop.on || loop.start != time.Second {
		t.Fatalf("a seek within the line, its lines at 2.5s, moved the loop to %+v", loop)
	}
	a.Seek(4 * time.Second)
	if loop := a.player.film.loop; !loop.on || loop.start != 3*time.Second || loop.end != 5*time.Second {
		t.Fatalf("a seek to the second line looped %+v", loop)
	}
	a.Seek(6500 * time.Millisecond)
	if loop := a.player.film.loop; !loop.on || loop.start != 7*time.Second {
		t.Fatalf("a seek between lines looped %+v, want the line coming", loop)
	}
	a.player.film.loop = playerLoopOf("film", lineCues, 1)
	a.Seek(30 * time.Second)
	if loop := a.player.film.loop; !loop.on || loop.start != 10*time.Second || loop.end != 0 {
		t.Fatalf("a seek into the last line looped %+v", loop)
	}
	a.player.subs.cues = lineCues[:3]
	a.player.film.loop = playerLoopOf("film", lineCues, 0)
	a.Seek(30 * time.Second)
	if a.player.film.loop.on {
		t.Fatalf("a seek past the last line kept looping %+v", a.player.film.loop)
	}
	a.player.film.loop = playerLoopOf("film", lineCues, 0)
	a.closeFilm()
	if a.player.film.loop.on {
		t.Fatal("closing the film kept looping")
	}
}

// playerOpenWithLines is playerOpen with lineCues as the film's loaded subtitles.
func playerOpenWithLines(t *testing.T) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	a, tt, updates := playerOpen(t)
	apply(tt, a, func() { a.player.subs.cues = lineCues })
	return a, tt, updates
}

// playerOpenWithLinesAndAI is playerOpenWithLines with an AI service set, which explains every line it is asked about; seen is what it was sent.
func playerOpenWithLinesAndAI(t *testing.T) (*app, *nsgotest.App, chan func(), *[]string) {
	t.Helper()
	srv, seen := fakeLLM(t, `{"meaning":"讲解。"}`)
	a, tt, updates, item := playerFilm(t, 12, media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}})
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	apply(tt, a, func() { a.player.subs.cues = lineCues })
	return a, tt, updates, seen
}

// playerWantHeldOn checks that the film, held at the end of line i of lineCues, whose original is original, takes that line and not the one after it, though the playhead has reached it: the picture and the held-line card show it, ⇧⌘C copies it, B saves it (and B again takes it out), I explains it, and R, last, plays it again from its start.
func playerWantHeldOn(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), seen *[]string, i int, original string) {
	t.Helper()
	if a.player.play.Playing() {
		t.Fatalf("the film plays on at %v", a.PlayHead())
	}
	if !onPicture(tt, original) || !tt.Exists("id:film-held-text >> "+whole(original)) {
		t.Fatalf("held at the end of %q at %v, the window shows %q", original, a.PlayHead(), tt.Texts())
	}
	for _, other := range lineCues {
		if _, o := film.SplitCue(other.Text); o != original && onPicture(tt, o) {
			t.Fatalf("held at the end of %q, the picture shows %q", original, o)
		}
	}
	tt.Clipboard = ""
	tt.Key("mod+shift+c")
	if _, got := film.SplitCue(tt.Clipboard); got != original {
		t.Fatalf("⇧⌘C copied %q, want %q", tt.Clipboard, original)
	}
	tt.Key("b")
	if lines := a.savedBook().Lines; len(lines) != 1 || lines[0].Original != original || !a.LineSaved(i) {
		t.Fatalf("B saved %+v, want %q", lines, original)
	}
	tt.Key("b")
	if lines := a.savedBook().Lines; len(lines) != 0 {
		t.Fatalf("B again left %+v", lines)
	}
	*seen = nil
	tt.Key("i")
	waitUntil(t, tt, a, updates, func() bool { return a.explain.line.open && !a.explain.line.busy })
	// The line is sent the first time it is explained; held on it again, I shows what was said of it then (explainUI.cache).
	if sent := strings.Join(*seen, "\n"); a.explain.line.index != i || a.explain.line.line.Original != original || len(*seen) > 0 && !strings.Contains(sent, "要讲解的一句：["+cmdpalette.Clock(lineCues[i].Start)+"] "+original) {
		t.Fatalf("I explained line %d %q, sent %q, want %q", a.explain.line.index, a.explain.line.line.Original, sent, original)
	}
	apply(tt, a, a.CloseExplain)
	before := len(playerSeeks(tt))
	tt.Key("r")
	if got := playerSeeks(tt); len(got) != before+1 || got[len(got)-1] != lineCues[i].Start+a.player.film.delay {
		t.Fatalf("R seeked %v, want to %v", got, lineCues[i].Start)
	}
}

// TestLineKeysMoveBetweenLines checks A, D and R on lineCues, with the subtitles half a second late: D plays from the next line's start, A from the line before, or from the start of the line playing once more than a second of it has played, R from the start of the line playing, or between lines of the line just heard, each saying which line it is. Before the first line and after the last has started, there is nowhere to go, and the picture says so.
func TestLineKeysMoveBetweenLines(t *testing.T) {
	a, tt, _ := playerOpenWithLines(t)
	a.player.film.delay = 500 * time.Millisecond
	key := func(k string, want time.Duration, osd string) {
		t.Helper()
		before := len(playerSeeks(tt))
		tt.Key(k)
		got := playerSeeks(tt)
		if len(got) != before+1 || got[len(got)-1] != want || !hasText(tt, osd) {
			t.Fatalf("after %s: seeks %v, want one to %v; texts %q", k, got, want, tt.Texts())
		}
		// The seek lands and the film is held there.
		playerPlayTo(tt, a, want)
		tt.Key("space")
	}
	seek := func(at time.Duration) {
		t.Helper()
		apply(tt, a, func() { a.seekPlayback(at) })
		playerPlayTo(tt, a, at)
		tt.Key("space")
	}
	stays := func(k, osd string) {
		t.Helper()
		before := len(playerSeeks(tt))
		tt.Key(k)
		if len(playerSeeks(tt)) != before || !hasText(tt, osd) {
			t.Fatalf("after %s: seeks %v; texts %q", k, playerSeeks(tt), tt.Texts())
		}
	}
	key("d", 1500*time.Millisecond, "第 1 / 4 句")
	key("d", 3500*time.Millisecond, "第 2 / 4 句")
	key("a", 1500*time.Millisecond, "第 1 / 4 句")
	key("r", 1500*time.Millisecond, "第 1 / 4 句")
	seek(5 * time.Second)
	key("a", 3500*time.Millisecond, "第 2 / 4 句")
	seek(6500 * time.Millisecond)
	key("r", 3500*time.Millisecond, "第 2 / 4 句")
	seek(6500 * time.Millisecond)
	key("a", 3500*time.Millisecond, "第 2 / 4 句")
	seek(6500 * time.Millisecond)
	key("d", 7500*time.Millisecond, "第 3 / 4 句")
	seek(11 * time.Second)
	stays("d", "后面没有台词了")
	seek(0)
	stays("a", "前面没有台词了")
}

// TestPeekKeyShowsTheLineInBothLanguages listens to the original alone and presses T on a line: the picture shows its translation too, until T again, or a move to another line; between lines it shows nothing.
func TestPeekKeyShowsTheLineInBothLanguages(t *testing.T) {
	a, tt, _ := playerOpenWithLines(t)
	apply(tt, a, func() { a.SetSubMode(film.SubOriginal) })
	playerPlayTo(tt, a, 2*time.Second)
	if !onPicture(tt, "One.") || onPicture(tt, "第一句") {
		t.Fatal("Original Only does not show the original alone")
	}
	tt.Key("t")
	if !onPicture(tt, "One.") || !onPicture(tt, "第一句") {
		t.Fatal("T does not show both lines")
	}
	tt.Key("t")
	if onPicture(tt, "第一句") {
		t.Fatal("T again leaves the translation")
	}
	tt.Key("t")
	tt.Key("d")
	playerPlayTo(tt, a, 3500*time.Millisecond)
	if a.Peek() != -1 || onPicture(tt, "第二句") {
		t.Fatalf("the next line, after T on the one before, peeks at %d", a.Peek())
	}
	playerPlayTo(tt, a, 6*time.Second)
	tt.Key("t")
	if a.Peek() != -1 {
		t.Fatalf("T between lines peeks at line %d", a.Peek())
	}
}

// onPicture reports whether the subtitles on the picture show a line that is s as a whole.
func onPicture(tt *nsgotest.App, s string) bool {
	return tt.Exists("id:film-subtitle-lines >> " + whole(s))
}

// TestExtendLoopTakesInTheNextLine presses ⇧L: without a loop it loops the line playing, as L does, and then each press takes in the line after the loop, up to the last line, whose loop runs to the video's end; past it there is no line to take in. The picture says which lines loop. A move within the span keeps it, and a click on a line with Shift (LoopThrough) loops from the span's start through that line, or from that line through the span's end when it comes before it.
func TestExtendLoopTakesInTheNextLine(t *testing.T) {
	a := linesApp(t, "film")
	a.player.opening = 2 * time.Second
	extend := func(start, end time.Duration, osd string) {
		t.Helper()
		a.ExtendLoop()
		if loop := a.player.film.loop; !loop.on || loop.start != start || loop.end != end || a.player.film.osd != osd {
			t.Fatalf("loop %+v osd %q, want %v to %v and %q", loop, a.player.film.osd, start, end, osd)
		}
	}
	extend(time.Second, 3*time.Second, "循环：开")
	extend(time.Second, 5*time.Second, "循环第 1–2 句")
	extend(time.Second, 9*time.Second, "循环第 1–3 句")
	extend(time.Second, 0, "循环第 1–4 句")
	a.ExtendLoop()
	if loop := a.player.film.loop; loop.end != 0 || a.player.film.osd != "后面没有台词了" {
		t.Fatalf("past the last line: loop %+v osd %q", loop, a.player.film.osd)
	}
	a.player.film.loop = playerLoop{on: true, key: "film", start: time.Second, end: 5 * time.Second}
	a.Seek(3500 * time.Millisecond)
	if loop := a.player.film.loop; loop.start != time.Second || loop.end != 5*time.Second {
		t.Fatalf("a move within the span left %+v", loop)
	}
	a.LoopThrough(2)
	if loop := a.player.film.loop; loop.start != time.Second || loop.end != 9*time.Second || a.player.film.osd != "循环第 1–3 句" {
		t.Fatalf("Shift and a click on the third line: loop %+v osd %q", loop, a.player.film.osd)
	}
	a.player.film.loop = playerLoop{on: true, key: "film", start: 7 * time.Second, end: 9 * time.Second}
	a.LoopThrough(1)
	if loop := a.player.film.loop; loop.start != 3*time.Second || loop.end != 9*time.Second {
		t.Fatalf("Shift and a click on the line before the loop: %+v", loop)
	}
	a.player.film.loop = playerLoop{}
	a.player.opening = 2 * time.Second
	a.LoopThrough(1)
	if loop := a.player.film.loop; !loop.on || loop.start != time.Second || loop.end != 5*time.Second {
		t.Fatalf("Shift and a click without a loop: %+v, want from the line playing", loop)
	}
}

// TestExtendLoopKey presses ⇧L on an open film: the loop takes in the next line, and the transport's toggle is lit.
func TestExtendLoopKey(t *testing.T) {
	a, tt, _ := playerOpenWithLines(t)
	tt.Key("l")
	tt.Key("shift+l")
	if loop := a.player.film.loop; !loop.on || loop.start != time.Second || loop.end != 5*time.Second || !tt.Find("id:film-loop").Props.Selected {
		t.Fatalf("⇧L looped %+v", loop)
	}
}

// TestLoopLineSeeksBackAtItsEnd plays a looped line and checks that the system's player is taken back to the line's start once it reports the line's end, without writing where the film continues from, and plays on past it once L turns the loop off. The transport's toggle is lit while the line loops.
func TestLoopLineSeeksBackAtItsEnd(t *testing.T) {
	a, tt, _ := playerOpenWithLines(t)
	tt.Key("l")
	if loop := a.player.film.loop; !loop.on || loop.start != time.Second || loop.end != 3*time.Second || !hasText(tt, "循环：开") {
		t.Fatalf("L before the first line looped %+v; texts %q", loop, tt.Texts())
	}
	if !tt.Find("id:film-loop").Props.Selected {
		t.Fatal("the transport's loop toggle is not lit")
	}
	tt.Key("space")
	positions := a.player.positions[a.player.play.path]
	playerPlayTo(tt, a, 2*time.Second)
	if len(playerSeeks(tt)) != 0 {
		t.Fatalf("the loop went back inside the line: %v", playerSeeks(tt))
	}
	playerPlayTo(tt, a, 3100*time.Millisecond)
	if got := playerSeeks(tt); len(got) != 1 || got[0] != time.Second || !a.player.play.Playing() {
		t.Fatalf("at the line's end the loop seeked %v, playing %v", got, a.player.play.Playing())
	}
	if a.player.positions[a.player.play.path] != positions {
		t.Fatal("the loop's return wrote where the film continues from")
	}
	tt.Key("l")
	if a.player.film.loop.on || !hasText(tt, "循环：关") {
		t.Fatalf("L kept looping %+v; texts %q", a.player.film.loop, tt.Texts())
	}
	playerPlayTo(tt, a, 1100*time.Millisecond)
	playerPlayTo(tt, a, 3100*time.Millisecond)
	if len(playerSeeks(tt)) != 1 {
		t.Fatalf("the film went back with the loop off: %v", playerSeeks(tt))
	}
}

// TestPauseAfterEachLine turns on Pause After Each Line with P and plays: the film pauses as each line ends, on the app's clock, so the learner can repeat it or write it down, and Space plays on to the next line's end, where it pauses again; R plays the same line again and pauses at its end. The transport's toggle is lit while it is on, and the setting is kept.
func TestPauseAfterEachLine(t *testing.T) {
	a, tt, updates, seen := playerOpenWithLinesAndAI(t)
	tt.Key("p")
	if !a.player.pauseLines || !hasText(tt, "逐句暂停：开") || !tt.Find("id:film-pause-lines").Props.Selected {
		t.Fatalf("P: on %v, texts %q", a.player.pauseLines, tt.Texts())
	}
	tt.Key("space")
	playerPlayTo(tt, a, 2*time.Second)
	tt.Advance(900 * time.Millisecond)
	if !a.player.play.Playing() {
		t.Fatal("the film paused inside the first line")
	}
	tt.Advance(200 * time.Millisecond)
	if a.player.play.Playing() || a.PlayHead() < 3*time.Second || len(playerSeeks(tt)) != 0 {
		t.Fatalf("at the first line's end: playing %v, at %v, seeks %v", a.player.play.Playing(), a.PlayHead(), playerSeeks(tt))
	}
	// The second line starts as the first ends, so the playhead held just past the end is in it; the film still takes the line just heard, each time it holds there.
	for range 3 {
		refresh(tt, a)
		playerWantHeldOn(t, a, tt, updates, seen, 0, "One.")
		playerPlayTo(tt, a, time.Second)
		tt.Advance(2100 * time.Millisecond)
	}
	// D goes on to the line after the one held on, not past it.
	tt.Key("d")
	if got := playerSeeks(tt); got[len(got)-1] != 3*time.Second {
		t.Fatalf("D held at the first line's end seeked %v", got)
	}
	playerPlayTo(tt, a, 3*time.Second)
	tt.Advance(2100 * time.Millisecond)
	if a.player.play.Playing() || a.PlayHead() < 5*time.Second {
		t.Fatalf("D, then the second line's end: playing %v, at %v", a.player.play.Playing(), a.PlayHead())
	}
	apply(tt, a, func() { a.playerSeek(time.Second) })
	playerPlayTo(tt, a, time.Second)
	tt.Advance(2100 * time.Millisecond)
	// The system's player reports where it held, a little before the line's end.
	playerPlayTo(tt, a, 2950*time.Millisecond)
	if !onPicture(tt, "One.") {
		t.Fatalf("held a little before the first line's end, the picture shows %q", tt.Texts())
	}
	tt.Key("space")
	tt.Advance(time.Second)
	if !a.player.play.Playing() {
		t.Fatal("Space did not play on into the second line")
	}
	tt.Advance(1100 * time.Millisecond)
	if a.player.play.Playing() || a.PlayHead() < 5*time.Second {
		t.Fatalf("at the second line's end: playing %v, at %v", a.player.play.Playing(), a.PlayHead())
	}
	tt.Key("r")
	playerPlayTo(tt, a, 3*time.Second)
	tt.Advance(2100 * time.Millisecond)
	if a.player.play.Playing() || a.PlayHead() < 5*time.Second {
		t.Fatalf("R, then the line's end: playing %v, at %v", a.player.play.Playing(), a.PlayHead())
	}
	if saved := loadSavedSettings(a.settingsPath); !saved.PauseLines {
		t.Fatal("Pause After Each Line was not kept")
	}
	tt.Key("p")
	if a.player.pauseLines || !hasText(tt, "逐句暂停：关") {
		t.Fatalf("P again: on %v, texts %q", a.player.pauseLines, tt.Texts())
	}
	tt.Key("space")
	tt.Advance(3 * time.Second)
	if !a.player.play.Playing() {
		t.Fatal("with P off again the film paused")
	}
}

// TestPauseAfterEachLineInALoop loops a line with Pause After Each Line on: at the line's end the film pauses rather than go back, and Space plays the line again from its start.
func TestPauseAfterEachLineInALoop(t *testing.T) {
	a, tt, updates, seen := playerOpenWithLinesAndAI(t)
	tt.Key("p")
	tt.Key("l")
	tt.Key("space")
	playerPlayTo(tt, a, 2*time.Second)
	tt.Advance(1100 * time.Millisecond)
	if a.player.play.Playing() || len(playerSeeks(tt)) != 0 {
		t.Fatalf("at the looped line's end: playing %v, seeks %v", a.player.play.Playing(), playerSeeks(tt))
	}
	tt.Key("space")
	tt.Advance(10 * time.Millisecond)
	if got := playerSeeks(tt); len(got) != 1 || got[0] != time.Second || !a.player.play.Playing() {
		t.Fatalf("Space after the pause: seeks %v, playing %v", got, a.player.play.Playing())
	}
	// Held at the looped line's end, the film takes the looped line, though the next starts there, on every pass.
	for range 3 {
		playerPlayTo(tt, a, 2*time.Second)
		tt.Advance(1100 * time.Millisecond)
		refresh(tt, a)
		playerWantHeldOn(t, a, tt, updates, seen, 0, "One.")
		if !a.LoopLine() {
			t.Fatal("the keys on the held line stopped the loop")
		}
	}
}

// TestLoopGapWaitsTheLinesLength loops a two-second line with Pause Between Repeats on: at its end the film holds for as long as the line took, for the learner to say it after it, and then plays it again from its start by itself, without keeping that hold as where the film continues from.
func TestLoopGapWaitsTheLinesLength(t *testing.T) {
	a, tt, _ := playerOpenWithLines(t)
	useMenuBar(t, a, tt)
	tt.ChooseMenu("播放", "循环间停顿")
	if a.player.loopGap != playerLoopGapLine || !hasText(tt, "循环间停顿：开") {
		t.Fatalf("the menu row left the gap %q; texts %q", a.player.loopGap, tt.Texts())
	}
	tt.Key("l")
	tt.Key("space")
	playerPlayTo(tt, a, 2*time.Second)
	positions := a.player.positions[a.player.play.path]
	tt.Advance(1100 * time.Millisecond)
	if a.player.play.Playing() || len(playerSeeks(tt)) != 0 {
		t.Fatalf("at the line's end: playing %v, seeks %v", a.player.play.Playing(), playerSeeks(tt))
	}
	tt.Advance(1800 * time.Millisecond)
	if a.player.play.Playing() || len(playerSeeks(tt)) != 0 {
		t.Fatal("the gap ended before the line's length")
	}
	if !onPicture(tt, "One.") || onPicture(tt, "Two.") {
		t.Fatalf("in the gap the picture shows %q, want the looped line", tt.Texts())
	}
	tt.Advance(300 * time.Millisecond)
	if got := playerSeeks(tt); len(got) != 1 || got[0] != time.Second || !a.player.play.Playing() {
		t.Fatalf("after the gap: seeks %v, playing %v", got, a.player.play.Playing())
	}
	if a.player.positions[a.player.play.path] != positions {
		t.Fatal("the gap's hold wrote where the film continues from")
	}
	if saved := loadSavedSettings(a.settingsPath); saved.LoopGap != playerLoopGapLine {
		t.Fatalf("the gap was kept as %q", saved.LoopGap)
	}
}

// TestStudySettingsRoundTrip reads Pause After Each Line and Pause Between Repeats back from settings.json; a gap this version does not know is none.
func TestStudySettingsRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		saved string
		pause bool
		gap   string
	}{{`{"pauseAfterLine": true, "loopGap": "line"}`, true, playerLoopGapLine}, {`{"loopGap": "forever"}`, false, ""}, {`{}`, false, ""}} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(tc.saved), 0o600); err != nil {
			t.Fatal(err)
		}
		a, _, _ := newTestApp(t, &fakePipeline{})
		a.restorePlayer(loadSavedSettings(path))
		if a.player.pauseLines != tc.pause || a.player.loopGap != tc.gap {
			t.Fatalf("%s restored as pause %v gap %q", tc.saved, a.player.pauseLines, a.player.loopGap)
		}
	}
}

// TestLoopGoesBackAtTheLinesEndBetweenReports plays a looped line on the app's clock alone: the film goes back to the line's start as the clock reaches its end, without waiting for the system's player's next report.
func TestLoopGoesBackAtTheLinesEndBetweenReports(t *testing.T) {
	a, tt, _ := playerOpenWithLines(t)
	tt.Key("l")
	tt.Key("space")
	playerPlayTo(tt, a, 2*time.Second)
	tt.Advance(900 * time.Millisecond)
	if len(playerSeeks(tt)) != 0 {
		t.Fatalf("the loop went back inside the line: %v", playerSeeks(tt))
	}
	tt.Advance(200 * time.Millisecond)
	if got := playerSeeks(tt); len(got) != 1 || got[0] != time.Second || !a.player.play.Playing() {
		t.Fatalf("at the line's end the loop seeked %v, playing %v", got, a.player.play.Playing())
	}
}

// TestShortLoopIgnoresAReportFromItsEnd loops a line shorter than a second: once the loop has taken the film back to the line's start, a report the system's player sent from the line's end before the seek landed does not take the picture back there, nor have the loop seek again, while the report of the seek landing does move it.
func TestShortLoopIgnoresAReportFromItsEnd(t *testing.T) {
	a, tt, _ := playerOpen(t)
	apply(tt, a, func() {
		a.player.subs.cues = []core.Segment{{Start: time.Second, End: 1600 * time.Millisecond, Text: "短句\nShort."}}
	})
	tt.Key("l")
	tt.Key("space")
	playerPlayTo(tt, a, 1200*time.Millisecond)
	playerPlayTo(tt, a, 1650*time.Millisecond)
	if got := playerSeeks(tt); len(got) != 1 || got[0] != time.Second {
		t.Fatalf("at the line's end the loop seeked %v", got)
	}
	playerPlayTo(tt, a, 1700*time.Millisecond)
	if got := playerSeeks(tt); len(got) != 1 || a.PlayHead() != time.Second {
		t.Fatalf("a report from the line's end: seeks %v, head %v", got, a.PlayHead())
	}
	playerPlayTo(tt, a, 1050*time.Millisecond)
	if got := playerSeeks(tt); len(got) != 1 || a.PlayHead() != 1050*time.Millisecond {
		t.Fatalf("the seek landed: seeks %v, head %v", got, a.PlayHead())
	}
}

// TestLoopToggleLoopsTheLine clicks the transport's 循环 (Loop): it waits for the film's lines, then loops the line and is lit, and a second click stops it.
func TestLoopToggleLoopsTheLine(t *testing.T) {
	a, tt, _ := playerOpen(t)
	apply(tt, a, func() { a.player.subs.cues = nil })
	if tt.Enabled("id:film-loop") {
		t.Fatal("the toggle takes a press on a film without lines")
	}
	apply(tt, a, func() { a.player.subs.cues = lineCues })
	tt.Press("id:film-loop")
	if !a.player.film.loop.on || a.player.film.loop.start != time.Second {
		t.Fatalf("the toggle looped %+v", a.player.film.loop)
	}
	tt.Press("id:film-loop")
	if a.player.film.loop.on {
		t.Fatal("a second click kept looping")
	}
}

// TestLoopStopsForALineAtTheVideosEnd checks that a looped line the delay has pushed to the video's end stops looping rather than seek to its end again and again, while one inside the video keeps looping.
func TestLoopStopsForALineAtTheVideosEnd(t *testing.T) {
	a := linesApp(t, "film")
	a.player.play = &filmPlay{loaded: true, duration: 10 * time.Second, paused: true}
	a.player.film.loop = playerLoopOf("film", lineCues, 2)
	a.playerKeepLoop()
	if !a.player.film.loop.on {
		t.Fatal("a line inside the video stopped looping")
	}
	a.player.film.delay = 2900 * time.Millisecond
	a.playerKeepLoop()
	if a.player.film.loop.on {
		t.Fatalf("a line starting at %v of a 10s video kept looping", lineCues[2].Start+a.player.film.delay)
	}
	// The first line, at 1s to 3s, with the subtitles 2.5s early still has half a second in the video; 10s early it ends before the video starts.
	a.player.film.loop = playerLoopOf("film", lineCues, 0)
	a.player.film.delay = -2500 * time.Millisecond
	a.playerKeepLoop()
	if !a.player.film.loop.on {
		t.Fatal("a line the delay left half a second of in the video stopped looping")
	}
	a.player.film.delay = -10 * time.Second
	a.playerKeepLoop()
	if a.player.film.loop.on {
		t.Fatalf("a line ending at %v before the video kept looping", lineCues[0].End+a.player.film.delay)
	}
}

// TestLoopFollowsOpenAt checks the loop when a film is opened playing (OpenAt): a line of the open film picked in the command palette moves the loop to it, while another film, or the same one opened again after leaving it, starts without a loop. A loop left from another film counts as none.
func TestLoopFollowsOpenAt(t *testing.T) {
	a := linesApp(t, "film")
	a.player.film.loop = playerLoopOf("film", lineCues, 0)
	a.OpenAt(films.Film{Key: "film"}, 7*time.Second)
	if loop := a.player.film.loop; !loop.on || loop.start != 7*time.Second || loop.end != 9*time.Second || a.player.opening != 7*time.Second {
		t.Fatalf("a jump to the third line looped %+v, opening at %v", loop, a.player.opening)
	}
	a.OpenAt(films.Film{Key: "other"}, 7*time.Second)
	if a.player.film.loop.on || a.LoopLine() {
		t.Fatalf("another film opened looping %+v", a.player.film.loop)
	}

	a = linesApp(t, "film")
	a.player.film.loop = playerLoopOf("film", lineCues, 0)
	a.Select(pageFilms)
	a.OpenAt(films.Film{Key: "film"}, 0)
	if a.player.film.loop.on {
		t.Fatalf("the film opened again from the hero kept looping %+v", a.player.film.loop)
	}

	a = linesApp(t, "job")
	a.player.film.loop = playerLoopOf("film", lineCues, 0)
	a.player.play = &filmPlay{loaded: true, duration: 10 * time.Second, head: 4 * time.Second}
	if a.LoopLine() {
		t.Fatal("a loop of another film shows as on")
	}
	a.playerKeepLoop()
	if a.player.film.loop.on || a.player.play.seeks != 0 {
		t.Fatalf("a loop of another film was kept %+v, seeks %d", a.player.film.loop, a.player.play.seeks)
	}
}

// TestCopyCurrentLineKey checks ⇧⌘C on the open film: it copies the line playing, following the subtitle delay, and says so on the picture; between two lines it leaves the clipboard and the picture alone; with the subtitles off it still copies; and the 字幕 menu's item copies the same way.
func TestCopyCurrentLineKey(t *testing.T) {
	a, tt, _ := playerOpen(t)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Clipboard = "before"
	tt.Key("mod+shift+c")
	if tt.Clipboard != "你好。\nHello." || !hasText(tt, "已拷贝") {
		t.Fatalf("⇧⌘C on the first line copied %q; texts %q", tt.Clipboard, tt.Texts())
	}
	tt.Advance(1500 * time.Millisecond)
	// 3 seconds in is after the first line ends (2.5s) and before the second starts (4s).
	playerPlayTo(tt, a, 3*time.Second)
	tt.Clipboard = "before"
	tt.Key("mod+shift+c")
	if tt.Clipboard != "before" || hasText(tt, "已拷贝") {
		t.Fatalf("⇧⌘C between two lines copied %q", tt.Clipboard)
	}
	// With the subtitles 2 seconds late, 3 seconds in shows the line that starts at 1 second.
	apply(tt, a, func() { a.player.film.delay = 2 * time.Second })
	tt.Key("mod+shift+c")
	if tt.Clipboard != "你好。\nHello." {
		t.Fatalf("⇧⌘C with the delay copied %q", tt.Clipboard)
	}
	apply(tt, a, func() {
		a.player.film.delay = 0
		a.player.subMode = film.SubOff
	})
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Clipboard = "before"
	tt.Key("mod+shift+c")
	if tt.Clipboard != "你好。\nHello." {
		t.Fatalf("⇧⌘C with the subtitles off copied %q", tt.Clipboard)
	}
	playerPlayTo(tt, a, 4500*time.Millisecond)
	useMenuBar(t, a, tt)
	tt.ChooseMenu("字幕", "拷贝当前台词")
	if tt.Clipboard != "Bye." {
		t.Fatalf("the menu's item copied %q", tt.Clipboard)
	}
}

// TestCopyCurrentLineWaitsForTheSubtitles: before the film's subtitles are read, ⇧⌘C has no line to copy, nor from subtitles read for another film.
func TestCopyCurrentLineWaitsForTheSubtitles(t *testing.T) {
	a, tt, _ := playerOpen(t)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	tt.Clipboard = "before"
	apply(tt, a, func() {
		a.player.subs.ready = false
		a.copyCurrentLine()
	})
	if tt.Clipboard != "before" || hasText(tt, "已拷贝") {
		t.Fatalf("copied %q before the subtitles were read", tt.Clipboard)
	}
	apply(tt, a, func() {
		a.player.subs.ready = true
		a.player.subs.key = "another film"
		a.copyCurrentLine()
	})
	if tt.Clipboard != "before" || hasText(tt, "已拷贝") {
		t.Fatalf("copied %q from another film's subtitles", tt.Clipboard)
	}
}

// TestCueMenuCopiesTheLine right-clicks lines of the side panel's subtitle list: a bilingual line offers to explain it and to save it first, then its two lines together, its original and its translation, each put on the clipboard as chosen, and a line without a translation offers only itself; copying is no seek.
func TestCueMenuCopiesTheLine(t *testing.T) {
	_, tt, _ := playerOpen(t)
	copyFrom := func(line, item, want string) {
		t.Helper()
		tt.Clipboard = "before"
		tt.RightClick(whole(line))
		tt.ChoosePopup(item)
		if tt.Clipboard != want {
			t.Fatalf("%s › %s copied %q, want %q", line, item, tt.Clipboard, want)
		}
	}
	tt.RightClick(whole("你好。"))
	var labels []string
	for _, it := range tt.PopupMenu {
		labels = append(labels, it.Label)
	}
	if want := []string{"讲解这句", "收藏这句", "", "拷贝台词", "拷贝原文", "拷贝译文"}; !slices.Equal(labels, want) {
		t.Fatalf("the bilingual line's menu is %q, want %q", labels, want)
	}
	tt.ClosePopup()
	copyFrom("你好。", "拷贝台词", "你好。\nHello.")
	copyFrom("你好。", "拷贝原文", "Hello.")
	copyFrom("你好。", "拷贝译文", "你好。")
	copyFrom("Bye.", "拷贝台词", "Bye.")
	if len(playerSeeks(tt)) != 0 {
		t.Fatalf("copying from the list seeked %v", playerSeeks(tt))
	}
}

// TestSettingsOverTheFilmHoldItsKeys opens the settings over the main window while a film is open, as on Windows and Linux, where they have no window of their own: Space does not play the film behind them and Escape closes the settings, not the film.
func TestSettingsOverTheFilmHoldItsKeys(t *testing.T) {
	usePlatform(t, ns.OSLinux)
	a, tt, _ := playerOpen(t)
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	if !a.prefs.inWindow || !tt.Exists("id:settings-in-window") {
		t.Fatal("the settings did not show over the main window")
	}
	tt.Key("space")
	if a.player.play.Playing() {
		t.Fatal("Space behind the settings played the film")
	}
	tt.Key("escape")
	refresh(tt, a)
	if a.prefs.open {
		t.Fatal("Escape left the settings open")
	}
	if a.openKey == "" || a.player.play == nil {
		t.Fatal("Escape behind the settings closed the film")
	}
}

// TestEscapeEmptiesTheSearchFirst types in the subtitle search before the video has loaded, so the keys are bound again as it loads, and checks that Escape still empties the search and leaves the film open.
func TestEscapeEmptiesTheSearchFirst(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	playerOpenFilm(t, a, tt, updates, item)
	tt.Key("mod+f")
	tt.Settle()
	typeFocused(t, tt, "river")
	playerLoadFilm(tt, a, 12*time.Second)
	if !a.player.film.searchFocused || a.player.film.search != "river" {
		t.Fatalf("search %q focused %v", a.player.film.search, a.player.film.searchFocused)
	}
	tt.Key("escape")
	refresh(tt, a)
	if a.player.film.search != "" || a.openKey == "" {
		t.Fatalf("after Escape the search is %q and the film open %q", a.player.film.search, a.openKey)
	}
}

// TestTransportMenusKeepSpaceAndTheArrows opens the speed menu: while it is open Space and the arrows are left to its rows, so they neither play the film nor seek nor change the volume.
func TestTransportMenusKeepSpaceAndTheArrows(t *testing.T) {
	a, tt, _ := playerOpen(t)
	apply(tt, a, func() { a.player.film.speedMenu = true })
	volume := a.player.volume
	for _, key := range []string{"space", "left", "right", "up", "down"} {
		tt.Key(key)
	}
	if a.player.play.Playing() || len(playerSeeks(tt)) != 0 || a.player.volume != volume {
		t.Fatalf("with the speed menu open: playing %v seeks %v volume %v", a.player.play.Playing(), playerSeeks(tt), a.player.volume)
	}
	apply(tt, a, func() { a.player.film.speedMenu = false })
	tt.Key("space")
	if !a.player.play.Playing() {
		t.Fatal("Space did not play once the menu closed")
	}
}

// TestFilmKeepsTheSpokenLanguageNamed checks that the language the info tab names as the one spoken in a done film is kept with its job in library.json, that a language whisper is not offered for, or one named for a film still being processed, is not, and that processing the film again keeps it, so the job is transcribed in it (media.Run) rather than in the settings' language.
func TestFilmKeepsTheSpokenLanguageNamed(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	path := filepath.Join(t.TempDir(), "library.json")
	a.openLibrary(path)
	a.library.Upsert(core.Job{ID: "talk", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageComplete, Language: "简体中文", SpokenLanguage: "ja",
		VideoPath: testVideos + "/talk.mp4", SubtitlePath: testVideos + "/talk.bilingual.srt", CompletedAt: time.Now()})
	a.openKey = "talk"
	refresh(tt, a)
	a.SetFilmSpeechLanguage("xx")
	if job, _ := a.library.Find("talk"); job.SpeechLanguage != "" {
		t.Fatalf("a language whisper is not offered for was kept: %q", job.SpeechLanguage)
	}
	a.SetFilmSpeechLanguage("en")
	saved, err := shelf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if job, _ := saved.Find("talk"); job.SpeechLanguage != "en" || job.SpokenLanguage != "ja" {
		t.Fatalf("library.json keeps %+v", job)
	}
	a.RetryOpen("talk")
	if job, _ := a.library.Find("talk"); job.Stage == core.StageComplete || job.SpeechLanguage != "en" {
		t.Fatalf("processing the film again queued %+v", job)
	}
	a.SetFilmSpeechLanguage("ko")
	if job, _ := a.library.Find("talk"); job.SpeechLanguage != "en" {
		t.Fatalf("a film being processed took the language %q", job.SpeechLanguage)
	}
	settle(t, a, tt, updates)
	if job, _ := a.library.Find("talk"); job.Stage != core.StageComplete || job.SpeechLanguage != "en" {
		t.Fatalf("processed again %+v", job)
	}
}

// TestOldGlobalSubtitlePlaceIsDropped reads settings.json from a version that kept one subtitle place for every film (the look's Lift): every film's subtitles then start at the look's place, the rest of the look is kept, and the next save writes the look without it.
func TestOldGlobalSubtitlePlaceIsDropped(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	old := `{"look": {"Preset": "box", "Box": true, "High": true, "Lift": 0.2, "Translation": {"Family": "默认", "Size": 30, "Color": "#FFFFFF", "Bold": true}, "Original": {"Family": "默认", "Size": 22, "Color": "#FFD60A"}}}`
	if err := os.WriteFile(a.settingsPath, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	a.restoreSettings(loadSavedSettings(a.settingsPath))
	if l := a.player.look; !l.Box || !l.High || l.Preset != "box" {
		t.Fatalf("the rest of the old look was lost: %+v", l)
	}
	a.Open(films.Film{Key: "a", VideoPath: "/films/a.mp4"})
	if got := a.SubLift(); got != 0 {
		t.Fatalf("a film opens with its subtitles at %v, want the look's place", got)
	}
	a.playerSaveSettings()
	data, err := os.ReadFile(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Lift") || !strings.Contains(string(data), `"High": true`) {
		t.Fatalf("settings.json after a save: %s", data)
	}
}

// TestSubtitlesInTheDefaultFaceKeepTheSystemsFont checks that lines in 默认, the default look's face, are drawn in the system's font on every platform, not in the design's interface font the window sets around them (Cantarell on Linux, Segoe UI Variable on Windows).
func TestSubtitlesInTheDefaultFaceKeepTheSystemsFont(t *testing.T) {
	a, tt, _ := playerOpen(t)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	for _, os := range []ns.OS{ns.OSMacOS, ns.OSWindows, ns.OSLinux} {
		env := testEnvironment()
		env.OS, env.AppleKeyboard = os, os == ns.OSMacOS
		tt.SetEnvironment(env)
		refresh(tt, a)
		var families []string
		var walk func(v *nsgotest.View)
		walk = func(v *nsgotest.View) {
			if v.Text != "" {
				families = append(families, v.TextStyle.Font.Family)
			}
			for _, c := range v.Children {
				walk(c)
			}
		}
		walk(tt.Find("id:film-subtitle-lines"))
		if len(families) == 0 {
			t.Fatalf("%v: no lines shown", os)
		}
		for _, f := range families {
			if f != "system-ui" {
				t.Errorf("%v: a line in %q, want the system's font", os, f)
			}
		}
	}
}

// TestPlayerFollowsTheMainWindowsLifecycle checks that the player hears that the main window went out of sight, and that it shows again. Every window goes out of sight together here, so it would pass as well on the app-wide lifecycle playerAwakeLifecycle read before nsgo#197.
// TODO(nsgo#783): nsgotest.App.SetEnvironment gives every window the same environment (nsgotest/nsgotest.go:909-927) and nothing reports one window's lifecycle alone; once something does, put the main window in the background while the settings window shows, the case the main window's own lifecycle is read for. https://github.com/nsgo-dev/nsgo/issues/783
func TestPlayerFollowsTheMainWindowsLifecycle(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.playerAwakeLifecycle()
	refresh(tt, a)
	if a.player.windowAway {
		t.Fatal("the player thinks the window is away while it shows")
	}
	env := testEnvironment()
	env.Lifecycle = ns.LifecycleBackground
	tt.SetEnvironment(env)
	refresh(tt, a)
	if !a.player.windowAway {
		t.Fatal("the player did not hear that the main window went out of sight")
	}
	env.Lifecycle = ns.LifecycleActive
	tt.SetEnvironment(env)
	refresh(tt, a)
	if a.player.windowAway {
		t.Fatal("the player did not hear that the main window shows again")
	}
}

// TestALineOnScreenIsNotRewrittenWhileItIsRead checks that a line the machine rewrites while it is on screen keeps showing as it was until the playhead leaves it, while a line not on screen takes its new text at once, and so does a line on screen that had no translation and gets one. The machine's lines hold the new text all along, which the export writes.
func TestALineOnScreenIsNotRewrittenWhileItIsRead(t *testing.T) {
	a, tt, updates := playerOpen(t)
	path := a.player.subs.path
	rewrite := func(srt string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(srt), 0o644); err != nil {
			t.Fatal(err)
		}
		later := time.Now().Add(time.Minute)
		if err := os.Chtimes(path, later, later); err != nil {
			t.Fatal(err)
		}
		apply(tt, a, a.reloadOpenSubtitles)
		waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	}
	shown := func(i int) string { return a.player.subs.cues[i].Text }
	playerPlayTo(tt, a, 1500*time.Millisecond)
	rewrite("1\n00:00:01,000 --> 00:00:02,500\n您好。\nHello.\n\n2\n00:00:04,000 --> 00:00:05,000\n再见。\nBye.\n\n")
	if shown(0) != "你好。\nHello." || shown(1) != "再见。\nBye." {
		t.Fatalf("lines %q, %q: want the one on screen as it was and the other rewritten", shown(0), shown(1))
	}
	if got := a.player.subs.machine[0].Text; got != "您好。\nHello." {
		t.Fatalf("the machine's line on screen reads %q, want its new text", got)
	}
	if lines := a.subexportLines(); len(lines) != 2 || lines[0].Translation != "您好。" {
		t.Fatalf("the export of a line held on screen %+v, want its new text", lines)
	}
	playerPlayTo(tt, a, 3*time.Second)
	if shown(0) != "您好。\nHello." {
		t.Fatalf("line left reads %q, want its new text", shown(0))
	}
	// A line on screen without a translation takes the one that comes at once.
	rewrite("1\n00:00:01,000 --> 00:00:02,500\n您好。\nHello.\n\n2\n00:00:04,000 --> 00:00:05,000\nBye.\n\n")
	playerPlayTo(tt, a, 4500*time.Millisecond)
	rewrite("1\n00:00:01,000 --> 00:00:02,500\n您好。\nHello.\n\n2\n00:00:04,000 --> 00:00:05,000\n回头见。\nBye.\n\n")
	if shown(1) != "回头见。\nBye." {
		t.Fatalf("line on screen without a translation reads %q, want the translation at once", shown(1))
	}
}
