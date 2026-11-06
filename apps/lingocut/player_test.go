package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"lingocut/apps/lingocut/film"
	"lingocut/apps/lingocut/films"
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
	waitUntil(t, updates, func() bool { return a.player.subs.ready })
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
	chapters := film.Chapters(cues, 0)
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
	if before.X >= 80 || after.X < 80 || abs32(shifts[0]) > 0.5 {
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
