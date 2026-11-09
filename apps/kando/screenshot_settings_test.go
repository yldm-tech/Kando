package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsSettings renders the settings window, one shot per category, at the prototype's 880 by 640 with the traffic lights in their 50-point band (screenshot_helpers_test.go). The window has a view of its own, so it is drawn here rather than through the fixture's shoot, which draws the main window.
func TestScreenshotsSettings(t *testing.T) {
	f := newScreenshotFixture(t)
	shown, models, turbo := settingsShotFolders(t, f)
	for _, section := range []string{settings.SectionGeneral, settings.SectionSpeech, settings.SectionAI, settings.SectionTranslate, settings.SectionLook, settings.SectionShortcuts, settings.SectionAccount} {
		shootSettings(t, f, section, "settings-"+section, shown, models, turbo)
	}
}

// shootSettings renders the settings window open at section, at the prototype's 880 by 640, as name.png: the library shown, the whisper models in models with turbo in use, and EveryAPI as the AI service.
func shootSettings(t *testing.T, f *screenshotFixture, section, name, shown, models, turbo string) {
	t.Helper()
	a := newApp(f.pipeline, func(func()) {})
	a.prefs.interfaceLanguage = testlocale.Current()
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
	file, err := os.Create(filepath.Join(f.out, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

// settingsShotFolders makes what the settings window shows in the fixture's fake home, and returns the library it names, the folder of whisper models and the model in use.
func settingsShotFolders(t *testing.T, f *screenshotFixture) (shown, models, turbo string) {
	t.Helper()
	// The window only shows the library's path, so it names one in the fixture's fake home, which the window shows as ~/Movies/….
	shown = filepath.Join(f.home, "Movies", protoLibraries[0].Name+".kando")
	// Two whisper models on disk, the recommended one in use, so the 语音识别 page shows models that can be chosen beside ones still to download.
	models = filepath.Join(f.home, "models")
	if err := os.MkdirAll(models, 0o755); err != nil {
		t.Fatal(err)
	}
	turbo = filepath.Join(models, "ggml-large-v3-turbo-q5_0.bin")
	for _, file := range []string{"ggml-base.bin", "ggml-large-v3-turbo-q5_0.bin"} {
		if err := os.WriteFile(filepath.Join(models, file), []byte("ggml"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return shown, models, turbo
}
