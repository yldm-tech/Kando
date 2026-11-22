package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/film"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestSettingsWindowSwitchesCategoryAndToggles switches the window's category in its sidebar and turns a switch there, which is saved.
func TestSettingsWindowSwitchesCategoryAndToggles(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	for _, text := range []string{"通用", "语音识别", "AI 服务", "翻译", "字幕外观", "快捷键", "账号与上传"} {
		if !hasText(tt, text) {
			t.Fatalf("the window's sidebar is missing %q: %q", text, tt.Texts())
		}
	}
	tt.Press("id:settings-section-" + settings.SectionTranslate)
	if a.SettingsSection() != settings.SectionTranslate || !hasText(tt, "默认目标语言") {
		t.Fatalf("section %q texts %q", a.SettingsSection(), tt.Texts())
	}
	before := a.prefs.translateContext
	tt.Press("智能翻译")
	if a.prefs.translateContext == before {
		t.Fatal("the switch did not turn")
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.TranslateContext == nil || *saved.TranslateContext != a.prefs.translateContext {
		t.Fatalf("saved %#v", saved.TranslateContext)
	}
}

// TestSettingsSaySaveFailed checks that a switch turned on a settings page whose settings.json cannot be written says so under the page, rather than seeming to stick until Kando quits.
func TestSettingsSaySaveFailed(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	// A file where settings.json's folder should be cannot hold it.
	blocker := filepath.Join(t.TempDir(), "Kando")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	a.settingsPath = filepath.Join(blocker, "settings.json")
	tt = settingsOpenAt(a, tt, settings.SectionTranslate)
	tt.Press("智能翻译")
	if prefix := testlocale.Messages().SettingsSaveFailed(""); !strings.HasPrefix(a.saveErr, prefix) || !hasText(tt, a.saveErr) {
		t.Fatalf("save error %q, texts %q", a.saveErr, tt.Texts())
	}
}

// TestSettingsSearchFindsCategories types in the window's search field: categories whose words do not match leave the sidebar, and a query nothing matches says so.
func TestSettingsSearchFindsCategories(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	tt.Type("搜索设置", "bucket")
	if !hasText(tt, "账号与上传") || hasText(tt, "字幕外观") || hasText(tt, "快捷键") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Type("搜索设置", "没有这种设置")
	if !hasText(tt, "没有匹配的设置") || hasText(tt, "账号与上传") {
		t.Fatalf("texts %q", tt.Texts())
	}
	// The speech page's controls are found by their names, as the hint after a failed download from Hugging Face sends users to the download source.
	for _, query := range []string{"下载源", "镜像", "hf-mirror", "识别语言", "模型文件", "ggml"} {
		tt.Clear("搜索设置")
		tt.Type("搜索设置", query)
		if !hasText(tt, "语音识别") || hasText(tt, "账号与上传") {
			t.Fatalf("%q: texts %q", query, tt.Texts())
		}
	}
}

// TestSettingsSearchFindsTheSpeechControlsInEnglish searches the English settings for the speech page's controls by their names.
func TestSettingsSearchFindsTheSpeechControlsInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, tt, _ := newTestApp(t, &fakePipeline{})
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	msgs := testlocale.Messages()
	for _, query := range []string{"download source", "mirror", "hf-mirror.com", "spoken language", "model file", "ggml", "hugging face"} {
		tt.Clear(msgs.SettingsSearchLabel())
		tt.Type(msgs.SettingsSearchLabel(), query)
		if !hasText(tt, msgs.SettingsSectionSpeech()) || hasText(tt, msgs.SettingsSectionAccount()) {
			t.Fatalf("%q: texts %q", query, tt.Texts())
		}
	}
}

// TestLookSettingsChangeThePlayer chooses the subtitle settings on the 字幕外观 page, which lines show and how they look: they are the player's own state, and they are saved.
func TestLookSettingsChangeThePlayer(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt = settingsOpenAt(a, tt, settings.SectionLook)
	if !hasText(tt, "每条河都始于高山上的一股细流。") || textCount(tt, "Every river begins") == 0 {
		t.Fatalf("the preview is missing: %q", tt.Texts())
	}
	for _, label := range []string{"仅原文", "原文在上", "留空"} {
		tt.Press(whole(label))
	}
	tt.Press("控件出现时字幕上移让位")
	if a.player.subMode != film.SubOriginal || a.player.subOrder != film.SubOriginalFirst || a.player.untranslated != film.UntranslatedBlank || a.player.yieldSubs {
		t.Fatalf("player %q %q %q %v", a.player.subMode, a.player.subOrder, a.player.untranslated, a.player.yieldSubs)
	}
	if hasText(tt, "每条河都始于高山上的一股细流。") || textCount(tt, "Every river begins") == 0 {
		t.Fatalf("the preview does not follow the mode: %q", tt.Texts())
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.SubMode != film.SubOriginal || saved.SubOrder != film.SubOriginalFirst || saved.Untranslated != film.UntranslatedBlank || saved.YieldSubs == nil || *saved.YieldSubs {
		t.Fatalf("saved %q %q %q %v", saved.SubMode, saved.SubOrder, saved.Untranslated, saved.YieldSubs)
	}
	tt.Press("id:settings-look-preset >> " + whole("底框"))
	tt.Press("id:settings-look-bold-original")
	if look := a.player.look; !look.Box || look.Preset != "box" || !look.Original.Bold {
		t.Fatalf("the player's look %+v", look)
	}
	if look := loadSavedSettings(a.settingsPath).Look; look == nil || !look.Box || !look.Original.Bold {
		t.Fatalf("saved look %+v", look)
	}
}

// TestShortcutsPageListsTheTable checks that the 快捷键 page lists the app's shortcut table, with its keys as the platform writes them (⌘ on macOS, Ctrl elsewhere), and filters it by its search field.
func TestShortcutsPageListsTheTable(t *testing.T) {
	for _, c := range []struct {
		name                       string
		linux                      bool
		settingsKey, cheatsheetKey string
	}{{"macOS", false, "⌘,", "⌘/"}, {"Linux", true, "Ctrl+,", "Ctrl+/"}} {
		t.Run(c.name, func(t *testing.T) {
			if c.linux {
				usePlatform(t, ns.OSLinux)
			}
			a, tt, _ := newTestApp(t, &fakePipeline{})
			tt = settingsOpenAt(a, tt, settings.SectionShortcuts)
			for _, s := range a.shortcuts(a.cx) {
				if s.Key != "" && !hasText(tt, s.Label) {
					t.Fatalf("missing %q in %q", s.Label, tt.Texts())
				}
			}
			if !hasText(tt, c.settingsKey) || !hasText(tt, c.cheatsheetKey) {
				t.Fatalf("keys are missing: %q", tt.Texts())
			}
			tt.Type("搜索命令或按键", "全屏")
			if !hasText(tt, "全屏") || hasText(tt, "设置…") {
				t.Fatalf("texts %q", tt.Texts())
			}
		})
	}
}

// TestSecretsStayHidden checks that the API key and the storage secret are edited in password fields, whose text never shows: the platform's secure field neither shows nor copies it.
func TestSecretsStayHidden(t *testing.T) {
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1", APIKey: "sk-visible-key"}}
	a, tt, _ := newTestApp(t, tools)
	a.syncLLM(tools)
	a.cloud.r2Secret = "r2-visible-secret"
	for _, c := range []struct{ section, row, secret string }{
		{settings.SectionAI, "API Key", "sk-visible-key"},
		{settings.SectionAccount, "Secret", "r2-visible-secret"},
	} {
		tt = settingsOpenAt(a, tt, c.section)
		if textCount(tt, c.secret) != 0 {
			t.Fatalf("%s shows the secret: %q", c.row, tt.Texts())
		}
		field := tt.Find("role:textbox " + c.row)
		if !field.Input.Masked {
			t.Fatalf("the %s field is not a password field", c.row)
		}
		clickRow(t, tt, "role:textbox "+c.row)
	}
}

// TestSpeechModelsAreFoundOutsideTheView checks that the 语音识别 page reads the models on disk from the app, which looks them up when the window opens and not while the page is drawn, and that a model not on disk cannot be chosen.
func TestSpeechModelsAreFoundOutsideTheView(t *testing.T) {
	dir := t.TempDir()
	a, tt, _ := newTestApp(t, media.Tools{})
	a.prefs.modelsDir = dir
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt = settingsOpenAt(a, tt, settings.SectionSpeech)
	if hasText(tt, "已下载") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if tt.Enabled("Base") {
		t.Fatal("a model that is not on disk can be chosen")
	}
	model := filepath.Join(dir, "ggml-base.bin")
	if err := os.WriteFile(model, []byte("ggml"), 0o644); err != nil {
		t.Fatal(err)
	}
	refresh(tt, a)
	if hasText(tt, "已下载") {
		t.Fatal("drawing the page read the disk")
	}
	a.OpenSettings(settings.SectionSpeech)
	refresh(tt, a)
	if !hasText(tt, "已下载") {
		t.Fatalf("opening the window again did not find the model: %q", tt.Texts())
	}
	tt.Press("Base")
	if tools := a.pipeline.(media.Tools); tools.WhisperModel != model {
		t.Fatalf("model %q", tools.WhisperModel)
	}
}

// TestModelDownloadShowsProgress checks a downloading model's row: a progress bar and a button that cancels the download.
func TestModelDownloadShowsProgress(t *testing.T) {
	a, tt, _ := newTestApp(t, media.Tools{})
	ctx, cancel := context.WithCancel(context.Background())
	a.prefs.downloading, a.prefs.downloadWritten, a.prefs.downloadTotal, a.prefs.downloadCancel = "base", 50, 100, cancel
	tt = settingsOpenAt(a, tt, settings.SectionSpeech)
	if !tt.Exists("Base 下载进度") {
		t.Fatalf("no progress bar: %q", tt.Texts())
	}
	tt.Press("取消")
	if ctx.Err() == nil {
		t.Fatal("the download was not canceled")
	}
}

// TestSettingsStayWhileTheAppChanges types into the AI service's Base URL in the settings over the main window, then lets background reports change the app's state as a download does: the field stays the same view, keeping the focus and what was typed.
func TestSettingsStayWhileTheAppChanges(t *testing.T) {
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1"}}
	a, tt, _ := newTestApp(t, tools)
	a.syncLLM(tools)
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt = settingsOpenAt(a, tt, settings.SectionAI)
	clickRow(t, tt, "id:settings-llm-base")
	typeFocused(t, tt, "/x")
	field := tt.Find("id:settings-llm-base")
	apply(tt, a, func() { a.prefs.downloadWritten = 10 })
	apply(tt, a, func() {})
	again := tt.Find("id:settings-llm-base")
	if again.ID != field.ID || !again.Focused || again.Text != "https://api.openai.com/v1/x" {
		t.Fatalf("the field was built again (%d, then %d) or lost the focus (%v): %q", field.ID, again.ID, again.Focused, again.Text)
	}
}

// TestOpeningTheSettingsBringsTheMainWindowBack checks that opening the settings window, a child of the main window, which macOS hides while the main window is put away or minimized, brings the main window to the front first when its close button has put it away or it is minimized, and leaves it alone when it shows.
func TestOpeningTheSettingsBringsTheMainWindowBack(t *testing.T) {
	for _, away := range []string{"", "hidden", "minimized"} {
		a, tt, _ := newTestApp(t, &fakePipeline{})
		a.prefs.overMain = false
		// windowWatch, which keeps the main window, runs from ready, which the tests' app does not run.
		a.window.main = nsdesktop.MainWindow(a.cx)
		switch away {
		case "hidden":
			a.window.hidden = true
		case "minimized":
			tt.SetWindowEnvironment("", func(env *ns.Environment) { env.Minimized = true })
		}
		before := len(tt.Window)
		a.OpenSettings(settings.SectionGeneral)
		refresh(tt, a)
		fronted := slices.ContainsFunc(tt.Window[before:], func(r nsgotest.WindowRequest) bool { return r.Action == nsgotest.WindowOrderFront })
		if a.prefs.win == nil || a.window.hidden || fronted != (away != "") {
			t.Fatalf("main window %q: settings window %v, main window hidden %v, brought to the front %v", away, a.prefs.win, a.window.hidden, fronted)
		}
	}
}

// TestSettingsInTheirOwnWindow tests the settings in the window of their own they open in, as users meet them, rather than over the main window, where newTestApp has the other settings tests show them (settingsUI.overMain): the window's size and limits, its sidebar and a switch that is saved, its search, ⌘, bringing the same window back to another section, and its close button, which forgets it, so the next ⌘, opens a new one.
func TestSettingsInTheirOwnWindow(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.prefs.overMain = false
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.OpenSettings(settings.SectionGeneral)
	refresh(tt, a)
	title := "title:" + testlocale.Messages().SettingsWindowTitle()
	win := tt.Surface(title)
	if c := win.Config(); c.Width != 880 || c.Height != 640 || c.Constraints.MinWidth != 720 || c.Constraints.MinHeight != 480 || !c.Constraints.NoMaximize {
		t.Fatalf("settings window opened %+v", c)
	}
	inWindow := func(text string) bool { return len(win.Query(whole(text))) > 0 }
	if tt.Exists("settings-in-window") || !inWindow("通用") || !inWindow("账号与上传") {
		t.Fatalf("the settings are not in their window alone: over the main window %v, sidebar shows %v", tt.Exists("settings-in-window"), inWindow("通用"))
	}

	win.Press("id:settings-section-" + settings.SectionTranslate)
	wantSettings(t, a, settings.SectionTranslate)
	before := a.prefs.translateContext
	win.Press("智能翻译")
	if saved := loadSavedSettings(a.settingsPath); a.prefs.translateContext == before || saved.TranslateContext == nil || *saved.TranslateContext != a.prefs.translateContext {
		t.Fatalf("the switch in the settings window turned %v, saved %#v", a.prefs.translateContext != before, saved.TranslateContext)
	}
	win.Type("搜索设置", "bucket")
	if a.prefs.query != "bucket" || !inWindow("账号与上传") || inWindow("字幕外观") {
		t.Fatalf("search %q: 账号与上传 %v, 字幕外观 %v", a.prefs.query, inWindow("账号与上传"), inWindow("字幕外观"))
	}

	a.OpenSettings(settings.SectionSpeech)
	refresh(tt, a)
	if n := len(tt.Surfaces()); n != 2 || tt.Surface(title) != win {
		t.Fatalf("⌘, with the settings open left %d windows, want the main window and the same settings window", n)
	}
	wantSettings(t, a, settings.SectionSpeech)

	if !win.Close() {
		t.Fatal("the settings window's close button did not close it")
	}
	refresh(tt, a)
	if a.prefs.open || a.prefs.win != nil || len(tt.Surfaces()) != 1 {
		t.Fatalf("after closing: open %v, window %v, %d windows", a.prefs.open, a.prefs.win, len(tt.Surfaces()))
	}
	a.OpenSettings(settings.SectionGeneral)
	refresh(tt, a)
	if again := tt.Surface(title); again == win || a.prefs.win == nil {
		t.Fatal("⌘, after the settings window closed did not open a new one")
	}
}
