package main

import (
	"errors"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// TestScreenshots renders the main scenes (library, open film, settings > 翻译, account, the shortcut cheat sheet) to PNG files in the directory named by LINGOCUT_SCREENSHOTS, so a visual change can be compared before and after. The window is light, as main sets it. It is skipped when the variable is unset.
func TestScreenshots(t *testing.T) {
	out := os.Getenv("LINGOCUT_SCREENSHOTS")
	if out == "" {
		t.Skip("set LINGOCUT_SCREENSHOTS=<dir> to render the screenshots")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINGOCUT_GOOGLE_CLIENT_ID", "")
	t.Setenv("LINGOCUT_GOOGLE_CLIENT_SECRET", "")
	lib := filepath.Join(t.TempDir(), "影视库.lingocut")
	if err := os.Mkdir(lib, 0o755); err != nil {
		t.Fatal(err)
	}

	var pipeline media.Pipeline = &fakePipeline{}
	clip := filepath.Join(lib, "Colibri", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(clip), 0o755); err != nil {
		t.Fatal(err)
	}
	if ffmpeg, err := exec.LookPath("ffmpeg"); err == nil {
		if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=640x360:d=2:r=10", "-c:v", "mpeg4", "-pix_fmt", "yuv420p", clip).CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg could not make a clip: %v %s", err, out)
		}
		pipeline = media.Tools{FFmpeg: ffmpeg}
	} else if err := os.WriteFile(clip, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	srt := filepath.Join(lib, "Colibri", "video.bilingual.srt")
	cues := "1\n00:00:00,000 --> 00:00:01,000\n蜂鸟每秒振翅五十次。\nHummingbirds beat their wings fifty times a second.\n\n2\n00:00:01,000 --> 00:00:02,000\n它们能向后飞。\nThey can fly backwards.\n\n"
	if err := os.WriteFile(srt, []byte(cues), 0o644); err != nil {
		t.Fatal(err)
	}
	jobs := []core.Job{
		{ID: "colibri", Source: "file://" + clip, Language: "简体中文", Stage: core.StageComplete, VideoPath: clip, SubtitlePath: srt, LibraryDir: lib},
		{ID: "talk", Source: "https://www.youtube.com/watch?v=TRWgBxVAb1M", Language: "简体中文", Stage: core.StageFailed, Err: errors.New("translate: translation API returned 401"), LibraryDir: lib},
	}

	shoot := func(name string, steps func(a *app, tt *ui.Tester, updates chan func())) {
		t.Helper()
		updates := make(chan func(), 64)
		a := newApp(pipeline, func(fn func()) { updates <- fn })
		a.edit(func(s *session) { s.LibraryDir = lib })
		a.library.Jobs = append([]core.Job(nil), jobs...)
		a.refreshLibrary()
		t.Cleanup(a.stopPlayback)
		tt := ui.NewTester(a.view, 1100, 720)
		tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 80})
		tt.Frame()
		steps(a, tt, updates)
		tt.Frame()
		file, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	click := func(tt *ui.Tester, label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("%v; texts %q", err, tt.Texts())
		}
		tt.Frame()
	}

	shoot("library", func(*app, *ui.Tester, chan func()) {})
	shoot("film", func(a *app, tt *ui.Tester, updates chan func()) {
		click(tt, "Colibri")
		_, tools := pipeline.(media.Tools)
		deadline := time.After(8 * time.Second)
		for !tt.HasText("蜂鸟每秒振翅五十次。") || (tools && !tt.HasText("播放")) {
			select {
			case fn := <-updates:
				fn()
			case <-deadline:
				t.Fatalf("film did not load: %q err %q", tt.Texts(), a.player.playErr)
			}
			tt.Frame()
		}
	})
	shoot("settings", func(_ *app, tt *ui.Tester, _ chan func()) { click(tt, "翻译") })
	shoot("account", func(_ *app, tt *ui.Tester, _ chan func()) { click(tt, "账号") })
	shoot("cheatsheet", func(_ *app, tt *ui.Tester, _ chan func()) {
		tt.Key(ui.Cmd, ui.KeySlash)
		tt.Frame()
	})
}
