package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
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
	waitUntil(t, updates, func() bool { return a.player.subs.ready })
	tt.Frame()
	for _, text := range []string{"返回", "字幕 100%", "你好。", "Hello.", "再见。", "0:01", "1:02:03", "全部", "在访达中显示"} {
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
		ID: "clip", Source: "file://" + video, Stage: core.StageComplete, VideoPath: video,
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
		ID: "clip", Source: "file://" + video, Stage: core.StageComplete, VideoPath: video,
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

// TestPlaybackHandsTheSoundToTheReopenedPicture checks that the picture opened again on coming back plays on with the sound that played in the background, rather than starting the sound again.
func TestPlaybackHandsTheSoundToTheReopenedPicture(t *testing.T) {
	a, _, updates := openClip(t, 12)
	p := a.player.play
	p.toggle()
	sound := p.audio
	if !p.Playing() || sound == nil && runtime.GOOS == "darwin" {
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
		ID: "clip", Source: "file://" + video, Stage: core.StageComplete, VideoPath: video,
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
		ID: "job1", Source: "file://" + video, Language: "简体中文", Stage: core.StageComplete,
		VideoPath: video, SubtitlePath: srt,
	}}
	tt.Frame()
	if err := tt.Click("talk"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	waitUntil(t, updates, func() bool { return a.player.subs.ready && a.player.film.info.Folder != "" })
	tt.Frame()
	for _, want := range []string{"100% translated", "All", "Click a line to jump to it", "Search Lines", "Show in Finder", "Back to Library", "Subtitles", "Clips", "Info", "Style"} {
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
	// The font menu lists the families in English and the look keeps the saved name of the one chosen.
	if err := tt.Click("Default"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("PingFang SC"); err != nil {
		t.Fatalf("%v texts %q", err, tt.Texts())
	}
	tt.Frame()
	if look := a.player.look; look.Translation.Family != "苹方" || look.Original.Family != "苹方" {
		t.Fatalf("look %+v after PingFang SC", look)
	}
	for _, chinese := range []string{"字幕", "简体中文", "在访达中显示", "恢复默认样式", "苹方"} {
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
		ID: "job1", Source: "file://" + video, Language: "简体中文", Stage: core.StageComplete,
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

// TestQuittingStopsTheOpenFilm: when the app quits with a film playing, the film stops, so its sound, which plays in a process of its own, ends with the app, and where it stopped is kept for next time.
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

// TestVolumeKeys checks ↑, ↓ and M on an open film: ↑ and ↓ move the volume a tenth within 0 and 100%, M mutes and gives the volume back, and each says so on the picture. Once the keys rest, the open film plays at the new volume and the settings keep it.
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
	waitUntil(t, updates, func() bool { return math.Abs(a.player.play.volume-0.9) < 1e-9 })
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
