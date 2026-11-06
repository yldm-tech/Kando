package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsSettings renders the settings window, one shot per category, at the prototype's 880 by 640 with the traffic lights in their 50-point band (screenshot_helpers_test.go). The window has a view of its own, so it is drawn here rather than through the fixture's shoot, which draws the main window.
func TestScreenshotsSettings(t *testing.T) {
	f := newScreenshotFixture(t)
	// The window only shows the library's path, so it names one in the fixture's fake home, which the window shows as ~/Movies/….
	shown := filepath.Join(f.home, "Movies", "我的影片.lingocut")
	// Two whisper models on disk, the recommended one in use, so the 语音识别 page shows models that can be chosen beside ones still to download.
	models := filepath.Join(f.home, "models")
	if err := os.MkdirAll(models, 0o755); err != nil {
		t.Fatal(err)
	}
	turbo := filepath.Join(models, "ggml-large-v3-turbo-q5_0.bin")
	for _, file := range []string{"ggml-base.bin", "ggml-large-v3-turbo-q5_0.bin"} {
		if err := os.WriteFile(filepath.Join(models, file), []byte("ggml"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, section := range []string{settings.SectionGeneral, settings.SectionSpeech, settings.SectionAI, settings.SectionTranslate, settings.SectionLook, settings.SectionShortcuts, settings.SectionAccount} {
		a := newApp(f.pipeline, func(func()) {})
		a.edit(func(s *session) {
			s.LibraryDir = shown
			s.Libraries = []string{shown}
		})
		a.prefs.modelsDir = models
		if tools, ok := f.pipeline.(media.Tools); ok {
			tools.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}
			tools.WhisperModel = turbo
			a.syncLLM(tools)
		}
		a.OpenSettings(section)
		tt := ui.NewTester(a.settingsView, 880, 640)
		tt.SetTitleBar(ui.TitleBar{Height: 50, Left: 72})
		tt.Frame()
		tt.Frame()
		file, err := os.Create(filepath.Join(f.out, "settings-"+section+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
		file.Close()
	}
}
