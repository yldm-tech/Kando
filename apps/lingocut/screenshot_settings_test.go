package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"lingocut/apps/lingocut/settings"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsSettings renders the settings window, one shot per category, at the prototype's 880 by 640 with the traffic lights in their 50-point band (screenshot_helpers_test.go). The window has a view of its own, so it is drawn here rather than through the fixture's shoot, which draws the main window.
func TestScreenshotsSettings(t *testing.T) {
	f := newScreenshotFixture(t)
	// The window only shows the library's path, so it names one in the usual place rather than the test's temporary directory, which would show this machine's paths.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	shown := filepath.Join(home, "Movies", "我的影片.lingocut")
	for _, section := range []string{settings.SectionGeneral, settings.SectionSpeech, settings.SectionAI, settings.SectionTranslate, settings.SectionLook, settings.SectionShortcuts, settings.SectionAccount} {
		a := newApp(f.pipeline, func(func()) {})
		a.edit(func(s *session) {
			s.LibraryDir = shown
			s.Libraries = []string{shown}
		})
		if tools, ok := f.pipeline.(media.Tools); ok {
			tools.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}
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
