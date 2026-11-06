package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"lingocut/apps/lingocut/film"
	"lingocut/packages/core"
	"lingocut/packages/media"

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
	if err := tt.Click("返回"); err != nil {
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
	waitUpdate(t, updates)
	tt.Frame()
	if !tt.HasText("字幕文件不在了") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// TestOpenFilmClearsTheTitleBar checks that an open film hides the sidebar and that its top row still clears the traffic lights, moving down by the title bar's height.
func TestOpenFilmClearsTheTitleBar(t *testing.T) {
	open := func(a *app) {
		a.library.Jobs = []core.Job{{ID: "j", Source: "https://example.com/v", Stage: core.StageQueued}}
		a.openKey = "j"
	}
	shifts, barred := titleBarShifts(t, ui.TitleBar{Height: 28, Left: 80}, open, "返回")
	if abs32(shifts[0]-28) > 0.5 {
		t.Fatalf("返回 moved %v under a 28-high title bar", shifts[0])
	}
	if barred.HasText("全部影片") {
		t.Fatalf("sidebar shown over an open film: %q", barred.Texts())
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
