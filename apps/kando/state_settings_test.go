package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"kando/apps/kando/film"
	"kando/apps/kando/settings"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
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
