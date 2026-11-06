package main

import (
	"errors"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// The screenshot tests render the main scenes to PNG files in the directory named by LINGOCUT_SCREENSHOTS, so a visual change can be compared before and after. They are skipped when the variable is unset. Each area renders its own scenes in its own screenshot_<area>_test.go (TestScreenshots<Area>), from the library this file makes; `go test -run TestScreenshots` renders them all. The window is light, as main sets it.

// screenshotFixture is the library every scene starts from: a finished film, Colibri, with a bilingual subtitle, and a link whose translation failed.
type screenshotFixture struct {
	out      string
	lib      string
	pipeline media.Pipeline
	jobs     []core.Job
}

// newScreenshotFixture makes the library in a temporary folder, or skips t when LINGOCUT_SCREENSHOTS is unset. With ffmpeg on the PATH, Colibri is a real two-second clip and the pipeline can play it.
func newScreenshotFixture(t *testing.T) *screenshotFixture {
	t.Helper()
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
	return &screenshotFixture{out: out, lib: lib, pipeline: pipeline, jobs: jobs}
}

// shoot opens the window on the library at 1100×720, runs steps and saves what it shows as name.png.
func (f *screenshotFixture) shoot(t *testing.T, name string, steps func(a *app, tt *ui.Tester, updates chan func())) {
	t.Helper()
	updates := make(chan func(), 64)
	a := newApp(f.pipeline, func(fn func()) { updates <- fn })
	a.edit(func(s *session) { s.LibraryDir = f.lib })
	a.library.Jobs = append([]core.Job(nil), f.jobs...)
	a.refreshLibrary()
	t.Cleanup(a.stopPlayback)
	tt := ui.NewTester(a.view, 1100, 720)
	tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 80})
	tt.Frame()
	steps(a, tt, updates)
	tt.Frame()
	file, err := os.Create(filepath.Join(f.out, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

// screenshotClick clicks the control labeled label and draws the next frame.
func screenshotClick(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
}
