package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

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
		ID: "job1", Source: core.FileURL(video), Language: "简体中文", Stage: core.StageComplete,
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
	waitUntil(t, updates, func() bool { return a.player.subs.ready })
	tt.Frame()
	for _, text := range []string{"返回", "字幕 100%", "你好。", "Hello.", "再见。", "0:01", "1:02:03", "全部", locales.FilmShowInFinder(platformName())} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click(locales.FilmShowInFinder(platformName())); err != nil {
		t.Fatal(err)
	}
	if revealed != srt {
		t.Fatalf("revealed %q", revealed)
	}
	if err := tt.Click("返回资料库"); err != nil {
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
		skipWithoutFFmpeg(t, "ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=black:s=160x90:d=1:r=10", "-c:v", "mpeg4", "-pix_fmt", "yuv420p", video).CombinedOutput(); err != nil {
		skipWithoutFFmpeg(t, "ffmpeg could not make a clip: %v %s", err, out)
	}
	srt := filepath.Join(dir, "clip.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,000\n画面上的字幕\nOn the picture.\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(media.Tools{FFmpeg: ffmpeg})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "clip", Source: core.FileURL(video), Language: "简体中文", Stage: core.StageComplete,
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
			t.Fatalf("play button did not show: %q err %q", tt.Texts(), a.player.playErr)
		}
		tt.Frame()
	}
	if a.player.play == nil || a.player.play.player.Playing() {
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
	if a.player.play == nil || !a.player.play.player.Playing() || !tt.HasText("暂停") {
		t.Fatalf("playing %v texts %q", a.player.play != nil && a.player.play.player.Playing(), tt.Texts())
	}
	if err := tt.Click("暂停"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.player.play == nil || a.player.play.player.Playing() || !tt.HasText("播放") {
		t.Fatalf("playing %v texts %q", a.player.play != nil && a.player.play.player.Playing(), tt.Texts())
	}
	if err := tt.Click("返回资料库"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.player.play != nil || a.openKey != "" {
		t.Fatalf("open %q play %v", a.openKey, a.player.play != nil)
	}
}

func TestPlaybackAdvancesThePicture(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=160x90:d=1:r=10",
		"-f", "lavfi", "-i", "color=c=blue:s=160x90:d=1:r=10",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0",
		"-c:v", "mpeg4", "-pix_fmt", "yuv420p", video,
	).CombinedOutput(); err != nil {
		skipWithoutFFmpeg(t, "ffmpeg could not make a clip: %v %s", err, out)
	}
	a, tt, updates := newTestApp(media.Tools{FFmpeg: ffmpeg})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "clip", Source: core.FileURL(video), Stage: core.StageComplete, VideoPath: video,
	}}
	t.Cleanup(a.stopPlayback)
	tt.Frame()
	if err := tt.Click("clip"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(8 * time.Second)
	for a.player.play == nil {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatalf("player did not open: %q", a.player.playErr)
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
		t.Fatalf("picture stayed %v playing %v held %v err %v", before, a.player.play.player.Playing(), a.player.play.paused, a.player.play.player.Err())
	}
}

func TestPlaybackStopsAtTheEnd(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=red:s=160x90:d=1:r=10", "-c:v", "mpeg4", "-pix_fmt", "yuv420p", video).CombinedOutput(); err != nil {
		skipWithoutFFmpeg(t, "ffmpeg could not make a clip: %v %s", err, out)
	}
	a, tt, updates := newTestApp(media.Tools{FFmpeg: ffmpeg})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "clip", Source: core.FileURL(video), Stage: core.StageComplete, VideoPath: video,
	}}
	t.Cleanup(a.stopPlayback)
	tt.Frame()
	if err := tt.Click("clip"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(8 * time.Second)
	for a.player.play == nil {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatalf("player did not open: %q", a.player.playErr)
		}
		tt.Frame()
	}
	if a.player.play.duration <= 0 {
		t.Fatal("clip has no duration")
	}
	if err := tt.Click("播放"); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(4 * time.Second)
	for time.Now().Before(until) && (a.player.play == nil || !a.player.play.finished) {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
		time.Sleep(20 * time.Millisecond)
	}
	if a.player.play == nil || !a.player.play.finished || a.player.play.player.Playing() {
		t.Fatalf("finished %v playing %v", a.player.play != nil && a.player.play.finished, a.player.play != nil && a.player.play.player.Playing())
	}
	head := a.player.play.playhead()
	if head > a.player.play.duration {
		t.Fatalf("playhead %s past %s", head, a.player.play.duration)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if a.player.play.player.Playing() || a.player.play.playhead() != head {
		t.Fatalf("kept playing at %s", a.player.play.playhead())
	}
}

// TestPlaybackResyncsWhenAppReturns checks that coming back to the app opens the picture again only when it stood still behind the window while the sound played on, and never for a film the user paused.
func TestPlaybackResyncsWhenAppReturns(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	p := stillPlay(t, "/v/clip.mp4", 3*time.Second, true)
	a.player.play = p

	a.playerDidResignActive()
	if p.backgroundAt.IsZero() || p.backgroundHead != 3*time.Second || !p.player.Playing() {
		t.Fatalf("inactive playback: at %v head %v playing %v", p.backgroundAt, p.backgroundHead, p.player.Playing())
	}
	// The window stayed in sight: the picture played on with the sound.
	p.backgroundAt = time.Now().Add(-2 * time.Second)
	p.frames.shown = 5 * time.Second
	a.player.opening = -1
	a.playerDidBecomeActive()
	if a.player.opening != -1 || a.player.play != p || !p.backgroundAt.IsZero() {
		t.Fatalf("a picture that kept pace opened again at %v, background at %v", a.player.opening, p.backgroundAt)
	}

	// The window was out of sight: the picture stood at 5s while the sound reached 7s.
	a.playerDidResignActive()
	p.backgroundAt = p.backgroundAt.Add(-2 * time.Second)
	a.playerDidBecomeActive()
	if a.player.opening < 7*time.Second || a.player.play != p || !p.player.Playing() || p.paused {
		t.Fatalf("a picture behind its sound opened again at %v, playing %v paused %v", a.player.opening, p.player.Playing(), p.paused)
	}

	// The sound ended in the background: the picture opens just short of the end.
	p.backgroundAt, p.backgroundHead = time.Now().Add(-time.Hour), time.Minute
	if got := p.backgroundSound()(); got != p.duration-100*time.Millisecond {
		t.Fatalf("sound past the end at %v, want %v", got, p.duration-100*time.Millisecond)
	}

	p.backgroundAt, p.backgroundHead = time.Time{}, 0
	p.player.Pause()
	p.paused = true
	a.playerDidResignActive()
	if !p.backgroundAt.IsZero() {
		t.Fatal("a film paused by the user was marked for background resync")
	}
}

// TestPlaybackFollowsTheSoundWhileTheWindowIsPutAway checks that the main window put away by its close button takes the picture out of sight as the app going inactive does, and that the sound's clock is the one taken when the picture first went out of sight, however the app and the window come and go meanwhile, until both are back.
func TestPlaybackFollowsTheSoundWhileTheWindowIsPutAway(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	p := stillPlay(t, "/v/clip.mp4", 3*time.Second, true)
	a.player.play = p

	a.playerWindowHidden(true)
	if p.backgroundAt.IsZero() || p.backgroundHead != 3*time.Second {
		t.Fatalf("window put away: at %v head %v", p.backgroundAt, p.backgroundHead)
	}
	at := p.backgroundAt.Add(-2 * time.Second)
	p.backgroundAt = at
	// The app goes inactive and comes back while the window stays away: the picture stays out of sight, on the same clock.
	a.playerDidResignActive()
	a.playerDidBecomeActive()
	if p.backgroundAt != at || p.backgroundHead != 3*time.Second {
		t.Fatalf("the clock moved while the window stayed away: at %v head %v", p.backgroundAt, p.backgroundHead)
	}
	a.playerDidResignActive()
	a.playerWindowHidden(false)
	if p.backgroundAt != at {
		t.Fatal("the window came back while the app is inactive and the picture was put back in sight")
	}
	a.player.opening = -1
	a.playerDidBecomeActive()
	if !p.backgroundAt.IsZero() || a.player.opening < 5*time.Second {
		t.Fatalf("back in sight: background at %v, picture opened again at %v", p.backgroundAt, a.player.opening)
	}
}

// TestPlaybackHandsTheSoundToTheReopenedPicture checks that the picture opened again on coming back plays on with the sound that played in the background, rather than starting the sound again.
func TestPlaybackHandsTheSoundToTheReopenedPicture(t *testing.T) {
	a, _, updates := openClip(t, 12)
	p := a.player.play
	p.toggle()
	sound := p.audio
	if !p.Playing() || sound == nil {
		t.Fatalf("playing %v, sound %v", p.Playing(), sound)
	}
	a.playerDidResignActive()
	// Two seconds out of sight: the picture stood still while the sound played on.
	p.backgroundAt = p.backgroundAt.Add(-2 * time.Second)
	a.playerDidBecomeActive()
	waitUntil(t, updates, func() bool { return a.player.play != p })
	next := a.player.play
	if next.audio != sound || p.audio != nil || next.volume != p.volume {
		t.Fatalf("the sound started again: %p, was %p; volume %v, was %v", next.audio, sound, next.volume, p.volume)
	}
	if !next.Playing() || next.paused || next.playhead() < 2*time.Second {
		t.Fatalf("reopened picture playing %v paused %v at %v", next.Playing(), next.paused, next.playhead())
	}
}

// TestPlaybackFollowsTheSoundBehindTheWindow checks that a picture that stands still in the background (the Tester builds no frames, as a window out of sight) is followed by a decoder of its own, whose picture coming back shows at once, where the sound is, with the same sound and without opening the film again.
func TestPlaybackFollowsTheSoundBehindTheWindow(t *testing.T) {
	a, _, updates := openClip(t, 12)
	p := a.player.play
	p.toggle()
	sound := p.audio
	a.playerDidResignActive()
	bg := p.background
	if bg == nil {
		t.Fatal("no decoder follows the sound")
	}
	deadline := time.Now().Add(5 * time.Second)
	for !bg.opened.Load() {
		if time.Now().After(deadline) {
			t.Fatal("the picture standing still was not noticed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(time.Second)
	gen := a.player.playGen
	want := p.backgroundSound()()
	a.playerDidBecomeActive()
	// Until the picture decoded in the background takes over, the sound is where it played on to, not where the picture stood still.
	if got := p.soundAt(time.Now()); got < want || p.playhead() >= want-backgroundDrift {
		t.Fatalf("just back, sound at %v, picture at %v, want the sound at %v", got, p.playhead(), want)
	}
	waitUntil(t, updates, func() bool { return a.player.play != p })
	next := a.player.play
	if p.returning != nil {
		t.Fatal("the sound clock of the return outlived it")
	}
	if a.player.playGen != gen {
		t.Fatal("the film opened again instead of taking the picture decoded in the background")
	}
	if next.audio != sound || p.audio != nil {
		t.Fatalf("the sound started again: %p, was %p", next.audio, sound)
	}
	if at := next.playhead(); !next.Playing() || at < want-200*time.Millisecond || at > want+300*time.Millisecond {
		t.Fatalf("picture playing %v at %v, sound at %v", next.Playing(), at, want)
	}
	if p.background != nil || next.background != nil {
		t.Fatal("a decoder still follows the sound after coming back")
	}
}

// TestHandedOverPictureJoinsTheSound checks that a picture handed over from the background shows its held frame only until the window shows, and then the frame the sound is in by then, for what is left of it, rather than the held frame for its whole length and every frame after it late.
func TestHandedOverPictureJoinsTheSound(t *testing.T) {
	a, _, _ := openClip(t, 12)
	frames, err := media.OpenFramesAt(a.ffmpegPath(), a.player.play.path, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { frames.Close() })
	img, dur, err := frames.Next()
	if err != nil || dur != 100*time.Millisecond {
		t.Fatalf("first frame %v, %v: want the clip's 10 frames a second", dur, err)
	}
	sound := 450 * time.Millisecond
	dec := &frameDecoder{frames: frames, held: &ui.VideoFrame{Image: img, Duration: dur}, sound: func() time.Duration { return sound }}
	if f, err := dec.Next(); err != nil || f.Duration != time.Millisecond || dec.position() != dur {
		t.Fatalf("held frame for %v at %v, %v", f.Duration, dec.position(), err)
	}
	// The sound at 450ms is in the frame from 400ms to 500ms, 50ms of which are left.
	if f, err := dec.Next(); err != nil || f.Duration != 50*time.Millisecond || dec.position() != 500*time.Millisecond {
		t.Fatalf("frame after the held one for %v, up to %v, %v", f.Duration, dec.position(), err)
	}
	if f, err := dec.Next(); err != nil || f.Duration != 100*time.Millisecond || dec.position() != 600*time.Millisecond {
		t.Fatalf("next frame for %v, up to %v, %v", f.Duration, dec.position(), err)
	}
}

func TestSeekJumpsIntoTheLaterPicture(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=160x90:d=1:r=10",
		"-f", "lavfi", "-i", "color=c=blue:s=160x90:d=1:r=10",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0",
		"-c:v", "mpeg4", "-pix_fmt", "yuv420p", video,
	).CombinedOutput(); err != nil {
		skipWithoutFFmpeg(t, "ffmpeg could not make a clip: %v %s", err, out)
	}
	a, tt, updates := newTestApp(media.Tools{FFmpeg: ffmpeg})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "clip", Source: core.FileURL(video), Stage: core.StageComplete, VideoPath: video,
	}}
	t.Cleanup(a.stopPlayback)
	tt.Frame()
	if err := tt.Click("clip"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(8 * time.Second)
	for a.player.play == nil {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatalf("player did not open: %q", a.player.playErr)
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
		if a.player.play != nil && !a.player.play.paused && a.player.play.playhead() >= time.Second {
			after = sampleStage(tt.Image())
			if after != before {
				return
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	head := time.Duration(0)
	paused := false
	if a.player.play != nil {
		head = a.player.play.playhead()
		paused = a.player.play.paused
	}
	t.Fatalf("picture stayed %v head %s paused %v err %q", after, head, paused, a.player.playErr)
}

func TestFilmChaptersCoverTheTranscript(t *testing.T) {
	cues := []core.Segment{
		{Start: 0, End: 7 * time.Second, Text: "第一句\nFirst."},
		{Start: 7 * time.Second, End: 40 * time.Second, Text: "第二句\nSecond."},
		{Start: 40 * time.Second, End: 70 * time.Second, Text: "第三句\nThird."},
		{Start: 70 * time.Second, End: 100 * time.Second, Text: "第四句\nFourth."},
	}
	chapters := film.Chapters(cues, 0)
	if len(chapters) != 4 {
		t.Fatalf("chapters %d", len(chapters))
	}
	if chapters[0].Title != "第一句" || chapters[0].Start != 0 || chapters[3].End != 100*time.Second {
		t.Fatalf("chapters %#v", chapters)
	}
}

// TestPlayerInEnglish checks the player in English: the title bar, the subtitle list, the 信息 tab with the saved target language named in English, the mode menu and the style view.
func TestPlayerInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	dir := t.TempDir()
	video := filepath.Join(dir, "talk.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	srt := filepath.Join(dir, "talk.bilingual.srt")
	body := "1\n00:00:01,000 --> 00:00:02,500\n你好。\nHello.\n\n2\n00:01:02,004 --> 00:01:05,000\n再见。\nBye.\n\n"
	if err := os.WriteFile(srt, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "job1", Source: core.FileURL(video), Language: "简体中文", Stage: core.StageComplete,
		VideoPath: video, SubtitlePath: srt,
	}}
	tt.Frame()
	if err := tt.Click("talk"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	waitUntil(t, updates, func() bool { return a.player.subs.ready && a.player.film.info.Folder != "" })
	tt.Frame()
	for _, want := range []string{"100% translated", "All", "Click a line to jump to it", "Search Lines", locales.FilmShowInFinder(platformName()), "Back to Library", "Subtitles", "Clips", "Info", "Style"} {
		wantText(t, tt, want)
	}
	if err := tt.Click("Info"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	for _, want := range []string{"Film Folder", "Files", "Source", "Local file", "Job record", "Simplified Chinese", "Bilingual subtitles · ", "Video · "} {
		wantText(t, tt, want)
	}
	// The test app has no ffmpeg, so the video stays as it is while opening, which shows the transport.
	a.player.playPath = video
	tt.Frame()
	for _, want := range []string{"Opening video…", "Opening", "Back 5 Seconds", "Forward 5 Seconds", "Mute", "Volume", "Playback position", "Sidebar", "Full Screen", "Hide Sidebar"} {
		wantText(t, tt, want)
	}
	if err := tt.Click("Subtitles: Bilingual"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	for _, want := range []string{"Display", "Off", "Hides only the subtitles", "Original Only", "Translation Only", "Shows the original, dimmed, until the translation is ready", "Subtitle Delay", "0.0 s", "Style…"} {
		wantText(t, tt, want)
	}
	if err := tt.Click("Style…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, want := range []string{"Playback", "Restore Default Style", "Show Subtitles", "Outline", "Box", "Large", "Font", "Default", "Bilingual Order", "Translation on Top", "Original on Top", "Position", "Higher", "Bottom", "Translation", "Original", "Size", "Color"} {
		wantText(t, tt, want)
	}
	if err := tt.Click("Box"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if look := a.player.look; look.Preset != "box" || look.Translation.Family != "默认" {
		t.Fatalf("look %+v after Box", look)
	}
	// The font menu lists the platform's families in English and the look keeps the saved name of the one chosen: macOS's own fonts on macOS, a sans-serif and a serif font elsewhere.
	font, saved := "PingFang SC", "苹方"
	if breeze.Current() != breeze.Mac {
		font, saved = "Sans Serif", "黑体"
	}
	if err := tt.Click("Default"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click(font); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	if look := a.player.look; look.Translation.Family != saved || look.Original.Family != saved {
		t.Fatalf("look %+v after %s", look, font)
	}
	for _, chinese := range []string{"字幕", "简体中文", locales.In("zh-Hans").FilmShowInFinder(platformName()), "恢复默认样式", saved} {
		if tt.HasText(chinese) {
			t.Fatalf("the player still reads %q: %q", chinese, tt.Texts())
		}
	}
}

func TestFilmSideShowsChaptersAndStyle(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "talk.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	srt := filepath.Join(dir, "talk.bilingual.srt")
	body := "1\n00:00:01,000 --> 00:00:02,500\n你好。\nHello.\n\n2\n00:01:02,004 --> 00:01:05,000\n再见。\nBye.\n\n"
	if err := os.WriteFile(srt, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{
		ID: "job1", Source: core.FileURL(video), Language: "简体中文", Stage: core.StageComplete,
		VideoPath: video, SubtitlePath: srt,
	}}
	tt.Frame()
	if err := tt.Click("talk"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	waitUntil(t, updates, func() bool { return a.player.subs.ready && a.player.film.info.Folder != "" })
	tt.Frame()
	if err := tt.Click("片段"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	// 65 seconds of subtitles make the least 4 片段, each named after the line that started last by its start.
	for _, text := range []string{"片段 1 · 0:00–0:16", "片段 4 · 0:48–1:05", "你好。", "Hello."} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("信息"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, text := range []string{"影片文件夹", films.DisplayPath(dir), "talk.mp4", "talk.bilingual.srt", "本地文件"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	a.SetStyleView(true)
	tt.Frame()
	if !tt.HasText("译文在上") || !tt.HasText("字号") || !tt.HasText("底框") || !tt.HasText("恢复默认样式") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("底框"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if look := a.player.look; look.Preset != "box" || !look.Box || look.Shadow {
		t.Fatalf("look %+v after 底框", look)
	}
	if err := tt.Click("原文在上"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.player.subOrder != film.SubOriginalFirst {
		t.Fatalf("order %q after 原文在上", a.player.subOrder)
	}
	if err := tt.Click("恢复默认样式"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.player.look != film.DefaultLook() {
		t.Fatalf("look %+v after reset", a.player.look)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.player.film.styleView || a.openKey == "" || !tt.HasText("片段") {
		t.Fatalf("Escape left style %v open %q", a.player.film.styleView, a.openKey)
	}
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
	if a.queue.job == nil || a.queue.job.Stage != core.StageComplete || a.openKey != a.queue.job.ID {
		t.Fatalf("open %q job %#v", a.openKey, a.queue.job)
	}
	if !tt.HasText("已完成") || !tt.HasText("返回") {
		t.Fatalf("texts %q", tt.Texts())
	}
	waitUntil(t, updates, func() bool { return a.player.subs.ready })
	tt.Frame()
	if !tt.HasText("字幕文件不在了") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// TestOpenFilmClearsTheTitleBar checks that an open film hides the sidebar and fills the window up to its top edge, its header bar putting the back button after the traffic lights at the same height, with the film's own dark under the title bar.
func TestOpenFilmClearsTheTitleBar(t *testing.T) {
	open := func(a *app) {
		a.library.Jobs = []core.Job{{ID: "j", Source: "https://example.com/v", Stage: core.StageQueued}}
		a.openKey = "j"
	}
	_, plain := titleBarShifts(t, ui.TitleBar{}, open, "返回资料库")
	shifts, barred := titleBarShifts(t, ui.TitleBar{Height: 28, Left: 80}, open, "返回资料库")
	before, _ := plain.Find("返回资料库")
	after, _ := barred.Find("返回资料库")
	if before.X >= 80 || after.X < 120 || abs32(shifts[0]) > 0.5 {
		t.Fatalf("back button at %v without the traffic lights and %v beside them", before, after)
	}
	if barred.HasText("全部影片") {
		t.Fatalf("sidebar shown over an open film: %q", barred.Texts())
	}
	img := barred.Image()
	scale := float32(img.Bounds().Dx()) / 1100
	// The film's own black is under the title bar too.
	if px := img.RGBAAt(int(600*scale), int(4*scale)); px.R > 40 || px.G > 40 || px.B > 40 {
		t.Fatalf("title bar over an open film = %v, want the film's dark", px)
	}
}

// TestFilmInTheColumnStartsAtItsEdge checks that a film opened from the YouTube page, which shows in the content column beside the sidebar, puts its back button the prototype's padding of 20 from the column's edge rather than leaving room for the traffic lights, which are over the sidebar.
func TestFilmInTheColumnStartsAtItsEdge(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 80})
	a.page = pageYouTube
	a.library.Jobs = []core.Job{{ID: "j", Source: "https://www.youtube.com/watch?v=abc", Stage: core.StageQueued}}
	a.openKey = "j"
	tt.Frame()
	if !tt.HasText("全部影片") {
		t.Fatalf("sidebar hidden beside a film from the YouTube page: %q", tt.Texts())
	}
	back, ok := tt.Find("返回资料库")
	if !ok {
		t.Fatalf("missing the back button in %q", tt.Texts())
	}
	edge := float32(sidebarWidth + pagePadX)
	if back.X < edge || back.X-edge > 20 {
		t.Fatalf("back button at %v, want it within 20 of the column's edge at %v", back, edge)
	}
}

// TestRefreshClosesAFilmThatLeftTheShelf checks that an open video closes, and its playback stops, when a refresh of the shelf no longer finds it, without a frame being drawn.
func TestRefreshClosesAFilmThatLeftTheShelf(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
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
	a.openKey = items[0].Key
	a.player.playPath = items[0].VideoPath
	a.refreshLibrary()
	if a.openKey == "" {
		t.Fatal("the film closed while it is still on the shelf")
	}
	if err := os.Remove(items[0].VideoPath); err != nil {
		t.Fatal(err)
	}
	a.refreshLibrary()
	if a.openKey != "" || a.player.playPath != "" {
		t.Fatalf("open %q playing %q after the film left the shelf", a.openKey, a.player.playPath)
	}
}

// TestSubtitleKeys checks V, S, Z and X on an open film: V hides the subtitles and brings back the mode they had, S steps original, translation, bilingual, and Z and X move them a tenth of a second, each saying so on the picture for a moment.
func TestSubtitleKeys(t *testing.T) {
	a, tt, _ := openClip(t, 2)
	key := func(k ui.Key, mode, osd string) {
		t.Helper()
		tt.Key(0, k)
		tt.Frame()
		if a.player.subMode != mode || !tt.HasText(osd) {
			t.Fatalf("after %v: mode %q, want %q; texts %q", k, a.player.subMode, mode, tt.Texts())
		}
	}
	key(ui.KeyS, film.SubOriginal, "字幕：仅原文")
	key(ui.KeyV, film.SubOff, "字幕：关闭")
	key(ui.KeyV, film.SubOriginal, "字幕：仅原文")
	key(ui.KeyS, film.SubTranslation, "字幕：仅译文")
	key(ui.KeyS, film.SubBilingual, "字幕：双语")
	key(ui.KeyZ, film.SubBilingual, "字幕 -0.1 秒")
	key(ui.KeyZ, film.SubBilingual, "字幕 -0.2 秒")
	key(ui.KeyX, film.SubBilingual, "字幕 -0.1 秒")
	if a.player.film.delay != -100*time.Millisecond {
		t.Fatalf("delay %v", a.player.film.delay)
	}
	tt.Advance(1500 * time.Millisecond)
	tt.Frame()
	if tt.HasText("字幕 -0.1 秒") {
		t.Fatalf("the message stayed on the picture: %q", tt.Texts())
	}
}

// TestModeMenuPicksTheMode opens the transport's subtitle menu, picks a mode in it, and checks that Escape closes the menu without leaving the film.
func TestModeMenuPicksTheMode(t *testing.T) {
	a, tt, _ := openClip(t, 2)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("%v; texts %q", err, tt.Texts())
		}
		tt.Frame()
	}
	click("字幕：双语")
	if !a.player.film.modeMenu || !tt.HasText("字幕延迟") {
		t.Fatalf("menu not open: %q", tt.Texts())
	}
	click("仅原文")
	if a.player.subMode != film.SubOriginal || a.player.film.modeMenu || tt.HasText("字幕延迟") {
		t.Fatalf("mode %q menu %v", a.player.subMode, a.player.film.modeMenu)
	}
	click("字幕：仅原文")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.player.film.modeMenu || a.openKey == "" {
		t.Fatalf("Escape: menu %v open %q", a.player.film.modeMenu, a.openKey)
	}
	click("字幕：仅原文")
	click("样式…")
	if !a.player.film.styleView || !a.player.panel || !tt.HasText("恢复默认样式") {
		t.Fatalf("style view %v panel %v", a.player.film.styleView, a.player.panel)
	}
}

// TestFindFocusesTheSubtitleSearch checks that ⌘F opens the side panel on its subtitle list, with the keyboard in its search field, and that ⇧⌘T hides and shows the panel.
func TestFindFocusesTheSubtitleSearch(t *testing.T) {
	a, tt, _ := openClip(t, 2)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if a.player.panel || tt.HasText("搜索台词") {
		t.Fatalf("⇧⌘T left the panel: %q", tt.Texts())
	}
	a.player.filmSide = "info"
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Frame()
	if !a.player.panel || a.player.filmSide != "" || !tt.Focused("搜索台词") {
		t.Fatalf("panel %v side %q focused %v", a.player.panel, a.player.filmSide, tt.Focused("搜索台词"))
	}
	tt.Type("river")
	tt.Frame()
	if a.player.film.search != "river" {
		t.Fatalf("search %q", a.player.film.search)
	}
}

// TestPanelLeavesThePictureAlone checks that the open side panel sits beside the picture rather than over it, so nothing of a film (a screencast's text, say) is hidden under it, and that the picture takes the whole window again once the panel is hidden.
func TestPanelLeavesThePictureAlone(t *testing.T) {
	a, tt, _ := openClip(t, 2)
	picture, ok := tt.Find("画面")
	if !ok || !a.player.panel {
		t.Fatalf("picture %v panel %v: %q", ok, a.player.panel, tt.Texts())
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	tt.Frame()
	whole, _ := tt.Find("画面")
	if a.player.panel || whole.X != 0 || whole.W <= picture.W {
		t.Fatalf("panel %v: picture %+v, with the panel %+v", a.player.panel, whole, picture)
	}
	// The panel is 340 points wide, 16 in from the window's right edge.
	if under := picture.X + picture.W - (whole.W - 16 - 340); under > 0 {
		t.Fatalf("with the panel open the picture %+v reaches %v points under it in a %v-point window", picture, under, whole.W)
	}
}

// TestPanelAnimatesIntoThePictureRoom checks that closing the side panel grows
// the picture room over time instead of jumping straight to its final width.
func TestPanelAnimatesIntoThePictureRoom(t *testing.T) {
	_, tt, _ := openClip(t, 2)
	initial, ok := tt.Find("画面")
	if !ok {
		t.Fatal("picture is missing before the toggle")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
	tt.Advance(150 * time.Millisecond)
	tt.Frame()
	middle, ok := tt.Find("画面")
	if !ok || middle.W <= initial.W {
		t.Fatalf("picture did not grow during panel animation: initial %+v, middle %+v", initial, middle)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	final, ok := tt.Find("画面")
	if !ok || final.W <= middle.W {
		t.Fatalf("picture did not finish growing after panel animation: middle %+v, final %+v", middle, final)
	}
}

// TestTopBarTogglesThePanel checks the button at the title bar's end that folds the side panel away and brings it back, named for what a click does.
func TestTopBarTogglesThePanel(t *testing.T) {
	a, tt, _ := openClip(t, 2)
	if !a.player.panel {
		t.Fatalf("panel off on opening")
	}
	if err := tt.Click("收起侧栏"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if a.player.panel || tt.HasText("搜索台词") {
		t.Fatalf("收起侧栏 left the panel: %q", tt.Texts())
	}
	if err := tt.Click("展开侧栏"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if !a.player.panel || !tt.HasText("搜索台词") {
		t.Fatalf("展开侧栏 did not bring the panel back: %q", tt.Texts())
	}
}

// TestPanelShowsWithoutAPicture checks that a film with no picture, here for want of ffmpeg, shows its side panel even when the panel was hidden on the film before, since it has no transport to bring the panel back from.
func TestPanelShowsWithoutAPicture(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.library.Jobs = []core.Job{{ID: "j", Source: "https://example.com/v", Stage: core.StageQueued}}
	a.player.panel = false
	a.openKey = "j"
	tt.Frame()
	if _, ok := tt.Find("侧栏"); !ok || !tt.HasText("搜索台词") {
		t.Fatalf("no side panel on a film without a picture: %q", tt.Texts())
	}
}

// TestControlsFadeWhileTheFilmPlays checks that the controls and the pointer hide once the pointer rests for 2.5 seconds on a playing film, and come back when it moves.
func TestControlsFadeWhileTheFilmPlays(t *testing.T) {
	a, tt, _ := openClip(t, 10)
	tt.Move(400, 300)
	tt.Frame()
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if !a.player.play.player.Playing() {
		t.Fatal("Space did not start the film")
	}
	tt.Advance(time.Second)
	tt.Frame()
	if tt.Cursor() == ui.CursorNone {
		t.Fatal("the pointer hid a second after it moved")
	}
	tt.Advance(2 * time.Second)
	tt.Frame()
	if tt.Cursor() != ui.CursorNone {
		t.Fatalf("cursor %v after the pointer rested", tt.Cursor())
	}
	tt.Move(420, 310)
	tt.Frame()
	if tt.Cursor() == ui.CursorNone {
		t.Fatal("moving the pointer did not bring the controls back")
	}
	tt.Key(0, ui.KeySpace)
	tt.Advance(3 * time.Second)
	tt.Frame()
	if tt.Cursor() == ui.CursorNone {
		t.Fatal("the controls hid while the film was held")
	}
}

// TestScrubberSeeksOnRelease drags the scrubber and checks that the video opens again at the new time only once the pointer lets go, since each seek starts the decoders over.
func TestScrubberSeeksOnRelease(t *testing.T) {
	a, tt, updates := openClip(t, 12)
	r, ok := tt.Find("进度")
	if !ok {
		t.Fatalf("no scrubber in %q", tt.Texts())
	}
	y := r.Y + r.H/2
	gen := a.player.playGen
	tt.Press(r.X+r.W*0.25, y)
	tt.Frame()
	tt.Move(r.X+r.W*0.75, y)
	tt.Frame()
	if a.player.playGen != gen {
		t.Fatal("the video opened again while the scrubber was held")
	}
	// The value follows the pointer across the track, past the middle.
	want := time.Duration(a.player.film.scrub * float64(time.Second))
	if want < a.PlayLength()/2 || !tt.HasText(cueClockOf(want)) {
		t.Fatalf("the preview does not show where the scrubber is: %q", tt.Texts())
	}
	tt.Release(r.X+r.W*0.75, y)
	tt.Frame()
	if a.player.playGen == gen {
		t.Fatal("releasing the scrubber did not seek")
	}
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	if head := a.player.play.playhead(); head < want-time.Second || head > want+time.Second {
		t.Fatalf("head %v after the drag, want about %v", head, want)
	}
}

// cueClockOf writes d as the player's clock does, m:ss.
func cueClockOf(d time.Duration) string {
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// TestOpeningAFilmKeepsWhereItStopped opens a film that stopped at 0:06 last time from its poster, which holds it on its first frame as the prototype's poster does, and goes back without playing it: the film still continues from 0:06, in the app and in library.json, and the hero still offers 从 0:06 继续.
func TestOpeningAFilmKeepsWhereItStopped(t *testing.T) {
	a, tt, _ := openClip(t, 12)
	path := a.player.play.path
	a.player.positions = map[string]time.Duration{path: 6 * time.Second}
	a.areasPositionsChanged()
	until := time.Now().Add(4 * time.Second)
	for a.player.play.playhead() <= 0 && time.Now().Before(until) {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
		time.Sleep(20 * time.Millisecond)
	}
	if a.player.play.Playing() || a.player.play.playhead() <= 0 {
		t.Fatalf("the film is not held on its first frame: playing %v head %v", a.player.play.Playing(), a.player.play.playhead())
	}
	if err := tt.Click("返回资料库"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if at := a.ResumeAt(path); at != 6*time.Second {
		t.Fatalf("ResumeAt after opening and going back = %v, want 6s", at)
	}
	if at := a.library.Positions[path]; at != 6*time.Second {
		t.Fatalf("library.json keeps %v for the film, want 6s", at)
	}
	if !tt.HasText("从 0:06 继续") {
		t.Fatalf("hero %q", tt.Texts())
	}
}

// TestContinuingAFilmKeepsWhereItGotTo continues a film from 0:06 with the hero's 从 0:06 继续, which opens it playing without play being pressed, lets it play on and goes back: where it got to past 0:06 is kept, in the app and in library.json, as a playback opened playing at a point counts as played.
func TestContinuingAFilmKeepsWhereItGotTo(t *testing.T) {
	a, tt, updates := openClip(t, 12)
	path := a.player.play.path
	if err := tt.Click("返回资料库"); err != nil {
		t.Fatal(err)
	}
	a.player.positions = map[string]time.Duration{path: 6 * time.Second}
	a.areasPositionsChanged()
	tt.Frame()
	if err := tt.Click("从 0:06 继续"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	until := time.Now().Add(4 * time.Second)
	for a.player.play.playhead() <= 6500*time.Millisecond && time.Now().Before(until) {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
		time.Sleep(20 * time.Millisecond)
	}
	if !a.player.play.Playing() || a.player.play.playhead() <= 6500*time.Millisecond {
		t.Fatalf("the film did not play on from 0:06: playing %v head %v", a.player.play.Playing(), a.player.play.playhead())
	}
	if err := tt.Click("返回资料库"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if at := a.ResumeAt(path); at <= 6500*time.Millisecond {
		t.Fatalf("ResumeAt after continuing and going back = %v, want past 0:06.5", at)
	}
	if at := a.library.Positions[path]; at != a.ResumeAt(path) {
		t.Fatalf("library.json keeps %v for the film, want %v", at, a.ResumeAt(path))
	}
}

// TestQuittingStopsTheOpenFilm: when the app quits with a film playing, the film stops, so its sound, which ffmpeg decodes in a process of its own, ends with the app, and where it stopped is kept for next time.
func TestQuittingStopsTheOpenFilm(t *testing.T) {
	a, tt, _ := openClip(t, 5)
	path := a.player.play.path
	tt.Key(0, ui.KeySpace)
	until := time.Now().Add(4 * time.Second)
	for a.player.play.playhead() <= 0 && time.Now().Before(until) {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
		time.Sleep(20 * time.Millisecond)
	}
	if a.player.play.playhead() <= 0 {
		t.Fatal("the film did not start")
	}
	a.areasQuitting()
	if a.player.play != nil {
		t.Fatal("the film still plays after the app quit")
	}
	if a.ResumeAt(path) <= 0 {
		t.Fatalf("where the film stopped was not kept: %v", a.player.positions)
	}
}

// TestThePlayerKeepsTheDisplayAwakeWhileAFilmPlays: a film held on its first frame takes no hold; playing it keeps the display awake, and pausing it, playing it to its end, going back to the library and quitting each let the display sleep again.
func TestThePlayerKeepsTheDisplayAwakeWhileAFilmPlays(t *testing.T) {
	a, tt, updates := openClip(t, 1)
	awake := &fakeAwake{}
	a.player.keepAwake = awake.keepAwake
	tt.Frame()
	if len(awake.holds) != 0 {
		t.Fatalf("a film held on its first frame took holds %+v", awake.holds)
	}
	play := func(when string) {
		t.Helper()
		tt.Key(0, ui.KeySpace)
		tt.Frame()
		if !a.player.play.Playing() || awake.held != 1 {
			t.Fatalf("%s: playing %v, holds %+v, %d held", when, a.player.play.Playing(), awake.holds, awake.held)
		}
	}
	play("pressing Space")
	if hold := awake.holds[0]; hold.reason != "正在播放视频" || !hold.display {
		t.Fatalf("the hold is %+v, want the reason 正在播放视频 and the display kept on", hold)
	}
	tt.Frame()
	if len(awake.holds) != 1 {
		t.Fatalf("another frame of the playing film took another hold: %+v", awake.holds)
	}

	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if awake.held != 0 {
		t.Fatalf("after pausing: holds %+v, %d held", awake.holds, awake.held)
	}

	play("playing again")
	until := time.Now().Add(4 * time.Second)
	for a.player.play.Playing() && time.Now().Before(until) {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
		time.Sleep(20 * time.Millisecond)
	}
	if a.player.play.Playing() || awake.held != 0 {
		t.Fatalf("after the film's end: playing %v, holds %+v, %d held", a.player.play.Playing(), awake.holds, awake.held)
	}

	play("playing from the start")
	if err := tt.Click("返回资料库"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.player.play != nil || awake.held != 0 {
		t.Fatalf("after going back to the library: holds %+v, %d held", awake.holds, awake.held)
	}

	if err := tt.Click("clip"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	tt.Frame()
	play("playing the film opened again")
	a.areasQuitting()
	if awake.held != 0 {
		t.Fatalf("after quitting: holds %+v, %d held", awake.holds, awake.held)
	}
}

// TestTheDisplayHoldFollowsTheWindow: a playing film lets the display sleep while the window is minimized or hidden, as only its sound plays on, and keeps it awake again once the window is back; a seek keeps the one hold while it opens the film again.
func TestTheDisplayHoldFollowsTheWindow(t *testing.T) {
	a, tt, updates := openClip(t, 12)
	awake := &fakeAwake{}
	a.player.keepAwake = awake.keepAwake
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if !a.player.play.Playing() || awake.held != 1 {
		t.Fatalf("playing %v, holds %+v, %d held", a.player.play.Playing(), awake.holds, awake.held)
	}

	a.playerWindowAway(true)
	if awake.held != 0 {
		t.Fatalf("with the window away: holds %+v, %d held", awake.holds, awake.held)
	}
	a.areasAfterFrame()
	if awake.held != 0 || len(awake.holds) != 1 {
		t.Fatalf("a frame with the window away took the hold again: %+v", awake.holds)
	}
	a.playerWindowAway(false)
	if awake.held != 1 || len(awake.holds) != 2 {
		t.Fatalf("with the window back: holds %+v, %d held", awake.holds, awake.held)
	}

	tt.Key(0, ui.KeyRight)
	if a.player.play != nil {
		t.Fatal("the seek did not open the film again")
	}
	tt.Frame()
	if awake.held != 1 || len(awake.holds) != 2 {
		t.Fatalf("while the seek opens the film: holds %+v, %d held", awake.holds, awake.held)
	}
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	tt.Frame()
	if !a.player.play.Playing() || awake.held != 1 || len(awake.holds) != 2 {
		t.Fatalf("after the seek: playing %v, holds %+v, %d held", a.player.play.Playing(), awake.holds, awake.held)
	}
}

// TestTheDisplaySleepsWhileThePictureStandsStillBehindTheWindow: once the picture of a film playing in the background stands still, as the window is out of sight, the display may sleep while its sound plays on, and coming back keeps it awake again.
func TestTheDisplaySleepsWhileThePictureStandsStillBehindTheWindow(t *testing.T) {
	a, tt, updates := openClip(t, 12)
	awake := &fakeAwake{}
	a.player.keepAwake = awake.keepAwake
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	p := a.player.play
	if !p.Playing() || awake.held != 1 {
		t.Fatalf("playing %v, holds %+v, %d held", p.Playing(), awake.holds, awake.held)
	}
	a.playerDidResignActive()
	if p.background == nil {
		t.Fatal("no decoder follows the sound")
	}
	// The Tester builds no frames meanwhile, as a window out of sight.
	waitUntil(t, updates, func() bool { return p.unseen })
	if awake.held != 0 {
		t.Fatalf("with the picture standing still: holds %+v, %d held", awake.holds, awake.held)
	}
	a.areasAfterFrame()
	if awake.held != 0 || len(awake.holds) != 1 {
		t.Fatalf("a frame behind the window took the hold again: %+v", awake.holds)
	}

	a.playerDidBecomeActive()
	waitUntil(t, updates, func() bool { return a.player.play != p })
	tt.Frame()
	if !a.player.play.Playing() || awake.held != 1 || len(awake.holds) != 2 {
		t.Fatalf("back in sight: playing %v, holds %+v, %d held", a.player.play.Playing(), awake.holds, awake.held)
	}
}

// TestVolumeKeys checks ↑, ↓ and M on an open film: ↑ and ↓ move the volume a tenth within 0 and 100%, M mutes and gives the volume back, and each says so on the picture. The open film plays at the new volume at once, and once the keys rest the settings keep it.
func TestVolumeKeys(t *testing.T) {
	a, tt, updates := openClip(t, 2)
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	key := func(k ui.Key, volume float64, muted bool, osd string) {
		t.Helper()
		tt.Key(0, k)
		tt.Frame()
		if math.Abs(a.player.volume-volume) > 1e-9 || a.player.muted != muted || !tt.HasText(osd) {
			t.Fatalf("after %v: volume %v muted %v, want %v %v; texts %q", k, a.player.volume, a.player.muted, volume, muted, tt.Texts())
		}
	}
	key(ui.KeyUp, 1, false, "音量 100%")
	key(ui.KeyDown, 0.9, false, "音量 90%")
	key(ui.KeyDown, 0.8, false, "音量 80%")
	key(ui.KeyM, 0.8, true, "静音")
	if !tt.HasText("取消静音") {
		t.Fatalf("the speaker does not offer to unmute: %q", tt.Texts())
	}
	waitUntil(t, updates, func() bool { return a.player.play.volume == 0 })
	key(ui.KeyM, 0.8, false, "音量 80%")
	key(ui.KeyM, 0.8, true, "静音")
	// ↑ while muted unmutes, a step above the volume the mute kept.
	key(ui.KeyUp, 0.9, false, "音量 90%")
	if math.Abs(a.player.play.volume-0.9) > 1e-9 {
		t.Fatalf("the open film plays at %v after ↑, want 0.9 at once", a.player.play.volume)
	}
	waitUntil(t, updates, func() bool { return !a.player.volumeUnsaved })
	saved := loadSavedSettings(a.settingsPath)
	if saved.Volume == nil || math.Abs(*saved.Volume-0.9) > 1e-9 || saved.Muted {
		t.Fatalf("saved volume %v muted %v", saved.Volume, saved.Muted)
	}
	for range 12 {
		tt.Key(0, ui.KeyDown)
	}
	tt.Frame()
	if a.player.volume != 0 || !tt.HasText("音量 0%") || !tt.HasText("取消静音") {
		t.Fatalf("volume %v after ↓ to the bottom; texts %q", a.player.volume, tt.Texts())
	}
	// M on a volume of 0 makes it heard rather than muting it.
	key(ui.KeyM, 0.5, false, "音量 50%")
}

// TestMuteButtonMutes clicks the transport's speaker: it mutes the film and turns into 取消静音, which gives the volume back.
func TestMuteButtonMutes(t *testing.T) {
	a, tt, updates := openClip(t, 2)
	a.player.film.moved = tt.Now()
	tt.Frame()
	if err := tt.Click("静音"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if !a.player.muted {
		t.Fatal("the speaker did not mute the film")
	}
	waitUntil(t, updates, func() bool { return a.player.play.volume == 0 })
	if err := tt.Click("取消静音"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if a.player.muted || a.player.volume != 1 {
		t.Fatalf("muted %v volume %v after 取消静音", a.player.muted, a.player.volume)
	}
	waitUntil(t, updates, func() bool { return a.player.play.volume == 1 })
}

// TestVolumeSettingsRoundTrip checks that the volume and mute are kept in the settings, and that a volume out of range is brought back within 0 and 1.
func TestVolumeSettingsRoundTrip(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.player.volume = 0.3
	a.player.muted = true
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}
	b, _, _ := newTestApp(&fakePipeline{})
	b.restorePlayer(loadSavedSettings(a.settingsPath))
	if b.player.volume != 0.3 || !b.player.muted || b.playerVolume() != 0 {
		t.Fatalf("restored volume %v muted %v", b.player.volume, b.player.muted)
	}
	loud := 3.0
	c, _, _ := newTestApp(&fakePipeline{})
	c.restorePlayer(savedSettings{savedPlayer: savedPlayer{Volume: &loud}})
	if c.player.volume != 1 || c.player.muted {
		t.Fatalf("volume %v muted %v from a saved 3", c.player.volume, c.player.muted)
	}
	d, _, _ := newTestApp(&fakePipeline{})
	d.restorePlayer(savedSettings{})
	if d.player.volume != 1 || d.player.muted {
		t.Fatalf("default volume %v muted %v", d.player.volume, d.player.muted)
	}
}

// TestQuittingKeepsTheVolume: a volume changed too shortly before quitting for the change to have settled is still kept in the settings.
func TestQuittingKeepsTheVolume(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.ToggleMute()
	a.areasQuitting()
	if saved := loadSavedSettings(a.settingsPath); !saved.Muted {
		t.Fatalf("the mute was not kept: %+v", saved.savedPlayer)
	}
}

// TestSoundRestartsWhereItPlayedOn: back from the background, before the picture has caught up, the sound restarts where it has played on to, not where the picture stood still.
func TestSoundRestartsWhereItPlayedOn(t *testing.T) {
	now := time.Now()
	p := &filmPlay{backgroundAt: now.Add(-3 * time.Minute), backgroundHead: 10 * time.Second}
	if got := p.soundAt(now); got != 3*time.Minute+10*time.Second {
		t.Fatalf("sound at %v, want 3m10s", got)
	}
	p.backgroundAt = time.Time{}
	if got := p.soundAt(now); got != 0 {
		t.Fatalf("sound at %v with the picture at the start", got)
	}
}

// fakeScreen stands in for the main window's full screen, which tests have no window for.
type fakeScreen struct{ full bool }

func (s *fakeScreen) IsFullScreen() bool   { return s.full }
func (s *fakeScreen) SetFullScreen(v bool) { s.full = v }
func (s *fakeScreen) ToggleFullScreen()    { s.full = !s.full }

// TestEscapeLeavesFullScreenFirst checks that Esc on a film in full screen leaves full screen and keeps the film open, and that the next Esc goes back to the library.
func TestEscapeLeavesFullScreenFirst(t *testing.T) {
	a, tt, _ := openClip(t, 2)
	screen := &fakeScreen{}
	a.player.screen = screen
	tt.Key(0, ui.KeyF)
	tt.Frame()
	if !screen.full {
		t.Fatal("F did not enter full screen")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if screen.full || a.openKey == "" {
		t.Fatalf("first Escape: full screen %v open %q", screen.full, a.openKey)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if screen.full || a.openKey != "" {
		t.Fatalf("second Escape: full screen %v open %q", screen.full, a.openKey)
	}
}

// TestDoubleClickOnThePictureTogglesFullScreen checks that a double click on the picture enters full screen and the next one leaves it, while the film plays on: the double click's two clicks start and hold it, which cancel out.
func TestDoubleClickOnThePictureTogglesFullScreen(t *testing.T) {
	a, tt, _ := openClip(t, 10)
	screen := &fakeScreen{}
	a.player.screen = screen
	picture, ok := tt.Find("画面")
	if !ok {
		t.Fatalf("no picture: %q", tt.Texts())
	}
	// Left of the big play button in the middle, below the title bar and above the subtitles and the transport.
	x, y := picture.X+picture.W/4, picture.Y+picture.H*2/5
	tt.Advance(time.Second)
	tt.ClickAt(x, y)
	tt.Frame()
	if !a.player.play.player.Playing() || screen.full {
		t.Fatalf("a click: playing %v full screen %v", a.player.play.player.Playing(), screen.full)
	}
	double := func(want bool) {
		t.Helper()
		tt.Advance(time.Second)
		tt.ClickAt(x, y)
		tt.ClickAt(x, y)
		tt.Frame()
		if screen.full != want || !a.player.play.player.Playing() {
			t.Fatalf("double click: full screen %v, want %v; playing %v", screen.full, want, a.player.play.player.Playing())
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

// TestPlayingFilmIsKeptOnDisk plays a film without closing it and checks that where it got to reaches library.json now and then, so a crash or a force quit does not lose it: not on every frame, but once it has played on for playerKeepEvery, at once when it pauses, and at once where a seek lands. A film held on its first frame is not kept.
func TestPlayingFilmIsKeptOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a := openStill(t, path, "clip", "/v.mp4", 0)
	play := a.player.play
	play.duration = 10 * time.Minute
	play.player.Play()
	start := time.Date(2026, 11, 10, 20, 0, 0, 0, time.UTC)
	at := func(head, since time.Duration) {
		t.Helper()
		play.frames.shown = head
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
	want(20*time.Second, "a frame after a save")

	play.toggle()
	at(40*time.Second, 40*time.Second)
	want(40*time.Second, "paused")
	at(40*time.Second, 80*time.Second)
	want(40*time.Second, "held")

	seeked := &filmPlay{player: play.player, frames: &frameDecoder{shown: 5 * time.Minute}, path: "/v.mp4", duration: 10 * time.Minute, played: true}
	a.player.play = seeked
	a.areasAfterFrame()
	want(5*time.Minute, "a seek")

	poster := &filmPlay{player: play.player, frames: &frameDecoder{shown: 3 * time.Second}, path: "/v.mp4", duration: 10 * time.Minute}
	a.player.play = poster
	a.playerKeep(start.Add(2 * time.Minute))
	a.playerKeep(start.Add(5 * time.Minute))
	want(5*time.Minute, "a film held on its first frame")

	ending := &filmPlay{player: play.player, frames: &frameDecoder{shown: 9 * time.Minute}, path: "/v.mp4", duration: 10 * time.Minute, played: true}
	ending.player.Play()
	a.player.play = ending
	a.playerKeep(start.Add(6 * time.Minute))
	want(9*time.Minute, "a seek into the last minute")
	ending.player.Pause()
	ending.finish()
	a.playerKeep(start.Add(7 * time.Minute))
	if got, ok := onDisk(t, path).Positions["/v.mp4"]; ok {
		t.Fatalf("a film played to its end is kept at %v, want it to start over", got)
	}
}

// TestSubtitleDelayStaysWithItsFilm moves the subtitles of one film and checks that they are moved again when it opens again, in this launch and the next, that another film opens with its own in place, and that a run of Z presses is saved once it rests rather than on every press.
func TestSubtitleDelayStaysWithItsFilm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(&fakePipeline{})
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

	again, _, _ := newTestApp(&fakePipeline{})
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

// TestSubtitleDelayFollowsAMovedVideo checks that the delay of a video that moved on disk is kept by its new path, with the film open and in library.json.
func TestSubtitleDelayFollowsAMovedVideo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(&fakePipeline{})
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

// TestSubtitleDelayStaysWithAFilmOpenedPlaying opens films playing, as the hero's 播放, 从 X 继续 and 从头看 and the palette's jump into another film do (OpenAt), and checks that each opens with its own subtitle delay and keeps a change by its own video, while a jump to a line of the open film keeps the film as it is, as a seek does.
func TestSubtitleDelayStaysWithAFilmOpenedPlaying(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(&fakePipeline{})
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

// playerFakeSound is a clip of seconds that the fake ffmpeg, which it returns, plays the sound of (fakeSound).
func playerFakeSound(t *testing.T, seconds int) (ffmpeg, clip string) {
	t.Helper()
	dir := t.TempDir()
	ffmpeg = fakeTool(t, filepath.Join(dir, "bin"), "ffmpeg")
	clip = filepath.Join(dir, "clip.mp4")
	writeFakeClip(t, clip, seconds, "202020", "101010")
	return ffmpeg, clip
}

// playerReadSound reads d until it has frames frames, or to its end when frames is negative, in reads of an odd number of frames, so that the pipe hands over frames cut short. It fails t on a read that returns nothing short of the end, which the player takes for the end.
func playerReadSound(t *testing.T, d *playerAudioDecoder, frames int) []float32 {
	t.Helper()
	var got []float32
	buf := make([]float32, 1001*playerAudioChannels)
	for frames < 0 || len(got) < frames*playerAudioChannels {
		n, err := d.Read(buf)
		got = append(got, buf[:n]...)
		if err == io.EOF {
			return got
		}
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 || n%playerAudioChannels != 0 {
			t.Fatalf("a read of %d samples", n)
		}
	}
	return got
}

// playerWantSound fails t unless samples are the fake clip's sound from frame on, the same on both channels.
func playerWantSound(t *testing.T, samples []float32, frame int64) {
	t.Helper()
	for i := 0; i < len(samples); i += playerAudioChannels {
		want := fakeSoundSample(frame + int64(i/playerAudioChannels))
		if samples[i] != want || samples[i+1] != want {
			t.Fatalf("frame %d: %v %v, want the clip's frame %d (%v)", i/playerAudioChannels, samples[i], samples[i+1], frame+int64(i/playerAudioChannels), want)
		}
	}
}

// TestPlayerAudioDecodesTheSoundFrom: the sound of a film opened partway is ffmpeg's 48 kHz stereo floats from there to the end of the clip, every frame in order, and then the end.
func TestPlayerAudioDecodesTheSoundFrom(t *testing.T) {
	ffmpeg, clip := playerFakeSound(t, 2)
	d := &playerAudioDecoder{ffmpeg: ffmpeg, path: clip, from: 1500 * time.Millisecond}
	if err := d.start(0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if d.SampleRate() != 48000 || d.Channels() != 2 || d.Len() != -1 {
		t.Fatalf("rate %d channels %d length %d", d.SampleRate(), d.Channels(), d.Len())
	}
	if args := d.proc.cmd.Args; fakeArg(args, "-ss") != "1.500" || fakeArg(args, "-f") != "f32le" || fakeArg(args, "-ac") != "2" || fakeArg(args, "-ar") != "48000" || fakeArg(args, "-i") != clip {
		t.Fatalf("ffmpeg %v", args)
	}
	got := playerReadSound(t, d, -1)
	if len(got) != 24000*playerAudioChannels {
		t.Fatalf("%d frames of sound, want the last half second's 24000", len(got)/playerAudioChannels)
	}
	playerWantSound(t, got, 72000)
	if n, err := d.Read(make([]float32, 64)); n != 0 || err != io.EOF {
		t.Fatalf("after the end: %d %v", n, err)
	}
}

// TestPlayerAudioSetPositionStartsFFmpegAgain: moving the sound further than the decoder can reach on the ffmpeg it has, back past what it kept or far on past what it read, ends that ffmpeg and starts another where it moved to, counted from where the sound was opened.
func TestPlayerAudioSetPositionStartsFFmpegAgain(t *testing.T) {
	ffmpeg, clip := playerFakeSound(t, 12)
	d := &playerAudioDecoder{ffmpeg: ffmpeg, path: clip, from: time.Second}
	if err := d.start(0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	playerWantSound(t, playerReadSound(t, d, 4800), 48000)
	old := d.proc
	if err := d.SetPosition(6 * 48000); err != nil {
		t.Fatal(err)
	}
	if old.cmd.ProcessState == nil {
		t.Fatal("the ffmpeg before the move was left running")
	}
	if d.proc == old || fakeArg(d.proc.cmd.Args, "-ss") != "7.000" {
		t.Fatalf("ffmpeg after the move on %v", d.proc.cmd.Args)
	}
	playerWantSound(t, playerReadSound(t, d, 4800), 7*48000)
	old = d.proc
	if err := d.SetPosition(24000); err != nil {
		t.Fatal(err)
	}
	if old.cmd.ProcessState == nil || d.proc == old || fakeArg(d.proc.cmd.Args, "-ss") != "1.500" {
		t.Fatalf("ffmpeg after the move back %v", d.proc.cmd.Args)
	}
	playerWantSound(t, playerReadSound(t, d, 4800), 72000)
}

// TestPlayerAudioSetPositionNearbyKeepsFFmpeg: moving the sound back into the frames it read lately gives them again and goes on from ffmpeg without a gap, and moving it a few seconds on past what it read drops ffmpeg's output up to there, both on the same ffmpeg, as catching up with the picture does.
func TestPlayerAudioSetPositionNearbyKeepsFFmpeg(t *testing.T) {
	ffmpeg, clip := playerFakeSound(t, 6)
	d := &playerAudioDecoder{ffmpeg: ffmpeg, path: clip, from: time.Second}
	if err := d.start(0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	playerWantSound(t, playerReadSound(t, d, 9600), 48000)
	proc := d.proc
	if err := d.SetPosition(4800); err != nil {
		t.Fatal(err)
	}
	playerWantSound(t, playerReadSound(t, d, 9600), 48000+4800)
	on := d.keptEnd + 3*48000
	if err := d.SetPosition(on); err != nil {
		t.Fatal(err)
	}
	playerWantSound(t, playerReadSound(t, d, 4800), 48000+on)
	if d.proc != proc || proc.cmd.ProcessState != nil {
		t.Fatalf("ffmpeg started again for a nearby move: %v", d.proc.cmd.Args)
	}
	// Past the end of the track, the sound ends there.
	if err := d.SetPosition(d.keptEnd + 2*48000); err != nil {
		t.Fatal(err)
	}
	if n, err := d.Read(make([]float32, 64)); n != 0 || err != io.EOF {
		t.Fatalf("a read past the end: %d %v", n, err)
	}
}

// TestSoundCatchesUpWithThePicture: once a sound asked to play is heard to move, a sound that trails the time the picture has played since by more than playerSoundLag moves on to where the picture is, once; one that keeps pace is left alone, and one never heard to move is left alone after playerSoundGiveUp.
func TestSoundCatchesUpWithThePicture(t *testing.T) {
	ffmpeg, clip := playerFakeSound(t, 30)
	// Held, the sound stands where it is told it was heard, which the test moves by hand.
	sound, err := openPCM(ffmpeg, clip, 2*time.Second, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sound.Close)
	heard := func() time.Duration {
		at, ok := sound.heard()
		if !ok {
			t.Fatal("the sound ended")
		}
		return at
	}
	near := func(got, want time.Duration) bool { return (got - want).Abs() < time.Millisecond }
	if !near(heard(), 2*time.Second) {
		t.Fatalf("heard at %v, want where it was opened", heard())
	}
	p := &filmPlay{audio: sound}
	now := time.Now()

	p.watchSound(now)
	p.catchUpSound(now.Add(time.Second))
	if p.soundSince.IsZero() || !near(heard(), 2*time.Second) {
		t.Fatalf("a sound not heard to move yet moved to %v, still watched %v", heard(), !p.soundSince.IsZero())
	}
	p.soundFrom = 2*time.Second - 950*time.Millisecond
	p.catchUpSound(now.Add(playerSoundSettle / 2))
	if p.soundSince.IsZero() {
		t.Fatal("the sound was checked before it settled")
	}
	p.catchUpSound(now.Add(time.Second))
	if !p.soundSince.IsZero() || !near(heard(), 2*time.Second) {
		t.Fatalf("a sound 50 ms behind moved to %v", heard())
	}

	p.watchSound(now)
	p.soundFrom = 2*time.Second - 600*time.Millisecond
	p.catchUpSound(now.Add(time.Second))
	if !p.soundSince.IsZero() || !near(heard(), 2400*time.Millisecond) {
		t.Fatalf("a sound 400 ms behind is heard at %v, want 2.4s", heard())
	}

	p.watchSound(now)
	p.catchUpSound(now.Add(playerSoundGiveUp + time.Second))
	if !p.soundSince.IsZero() || !near(heard(), 2400*time.Millisecond) {
		t.Fatalf("a sound never heard to move: watched %v, at %v", !p.soundSince.IsZero(), heard())
	}
}

// TestMutedFilmOpensWithItsSound: a film opened muted gets its sound all the same, playing unheard, and a volume changed while it opened reaches that sound rather than opening it again.
func TestMutedFilmOpensWithItsSound(t *testing.T) {
	a, tt, updates := playerFakeFilm(t)
	a.player.volume, a.player.muted = 0.8, true
	tt.Frame()
	if err := tt.Click("clip"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	// Changed before the film's update comes, after its sound opened muted.
	a.SetVolume(0.4)
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	p := a.player.play
	if p.audio == nil {
		t.Fatal("a film opened muted has no sound")
	}
	proc := p.audio.decoder.proc
	if proc == nil || proc.cmd.ProcessState != nil {
		t.Fatal("the sound's ffmpeg is not running")
	}
	if math.Abs(p.volume-0.4) > 1e-9 || math.Abs(p.audio.player.Volume()-0.4) > 1e-9 {
		t.Fatalf("film at %v, sound at %v, want the 0.4 set while it opened", p.volume, p.audio.player.Volume())
	}
	a.ToggleMute()
	if p.audio.decoder.proc != proc || p.audio.player.Volume() != 0 {
		t.Fatalf("muting: sound at %v, same ffmpeg %v", p.audio.player.Volume(), p.audio.decoder.proc == proc)
	}
}

// TestResumingWatchesTheSound: playing a paused film has catchUpSound watch its sound from where it is heard, and pausing it stops that.
func TestResumingWatchesTheSound(t *testing.T) {
	a, tt, updates := playerFakeFilm(t)
	tt.Frame()
	if err := tt.Click("clip"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	p := a.player.play
	if !p.soundSince.IsZero() {
		t.Fatal("a film held on its first frame watches its sound")
	}
	p.toggle()
	if p.soundSince.IsZero() {
		t.Fatal("playing did not watch the sound")
	}
	if at, ok := p.audio.heard(); !ok || p.soundFrom != at {
		t.Fatalf("watched from %v, heard at %v", p.soundFrom, at)
	}
	p.toggle()
	if !p.soundSince.IsZero() {
		t.Fatal("a paused film's sound is still watched")
	}
}

// playerFakeFilm is an app whose library holds clip, a 30-second film the fake ffmpeg plays the picture and the sound of.
func playerFakeFilm(t *testing.T) (*app, *ui.Tester, chan func()) {
	t.Helper()
	ffmpeg, clip := playerFakeSound(t, 30)
	a, tt, updates := newTestApp(media.Tools{FFmpeg: ffmpeg})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.edit(func(s *session) { s.LibraryDir = filepath.Dir(clip) })
	a.library.Jobs = []core.Job{{ID: "clip", Source: core.FileURL(clip), Language: "简体中文", Stage: core.StageComplete, VideoPath: clip}}
	t.Cleanup(a.stopPlayback)
	return a, tt, updates
}

// TestPlayerAudioCloseEndsFFmpeg: closing the sound ends its ffmpeg, which waits on the full pipe while the sound is held, and a read after it is the end.
func TestPlayerAudioCloseEndsFFmpeg(t *testing.T) {
	ffmpeg, clip := playerFakeSound(t, 30)
	sound, err := openPCM(ffmpeg, clip, 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	proc := sound.decoder.proc
	done := make(chan struct{})
	go func() {
		sound.Close()
		sound.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("closing the sound did not return")
	}
	if proc.cmd.ProcessState == nil {
		t.Fatal("ffmpeg outlived its sound")
	}
	if n, err := sound.decoder.Read(make([]float32, 64)); n != 0 || err != io.EOF {
		t.Fatalf("a read after Close: %d %v", n, err)
	}
	if _, err := openPCM("", clip, 0, 1, 1); err == nil {
		t.Fatal("a sound opened without ffmpeg")
	}
}

// TestVolumeChangesTheSoundAsItPlays: the volume keys and the mute change how loud the open film's sound plays at once, on the same sound and the same ffmpeg, rather than starting the sound again; muted, it plays on unheard.
func TestVolumeChangesTheSoundAsItPlays(t *testing.T) {
	ffmpeg, clip := playerFakeSound(t, 30)
	a, _, _ := newTestApp(&fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	sound, err := openPCM(ffmpeg, clip, 0, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sound.Close)
	proc := sound.decoder.proc
	a.player.play = &filmPlay{audio: sound, ffmpeg: ffmpeg, path: clip, volume: 1, paused: true}
	t.Cleanup(func() { a.player.play = nil })
	want := func(volume float64) {
		t.Helper()
		p := a.player.play
		if p.audio != sound || sound.decoder.proc != proc || proc.cmd.ProcessState != nil {
			t.Fatalf("the sound started again for volume %v", volume)
		}
		if math.Abs(p.volume-volume) > 1e-9 || math.Abs(sound.player.Volume()-volume) > 1e-9 {
			t.Fatalf("film at %v, sound at %v, want %v", p.volume, sound.player.Volume(), volume)
		}
	}
	a.SetVolume(0.3)
	want(0.3)
	a.ToggleMute()
	want(0)
	a.ToggleMute()
	want(0.3)
	a.playerNudgeVolume(-1)
	want(0.2)
}

// TestPlayerAudioEndedDoesNotStartAgain: a sound that played to the end of its track stays ended when the film plays on, rather than starting again from where it was opened.
func TestPlayerAudioEndedDoesNotStartAgain(t *testing.T) {
	ffmpeg, clip := playerFakeSound(t, 1)
	sound, err := openPCM(ffmpeg, clip, 500*time.Millisecond, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sound.Close)
	sound.Start()
	deadline := time.Now().Add(5 * time.Second)
	for !sound.ended.Load() {
		if time.Now().After(deadline) {
			t.Fatal("half a second of sound did not end")
		}
		time.Sleep(20 * time.Millisecond)
	}
	proc := sound.decoder.proc
	sound.Pause()
	sound.Play()
	time.Sleep(200 * time.Millisecond)
	if sound.player.Playing() || sound.decoder.proc != proc {
		t.Fatalf("the ended sound played again: playing %v, ffmpeg %v", sound.player.Playing(), sound.decoder.proc != proc)
	}
}

// TestPlayerNamesTheSystemsFileManager checks that the player's reveal names the file manager of the system it runs on, and that the style view's font menu offers that system's fonts: macOS's own there, a sans-serif and a serif font elsewhere, saved by their stored names.
func TestPlayerNamesTheSystemsFileManager(t *testing.T) {
	for _, tc := range []struct {
		platform    breeze.Platform
		reveal      string
		fonts       []string
		pick, saved string
	}{
		{breeze.Mac, "在访达中显示", []string{"默认", "苹方", "冬青黑体"}, "冬青黑体", "冬青黑体"},
		{breeze.Windows, "在文件资源管理器中显示", []string{"默认", "黑体", "宋体"}, "宋体", "宋体"},
		{breeze.Linux, "在文件管理器中显示", []string{"默认", "黑体", "宋体"}, "宋体", "宋体"},
	} {
		t.Run(tc.platform.String(), func(t *testing.T) {
			usePlatform(t, tc.platform)
			dir := t.TempDir()
			video := filepath.Join(dir, "talk.mp4")
			srt := filepath.Join(dir, "talk.bilingual.srt")
			for path, body := range map[string]string{video: "v", srt: "1\n00:00:01,000 --> 00:00:02,000\n你好。\nHello.\n\n"} {
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			a, tt, updates := newTestApp(&fakePipeline{})
			a.edit(func(s *session) { s.LibraryDir = dir })
			a.library.Jobs = []core.Job{{ID: "job1", Source: "file://" + video, Language: "简体中文", Stage: core.StageComplete, VideoPath: video, SubtitlePath: srt}}
			tt.Frame()
			if err := tt.Click("talk"); err != nil {
				t.Fatalf("%v texts %q", err, tt.Texts())
			}
			tt.Frame()
			waitUntil(t, updates, func() bool { return a.player.subs.ready && a.player.film.info.Folder != "" })
			tt.Frame()
			wantText(t, tt, tc.reveal)
			if err := tt.Click("信息"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if n := textCount(tt, tc.reveal); n < 2 {
				t.Fatalf("%q shows %d times with the Info tab open, want the title bar's and the tab's: %q", tc.reveal, n, tt.Texts())
			}
			a.SetStyleView(true)
			tt.Frame()
			if err := tt.Click("默认"); err != nil {
				t.Fatalf("%v texts %q", err, tt.Texts())
			}
			tt.Frame()
			for _, font := range tc.fonts {
				wantText(t, tt, font)
			}
			if err := tt.Click(tc.pick); err != nil {
				t.Fatalf("%v texts %q", err, tt.Texts())
			}
			tt.Frame()
			if look := a.player.look; look.Translation.Family != tc.saved || look.Original.Family != tc.saved {
				t.Fatalf("look %+v after %s", look, tc.pick)
			}
		})
	}
}

// nowPlayingRecorder stands in for mygo.NowPlaying: it keeps what the player shows the system, in order, and sends commands as the media keys and the system's controls would.
type nowPlayingRecorder struct {
	events  []nowPlayingEvent
	command func(mygo.MediaCommand, time.Duration)
}

// nowPlayingEvent is one call the player made: "set" with info, "playback" with state and at, or "clear".
type nowPlayingEvent struct {
	kind  string
	info  mygo.NowPlayingInfo
	state mygo.PlaybackState
	at    time.Duration
}

func (r *nowPlayingRecorder) Set(info mygo.NowPlayingInfo) {
	r.events = append(r.events, nowPlayingEvent{kind: "set", info: info, state: info.State, at: info.Position})
}

func (r *nowPlayingRecorder) SetPlayback(state mygo.PlaybackState, at time.Duration) {
	r.events = append(r.events, nowPlayingEvent{kind: "playback", state: state, at: at})
}

func (r *nowPlayingRecorder) Clear() { r.events = append(r.events, nowPlayingEvent{kind: "clear"}) }

func (r *nowPlayingRecorder) OnCommand(fn func(mygo.MediaCommand, time.Duration)) func() {
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

// TestNowPlayingFollowsTheOpenFilm checks that opening a film shows it where the system shows what plays, with its library name and length, held; that Space, the media keys and the system's scrubber play, pause and move it and the system learns each; that a seek keeps the film shown while its picture opens again; and that closing the film takes it off.
func TestNowPlayingFollowsTheOpenFilm(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=black:s=160x90:d=5:r=10", "-c:v", "mpeg4", "-pix_fmt", "yuv420p", video).CombinedOutput(); err != nil {
		skipWithoutFFmpeg(t, "ffmpeg could not make a clip: %v %s", err, out)
	}
	a, tt, updates := newTestApp(media.Tools{FFmpeg: ffmpeg})
	rec := &nowPlayingRecorder{}
	a.playerNowPlayingConnect(rec)
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{ID: "clip", Title: "Spring Clip", Source: "file://" + video, Language: "简体中文", Stage: core.StageComplete, VideoPath: video}}
	t.Cleanup(a.stopPlayback)
	tt.Frame()
	if err := tt.Click("Spring Clip"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	tt.Frame()
	shown := rec.last(t)
	if shown.kind != "set" || shown.info.Title != "Spring Clip" || shown.info.Duration <= 4*time.Second || shown.info.State != mygo.PlaybackPaused || shown.info.Position >= time.Second || shown.info.Commands != mygo.MediaPlay|mygo.MediaPause|mygo.MediaSeek {
		t.Fatalf("opening showed %+v", shown)
	}

	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if e := rec.last(t); e.kind != "playback" || e.state != mygo.PlaybackPlaying {
		t.Fatalf("Space told the system %+v", e)
	}

	press := func(cmd mygo.MediaCommand, at time.Duration) {
		t.Helper()
		if rec.command == nil {
			t.Fatal("the player listens to no media key")
		}
		before := len(rec.events)
		rec.command(cmd, at)
		waitUntil(t, updates, func() bool { return len(rec.events) > before })
		tt.Frame()
	}
	press(mygo.MediaPause, time.Second)
	if a.player.play.Playing() {
		t.Fatal("the pause key did not pause the film")
	}
	if e := rec.last(t); e.kind != "playback" || e.state != mygo.PlaybackPaused {
		t.Fatalf("pausing told the system %+v", e)
	}
	press(mygo.MediaPause, time.Second)
	if a.player.play.Playing() {
		t.Fatal("a second pause played the film")
	}
	if e := rec.last(t); e.kind != "playback" || e.state != mygo.PlaybackPaused {
		t.Fatalf("a second pause told the system %+v", e)
	}
	press(mygo.MediaPlay, time.Second)
	if !a.player.play.Playing() {
		t.Fatal("the play key did not play the film")
	}
	if e := rec.last(t); e.kind != "playback" || e.state != mygo.PlaybackPlaying {
		t.Fatalf("playing told the system %+v", e)
	}

	sets := rec.count("set")
	press(mygo.MediaSeek, 3*time.Second)
	if e := rec.last(t); e.kind != "playback" || e.state != mygo.PlaybackPlaying || e.at != 3*time.Second {
		t.Fatalf("the scrubber told the system %+v", e)
	}
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	tt.Frame()
	if a.player.play.playhead() < 3*time.Second || !a.player.play.Playing() {
		t.Fatalf("the seek opened at %s playing %v", a.player.play.playhead(), a.player.play.Playing())
	}
	if rec.count("clear") != 0 || rec.count("set") != sets {
		t.Fatalf("the seek took the film off or showed it again: %+v", rec.events)
	}
	if e := rec.last(t); e.kind != "playback" || e.state != mygo.PlaybackPlaying || e.at < 3*time.Second {
		t.Fatalf("the reopened picture told the system %+v", e)
	}

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.player.play != nil || rec.last(t).kind != "clear" {
		t.Fatalf("closing the film left the system %+v", rec.last(t))
	}
}

// TestNowPlayingShowsThePosterAndTheEnd checks, without ffmpeg, that a film shows with its poster once read, that its end shows paused at its length so the play key starts it over, that a media key with no film open does nothing, and that another video opening takes the film off.
func TestNowPlayingShowsThePosterAndTheEnd(t *testing.T) {
	a := openStill(t, filepath.Join(t.TempDir(), "library.json"), "clip", "/v/clip.mp4", 3*time.Second)
	a.player.play.player.Play()
	updates := make(chan func(), 4)
	a.update = func(fn func()) { updates <- fn }
	a.lib.posterDir = t.TempDir()
	poster := []byte("poster of the clip")
	if err := os.WriteFile(posterDest(a.lib.posterDir, "/v/clip.mp4"), poster, 0o644); err != nil {
		t.Fatal(err)
	}
	rec := &nowPlayingRecorder{}
	a.playerNowPlayingConnect(rec)
	p := a.player.play
	a.player.play = nil
	a.playReplace(p)
	shown := rec.last(t)
	if shown.kind != "set" || shown.info.Title != "clip" || shown.info.Artwork != nil || shown.info.State != mygo.PlaybackPlaying || shown.info.Position != 3*time.Second || shown.info.Duration != 10*time.Second {
		t.Fatalf("opening showed %+v", shown)
	}
	waitUntil(t, updates, func() bool { return rec.count("set") == 2 })
	if e := rec.last(t); string(e.info.Artwork) != string(poster) || e.info.Title != "clip" || e.info.State != mygo.PlaybackPlaying {
		t.Fatalf("the poster showed as %+v", e)
	}

	p.finish()
	if e := rec.last(t); e.kind != "playback" || e.state != mygo.PlaybackPaused || e.at != 10*time.Second {
		t.Fatalf("the end told the system %+v", e)
	}
	a.playerMediaCommand(mygo.MediaPlay, 10*time.Second)
	if p.finished || !p.Playing() {
		t.Fatal("the play key did not start the film over")
	}
	if e := rec.last(t); e.kind != "playback" || e.state != mygo.PlaybackPlaying || e.at != 0 {
		t.Fatalf("starting over told the system %+v", e)
	}

	a.startPlayback("/v/other.mp4")
	if e := rec.last(t); e.kind != "clear" {
		t.Fatalf("another video opening left the system %+v", e)
	}
	before := len(rec.events)
	a.playerMediaCommand(mygo.MediaPause, 0)
	a.playerMediaCommand(mygo.MediaSeek, time.Second)
	if len(rec.events) != before || a.player.play != nil {
		t.Fatalf("a media key with no film open did %+v", rec.events[before:])
	}
}

// showClipNowPlaying connects a recorder in place of mygo.NowPlaying and shows the clip open in a there, as opening it would have.
func showClipNowPlaying(a *app) *nowPlayingRecorder {
	rec := &nowPlayingRecorder{}
	a.playerNowPlayingConnect(rec)
	p := a.player.play
	a.player.play = nil
	a.playReplace(p)
	return rec
}

// TestMediaKeysBehindTheWindowKeepPictureAndSoundLevel checks that pausing with a media key while the app is in the background, where the picture stood still while the sound played on, tells the system where the sound stopped and opens the picture there, held; that play from there follows the sound again; and that coming back after more time out of sight puts the picture where the sound got to.
func TestMediaKeysBehindTheWindowKeepPictureAndSoundLevel(t *testing.T) {
	a, _, updates := openClip(t, 12)
	rec := showClipNowPlaying(a)
	p := a.player.play
	p.toggle()
	a.playerDidResignActive()
	// Two seconds out of sight: the picture stood still while the sound played on.
	p.backgroundAt = p.backgroundAt.Add(-2 * time.Second)
	a.playerMediaCommand(mygo.MediaPause, 0)
	if p.Playing() || !p.paused || !p.backgroundAt.IsZero() || p.background != nil {
		t.Fatalf("paused in the background: playing %v paused %v, sound clock at %v, follower %v", p.Playing(), p.paused, p.backgroundAt, p.background != nil)
	}
	stopped := rec.last(t)
	if stopped.kind != "playback" || stopped.state != mygo.PlaybackPaused || stopped.at < 2*time.Second {
		t.Fatalf("the pause told the system %+v, with the sound past 2s", stopped)
	}
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	held := a.player.play
	if held == p || held.Playing() || !held.paused || !held.played {
		t.Fatalf("the picture opened again playing %v paused %v played %v", held.Playing(), held.paused, held.played)
	}
	// The playhead of a held picture counts the frame it holds.
	if at := held.playhead(); at < stopped.at-100*time.Millisecond || at > stopped.at+200*time.Millisecond {
		t.Fatalf("the picture opened again at %v, the sound stopped at %v", at, stopped.at)
	}
	if e := rec.last(t); e.kind != "playback" || e.state != mygo.PlaybackPaused || rec.count("clear") != 0 {
		t.Fatalf("opening the picture again told the system %+v", rec.events)
	}

	a.playerMediaCommand(mygo.MediaPlay, 0)
	if !held.Playing() || held.backgroundAt.IsZero() || held.background == nil {
		t.Fatalf("played in the background: playing %v, sound clock at %v, follower %v", held.Playing(), held.backgroundAt, held.background != nil)
	}
	if d := held.backgroundHead - held.playhead(); d < -100*time.Millisecond || d > 100*time.Millisecond {
		t.Fatalf("the sound clock starts at %v, the picture at %v", held.backgroundHead, held.playhead())
	}
	if e := rec.last(t); e.kind != "playback" || e.state != mygo.PlaybackPlaying {
		t.Fatalf("playing told the system %+v", e)
	}

	// Two more seconds out of sight, then back.
	held.backgroundAt = held.backgroundAt.Add(-2 * time.Second)
	want := held.soundAt(time.Now())
	a.playerDidBecomeActive()
	if a.player.inactive || a.player.opening < want || a.player.opening > want+500*time.Millisecond {
		t.Fatalf("back, the picture opens at %v with the sound at %v", a.player.opening, want)
	}
	waitUntil(t, updates, func() bool { return a.player.play != held })
	if next := a.player.play; !next.Playing() || next.playhead() < want {
		t.Fatalf("back, the picture plays %v at %v, the sound at %v", next.Playing(), next.playhead(), want)
	}
}

// TestSeekBehindTheWindowFollowsTheSound checks that a seek from the system's scrubber while the app is in the background follows the sound from where it opens, so coming back puts the picture where the sound got to rather than where the seek opened it.
func TestSeekBehindTheWindowFollowsTheSound(t *testing.T) {
	a, _, updates := openClip(t, 12)
	showClipNowPlaying(a)
	a.player.play.toggle()
	a.playerDidResignActive()
	a.playerMediaCommand(mygo.MediaSeek, 6*time.Second)
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	next := a.player.play
	if !next.Playing() || next.backgroundAt.IsZero() || next.backgroundHead < 6*time.Second || next.background == nil {
		t.Fatalf("seeked in the background: playing %v, sound clock at %v from %v, follower %v", next.Playing(), next.backgroundAt, next.backgroundHead, next.background != nil)
	}
	// Two seconds out of sight, then back.
	next.backgroundAt = next.backgroundAt.Add(-2 * time.Second)
	a.playerDidBecomeActive()
	if a.player.opening < 8*time.Second {
		t.Fatalf("back, the picture opens at %v, the sound past 8s", a.player.opening)
	}
	waitUntil(t, updates, func() bool { return a.player.play != next })
	if back := a.player.play; !back.Playing() || back.playhead() < 8*time.Second {
		t.Fatalf("back, the picture plays %v at %v", back.Playing(), back.playhead())
	}
}

// TestPauseKeyWhileASeekOpens checks that a pause key pressed while a seek opens the picture again, when there is no picture to pause yet, is not lost: the system learns at once that the film is paused there, and the picture opens held.
func TestPauseKeyWhileASeekOpens(t *testing.T) {
	a, _, updates := openClip(t, 12)
	rec := showClipNowPlaying(a)
	a.player.play.toggle()
	a.playerMediaCommand(mygo.MediaSeek, 4*time.Second)
	if a.player.play != nil {
		t.Fatal("the seek kept the old picture")
	}
	a.playerMediaCommand(mygo.MediaPause, 0)
	if e := rec.last(t); e.kind != "playback" || e.state != mygo.PlaybackPaused || e.at != 4*time.Second {
		t.Fatalf("the pause while opening told the system %+v", e)
	}
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	held := a.player.play
	// The playhead of a held picture counts the frame it holds.
	if at := held.playhead(); held.Playing() || !held.paused || at < 4*time.Second || at > 4*time.Second+200*time.Millisecond {
		t.Fatalf("the seek opened playing %v paused %v at %v", held.Playing(), held.paused, held.playhead())
	}
	if e := rec.last(t); e.kind != "playback" || e.state != mygo.PlaybackPaused {
		t.Fatalf("the held picture told the system %+v", e)
	}
	a.playerMediaCommand(mygo.MediaPlay, 0)
	if !held.Playing() {
		t.Fatal("the play key did not play the held picture")
	}
}

// TestFramesShowForTheirLengthOverTheRate: at 2× each frame of the clip's 10 a second shows for 50ms, the held one too, and at 0.5× for 200ms, while the decoder's position counts each frame's own 100ms, so the playhead stays in the film's time whatever the speed.
func TestFramesShowForTheirLengthOverTheRate(t *testing.T) {
	a, _, _ := openClip(t, 12)
	frames, err := media.OpenFramesAt(a.ffmpegPath(), a.player.play.path, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { frames.Close() })
	img, dur, err := frames.Next()
	if err != nil || dur != 100*time.Millisecond {
		t.Fatalf("first frame %v, %v: want the clip's 10 frames a second", dur, err)
	}
	dec := &frameDecoder{frames: frames, held: &ui.VideoFrame{Image: img, Duration: dur}, rate: 2}
	next := func(shows, at time.Duration) {
		t.Helper()
		f, err := dec.Next()
		if err != nil || f.Duration != shows || dec.position() != at {
			t.Fatalf("frame shows for %v up to %v (%v), want %v up to %v", f.Duration, dec.position(), err, shows, at)
		}
	}
	next(50*time.Millisecond, 100*time.Millisecond)
	next(50*time.Millisecond, 200*time.Millisecond)
	dec.setRate(0.5)
	next(200*time.Millisecond, 300*time.Millisecond)
	dec.setRate(1)
	next(100*time.Millisecond, 400*time.Millisecond)
}

// TestPlayheadMovesInFilmTimeAtTheRate plays the clip for a second of the window's clock at 2× and at 0.5×: the playhead gets two seconds and half a second into the film, give or take the frames decoded ahead.
func TestPlayheadMovesInFilmTimeAtTheRate(t *testing.T) {
	for _, rate := range []float64{2, 0.5} {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			a, tt, _ := openClip(t, 12)
			a.SetRate(rate)
			a.player.play.toggle()
			tt.Frame()
			for range 20 {
				tt.Advance(50 * time.Millisecond)
				tt.Frame()
				time.Sleep(20 * time.Millisecond)
			}
			// Frames the decoder had not sent yet as they came due are taken without the clock moving on.
			for range 5 {
				tt.Frame()
				time.Sleep(20 * time.Millisecond)
			}
			want := time.Duration(rate * float64(time.Second))
			if head := a.player.play.playhead(); head < want-100*time.Millisecond || head > want+400*time.Millisecond {
				t.Fatalf("after a second at %v×, playhead %v, want about %v", rate, head, want)
			}
		})
	}
}

// TestSpeedKeysStepThroughTheRates presses ⇧. and ⇧, on an open film: each steps to the next speed of film.Rates, stays at the fastest and the slowest, says so on the picture and on the transport's speed button, and the open film plays at it.
func TestSpeedKeysStepThroughTheRates(t *testing.T) {
	a, tt, _ := openClip(t, 2)
	a.player.film.moved = tt.Now()
	key := func(k ui.Key, rate float64, osd string) {
		t.Helper()
		tt.Key(ui.Shift, k)
		tt.Frame()
		if a.player.rate != rate || a.player.play.speed() != rate || !tt.HasText(osd) || !tt.HasText(film.RateText(rate)) {
			t.Fatalf("after ⇧%v: rate %v, film at %v, want %v; texts %q", k, a.player.rate, a.player.play.speed(), rate, tt.Texts())
		}
	}
	key(ui.KeyPeriod, 1.25, "速度 1.25×")
	key(ui.KeyPeriod, 1.5, "速度 1.5×")
	key(ui.KeyPeriod, 2, "速度 2×")
	key(ui.KeyPeriod, 2, "速度 2×")
	key(ui.KeyComma, 1.5, "速度 1.5×")
	for range 4 {
		tt.Key(ui.Shift, ui.KeyComma)
	}
	key(ui.KeyComma, 0.5, "速度 0.5×")
}

// TestSpeedMenuSetsTheRate opens the transport's speed menu, which lists every speed with 1× noted as normal and checked, and picks 1.5×: the film plays at it and the menu closes.
func TestSpeedMenuSetsTheRate(t *testing.T) {
	a, tt, _ := openClip(t, 2)
	a.player.film.moved = tt.Now()
	tt.Frame()
	if err := tt.Click("速度：1×"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if !a.player.film.speedMenu || !tt.HasText("播放速度") || !tt.HasText("正常") {
		t.Fatalf("speed menu open %v; texts %q", a.player.film.speedMenu, tt.Texts())
	}
	for _, rate := range film.Rates {
		if !tt.HasText(film.RateText(rate)) {
			t.Fatalf("speed menu misses %s: %q", film.RateText(rate), tt.Texts())
		}
	}
	if err := tt.Click("1.5×"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if a.player.rate != 1.5 || a.player.play.speed() != 1.5 || a.player.film.speedMenu || !tt.HasText("速度：1.5×") {
		t.Fatalf("rate %v, film at %v, menu open %v; texts %q", a.player.rate, a.player.play.speed(), a.player.film.speedMenu, tt.Texts())
	}
}

// TestSpeedSettingsRoundTrip checks that the speed is kept in the settings and that a saved speed that is not one of film.Rates comes back as the nearest of them.
func TestSpeedSettingsRoundTrip(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.SetRate(1.5)
	b, _, _ := newTestApp(&fakePipeline{})
	b.restorePlayer(loadSavedSettings(a.settingsPath))
	if b.player.rate != 1.5 {
		t.Fatalf("restored rate %v, want 1.5", b.player.rate)
	}
	for _, tc := range []struct {
		saved string
		want  float64
	}{{"3", 2}, {"0.1", 0.5}, {"0.6", 0.5}, {"0.9", 1}, {"1.3", 1.25}, {"-1", 0.5}} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(`{"rate": `+tc.saved+`}`), 0o600); err != nil {
			t.Fatal(err)
		}
		saved := loadSavedSettings(path)
		if saved.Rate == nil || *saved.Rate != tc.want {
			t.Fatalf("saved rate %s read as %v, want %v", tc.saved, saved.Rate, tc.want)
		}
		c, _, _ := newTestApp(&fakePipeline{})
		c.restorePlayer(saved)
		if c.player.rate != tc.want {
			t.Fatalf("saved rate %s restored as %v, want %v", tc.saved, c.player.rate, tc.want)
		}
	}
	d, _, _ := newTestApp(&fakePipeline{})
	d.restorePlayer(savedSettings{})
	if d.player.rate != 1 {
		t.Fatalf("default rate %v", d.player.rate)
	}
}

// TestSpeedWaitsForThePictureToComeBack: the sound of a film in the background plays on at the speed it started at, so the clock that finds where it is counts at that speed, and a speed changed meanwhile reaches the film only once the app is back and its picture level with the sound.
func TestSpeedWaitsForThePictureToComeBack(t *testing.T) {
	now := time.Now()
	fast := &filmPlay{backgroundAt: now.Add(-3 * time.Minute), backgroundHead: 10 * time.Second, rate: 2}
	if got := fast.soundAt(now); got != 6*time.Minute+10*time.Second {
		t.Fatalf("sound at %v after 3 minutes at 2×, want 6m10s", got)
	}

	a, _, _ := newTestApp(&fakePipeline{})
	p := stillPlay(t, "/v/clip.mp4", 3*time.Second, true)
	a.player.play = p
	a.playerDidResignActive()
	a.SetRate(2)
	a.areasAfterFrame()
	if p.speed() != 1 {
		t.Fatalf("the film in the background took up %v× before its picture came back", p.speed())
	}
	// The window stayed in sight: the picture kept pace and nothing opens again, so the film takes the speed up after the next frame.
	p.frames.shown = p.backgroundSound()()
	a.playerDidBecomeActive()
	a.areasAfterFrame()
	if p.speed() != 2 || p.frames.rate != 2 {
		t.Fatalf("back from the background, the film plays at %v× (picture %v×), want 2×", p.speed(), p.frames.rate)
	}
}

// TestPlayerAudioArgsKeepThePitchAtTheRate: ffmpeg decodes the sound of a film at another speed through atempo, which keeps the pitch, and at 1 (or none, 0) without a filter.
func TestPlayerAudioArgsKeepThePitchAtTheRate(t *testing.T) {
	for _, tc := range []struct {
		rate float64
		want string
	}{{1.5, "atempo=1.5"}, {0.75, "atempo=0.75"}, {2, "atempo=2"}, {0.5, "atempo=0.5"}, {1, ""}, {0, ""}} {
		args := playerAudioArgs("/tmp/video.mp4", 3*time.Second, tc.rate)
		if got := fakeArg(args, "-af"); got != tc.want || fakeArg(args, "-ss") != "3.000" || fakeArg(args, "-f") != "f32le" {
			t.Fatalf("rate %v: -af %q, want %q; args %v", tc.rate, got, tc.want, args)
		}
		if tc.want != "" && slices.Index(args, "-af") < slices.Index(args, "-i") {
			t.Fatalf("rate %v: the filter comes before the input: %v", tc.rate, args)
		}
	}
}

// TestPlayerAudioCountsTheFilmAtTheRate: the frames of a sound playing at 2× count the time they take to play, each second of them two of the film, so a move to a frame starts ffmpeg twice as far into the film, and where the sound is heard is in the film's own time.
func TestPlayerAudioCountsTheFilmAtTheRate(t *testing.T) {
	ffmpeg, clip := playerFakeSound(t, 12)
	d := &playerAudioDecoder{ffmpeg: ffmpeg, path: clip, from: time.Second, rate: 2}
	if err := d.start(0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if args := d.proc.cmd.Args; fakeArg(args, "-ss") != "1.000" || fakeArg(args, "-af") != "atempo=2" {
		t.Fatalf("ffmpeg %v", args)
	}
	if err := d.SetPosition(10 * 48000); err != nil {
		t.Fatal(err)
	}
	if args := d.proc.cmd.Args; fakeArg(args, "-ss") != "21.000" || fakeArg(args, "-af") != "atempo=2" {
		t.Fatalf("ffmpeg after a move 10 s of sound on: %v", args)
	}
	if got := d.filmAt(1500 * time.Millisecond); got != 4*time.Second {
		t.Fatalf("1.5 s of sound from 1 s is at %v of the film, want 4s", got)
	}
}

// TestSpeedStartsTheSoundAgainAtIt: a new speed starts a playing film's sound again at it, where the film is; a held film drops the sound it had ready, and play starts it at the new speed.
func TestSpeedStartsTheSoundAgainAtIt(t *testing.T) {
	a, tt, updates := playerFakeFilm(t)
	tt.Frame()
	if err := tt.Click("clip"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	waitUntil(t, updates, func() bool { return a.player.play != nil })
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if a.player.play.audio == nil {
		t.Fatal("the playing film has no sound")
	}
	tempo := func() string { return fakeArg(a.player.play.audio.decoder.proc.cmd.Args, "-af") }
	a.SetRate(1.5)
	if audio := a.player.play.audio; audio == nil || audio.decoder.rate != 1.5 || tempo() != "atempo=1.5" {
		t.Fatalf("the playing film's sound did not start again at 1.5×: %+v", audio)
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	a.SetRate(0.75)
	if a.player.play.audio != nil {
		t.Fatal("the held film kept its sound at the old speed")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if audio := a.player.play.audio; audio == nil || audio.decoder.rate != 0.75 || tempo() != "atempo=0.75" {
		t.Fatalf("play did not start the sound at 0.75×: %+v", audio)
	}
}
