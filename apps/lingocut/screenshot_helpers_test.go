package main

import (
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// The screenshot tests render the main scenes to PNG files in the directory named by LINGOCUT_SCREENSHOTS, so a visual change can be compared before and after. They are skipped when the variable is unset. Each area renders its own scenes in its own screenshot_<area>_test.go (TestScreenshots<Area>), from the library this file makes; `go test -run TestScreenshots` renders them all. The window is light, as main sets it.

// screenshotFixture is the library every scene starts from: a finished film, Colibri, with a bilingual subtitle, and a link whose translation failed. The scenes do not see this machine: home is a new empty home folder, the library is ~/Movies/影视库.lingocut in it, and the pipeline runs the fake ffmpeg, yt-dlp and whisper-cli in ~/bin (fake_tools_test.go), so every path a scene shows starts with ~ and the versions and the picture are the same everywhere. A scene that needs another folder makes it under home.
type screenshotFixture struct {
	out      string
	home     string
	lib      string
	pipeline media.Pipeline
	jobs     []core.Job
}

// newScreenshotFixture makes the library in a fake home folder (screenshotHome), or skips t when LINGOCUT_SCREENSHOTS is unset. Colibri's video is a placeholder the fake ffmpeg plays as a two-second clip (writeFakeClip).
func newScreenshotFixture(t *testing.T) *screenshotFixture {
	t.Helper()
	out := os.Getenv("LINGOCUT_SCREENSHOTS")
	if out == "" {
		t.Skip("set LINGOCUT_SCREENSHOTS=<dir> to render the screenshots")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	home := screenshotHome(t)
	lib := filepath.Join(home, "Movies", "影视库.lingocut")
	clip := filepath.Join(lib, "Colibri", "video.mp4")
	writeFakeClip(t, clip, 2, "9CC9E0", "4F7F6A")
	srt := filepath.Join(lib, "Colibri", "video.bilingual.srt")
	cues := "1\n00:00:00,000 --> 00:00:01,000\n蜂鸟每秒振翅五十次。\nHummingbirds beat their wings fifty times a second.\n\n2\n00:00:01,000 --> 00:00:02,000\n它们能向后飞。\nThey can fly backwards.\n\n"
	if err := os.WriteFile(srt, []byte(cues), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(home, "bin")
	pipeline := media.Tools{
		OutputDir:  lib,
		YTDLP:      fakeTool(t, bin, "yt-dlp"),
		FFmpeg:     fakeTool(t, bin, "ffmpeg"),
		Whisper:    fakeTool(t, bin, "whisper-cli"),
		Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel},
	}
	jobs := []core.Job{
		{ID: "colibri", Source: "file://" + clip, Language: "简体中文", Stage: core.StageComplete, VideoPath: clip, SubtitlePath: srt, LibraryDir: lib},
		{ID: "talk", Source: "https://www.youtube.com/watch?v=TRWgBxVAb1M", Language: "简体中文", Stage: core.StageFailed, Err: errors.New("translate: translation API returned 401"), LibraryDir: lib},
	}
	return &screenshotFixture{out: out, home: home, lib: lib, pipeline: pipeline, jobs: jobs}
}

// screenshotHome cuts t off from this machine until it ends and returns the fake home folder: HOME (USERPROFILE on Windows) is a new empty folder, so the config, cache and speech model folders are empty ones in it and the window shows paths in it as ~; the XDG folders follow HOME; PATH is only ~/bin, where the fixture puts the fake tools, so nothing finds a tool installed here; and every LINGOCUT_ and EVERYAPI_ variable is blank, so no credentials, endpoints, models or library folder of this machine reach a scene.
func screenshotHome(t *testing.T) string {
	t.Helper()
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "LINGOCUT_") || strings.HasPrefix(name, "EVERYAPI_") {
			t.Setenv(name, "")
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		t.Setenv(name, "")
	}
	t.Setenv("PATH", filepath.Join(home, "bin"))
	return home
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
