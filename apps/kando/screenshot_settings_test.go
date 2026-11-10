package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"kando/apps/kando/clitool"
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
		shootSettings(t, f, section, "settings-"+section, shown, models, turbo, func(a *app) { shotCLITool(f, a, true) })
	}
	// The 命令行与 MCP card at the end of 通用: before the kando command is installed, with 安装; and once a terminal finds it, after 拷贝配置.
	shootSettingsScrolled(t, f, settings.SectionGeneral, "settings-general-cli", shown, models, turbo, func(a *app) { shotCLITool(f, a, false) })
	shootSettingsScrolled(t, f, settings.SectionGeneral, "settings-general-cli-installed", shown, models, turbo, func(a *app) {
		shotCLITool(f, a, true)
		a.cliTool.copied = clitool.MCPConfig(a.cliTool.status.MCPCommand())
	})
	// An installed copy, which updates itself: the last check found 0.5.0, and the switches of the checks and downloads in the background show.
	shootSettings(t, f, settings.SectionGeneral, "settings-general-updates", shown, models, turbo, func(a *app) {
		shotCLITool(f, a, true)
		a.updates.shown = settings.Updates{Enabled: true, AutoCheck: true, Available: "0.5.0"}
	})
	// EveryAPI without a key on a computer without the everyapi program: the row under API Key offers 获取 EveryAPI and 重新检测.
	shootSettings(t, f, settings.SectionAI, "settings-ai-everyapi-missing", shown, models, turbo, func(a *app) {
		tools := a.pipeline.(media.Tools)
		tools.EveryAPI = ""
		a.syncLLM(tools.UseEveryAPICLI())
	})
	// The everyapi program is there but nobody signed in to it, as the sidebar's check found: the row says what to run in Terminal.
	shootSettings(t, f, settings.SectionAI, "settings-ai-everyapi-signed-out", shown, models, turbo, func(a *app) {
		a.side.ready.signInDone, a.side.ready.signInErr = true, media.ErrEveryAPISignedOut
	})
	// The end of 语音识别, with Japanese named as the language spoken in the videos.
	shootSettingsScrolled(t, f, settings.SectionSpeech, "settings-speech-language", shown, models, turbo, func(a *app) {
		tools := a.pipeline.(media.Tools)
		tools.SpeechLanguage = "ja"
		a.pipeline = tools
	})
	// The confirmation of moving Base, which Kando downloaded and which is not in use, to the Trash, as ⋯ › 移到废纸篓… opens it.
	shootSettings(t, f, settings.SectionSpeech, "settings-speech-trash", shown, models, turbo, func(a *app) {
		a.prefs.trashModel, a.prefs.trashOpen = "base", true
	})
	// The first launch on a computer without ffmpeg and whisper-cli: Kando downloads ffmpeg, and offers 重试 for whisper-cli, whose download stalled.
	shootSettings(t, f, settings.SectionGeneral, "settings-general-downloading", shown, models, turbo, func(a *app) {
		shotKitsDownloading(t, f, a)
		a.kits.running = map[string]*kitDownload{"ffmpeg": {written: 13 << 20, total: 31 << 20, cancel: func() {}}}
		a.kits.errs = map[string]error{"whisper-cli": media.ErrStalled}
	})
	// A speech model's download that lost its connection: the page says so, and that downloading again picks up where it stopped.
	shootSettingsScrolled(t, f, settings.SectionSpeech, "settings-speech-download-failed", shown, models, turbo, func(a *app) {
		a.prefs.downloadErr = fmt.Errorf("%w: unexpected EOF", media.ErrDisconnected)
	})
}

// shotKitsDownloading takes ffmpeg and whisper-cli out of a's tools and gives Kando a folder of its own in the fixture's fake home, as on a computer that lacks them, for the scenes of their downloads.
func shotKitsDownloading(t *testing.T, f *screenshotFixture, a *app) {
	t.Helper()
	saved := media.OwnToolsDir
	media.OwnToolsDir = filepath.Join(f.home, "Library", "Application Support", "Kando", "tools")
	t.Cleanup(func() { media.OwnToolsDir = saved })
	tools := a.pipeline.(media.Tools)
	tools.FFmpeg, tools.Whisper = "", ""
	a.pipeline = tools
}

// shotCLITool shows Kando.app in /Applications with the kando command in /usr/local/bin when installed, or no kando a terminal finds, as the app's own look would find them. The look the window starts when it opens sees a folder of the fake home with no command, and its answer is dropped, as the scenes' updates are.
func shotCLITool(f *screenshotFixture, a *app, installed bool) {
	a.cliTool.env = func() clitool.Env {
		return clitool.Env{Exe: filepath.Join(f.home, "Kando"), Searched: []string{filepath.Join(f.home, "bin")}}
	}
	a.cliTool.known = true
	a.cliTool.status = clitool.Status{Exe: "/Applications/Kando.app/Contents/MacOS/Kando", Installable: true}
	if installed {
		a.cliTool.status.Command, a.cliTool.status.Ours = "/usr/local/bin/kando", true
	}
}

// shootSettings renders the settings window open at section, at the prototype's 880 by 640, as name.png: the library shown, the whisper models in models with turbo in use, and EveryAPI as the AI service, without a key, through the everyapi program in ~/bin (writeFakeEveryAPI). setup, when given, changes the app before it is drawn.
func shootSettings(t *testing.T, f *screenshotFixture, section, name, shown, models, turbo string, setup ...func(*app)) {
	t.Helper()
	shootSettingsAt(t, f, section, name, shown, models, turbo, false, setup...)
}

// shootSettingsScrolled is shootSettings with the page scrolled to its end.
func shootSettingsScrolled(t *testing.T, f *screenshotFixture, section, name, shown, models, turbo string, setup ...func(*app)) {
	t.Helper()
	shootSettingsAt(t, f, section, name, shown, models, turbo, true, setup...)
}

func shootSettingsAt(t *testing.T, f *screenshotFixture, section, name, shown, models, turbo string, end bool, setup ...func(*app)) {
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
		tools.EveryAPI = writeFakeEveryAPI(t, filepath.Join(f.home, "bin"), true)
		a.syncLLM(tools.UseEveryAPICLI())
	}
	for _, fn := range setup {
		fn(a)
	}
	a.OpenSettings(section)
	tt := ui.NewTester(a.settingsView, 880, 640)
	tt.SetTitleBar(ui.TitleBar{Height: 50, Left: 72})
	tt.Frame()
	tt.Frame()
	if end {
		tt.Scroll(550, 400, 0, 4000)
		tt.Frame()
	}
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
