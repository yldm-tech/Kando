package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"kando/apps/kando/film"
	"kando/apps/kando/settings"
	"kando/packages/media"
)

// TestSettingsWindowSwitchesCategoryAndToggles switches the window's category in its sidebar and turns a switch there, which is saved.
func TestSettingsWindowSwitchesCategoryAndToggles(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt := newSettingsTester(a, settings.SectionGeneral)
	for _, text := range []string{"通用", "语音识别", "AI 服务", "翻译", "字幕外观", "快捷键", "账号与上传"} {
		if !tt.HasText(text) {
			t.Fatalf("the window's sidebar is missing %q: %q", text, tt.Texts())
		}
	}
	if err := tt.Click("翻译"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.SettingsSection() != settings.SectionTranslate || !tt.HasText("默认目标语言") {
		t.Fatalf("section %q texts %q", a.SettingsSection(), tt.Texts())
	}
	before := a.prefs.translateContext
	if err := tt.Click("智能翻译"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
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
	a, _, _ := newTestApp(&fakePipeline{})
	tt := newSettingsTester(a, settings.SectionGeneral)
	if err := tt.Click("搜索设置"); err != nil {
		t.Fatal(err)
	}
	tt.Type("bucket")
	tt.Frame()
	if !tt.HasText("账号与上传") || tt.HasText("字幕外观") || tt.HasText("快捷键") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Type("没有这种设置")
	tt.Frame()
	if !tt.HasText("没有匹配的设置") || tt.HasText("账号与上传") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// TestLookSettingsChangeThePlayer chooses the subtitle settings on the 字幕外观 page: they are the player's own state, and they are saved.
func TestLookSettingsChangeThePlayer(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt := newSettingsTester(a, settings.SectionLook)
	if !tt.HasText("每条河都始于高山上的一股细流。") || !tt.HasText("Every river begins") {
		t.Fatalf("the preview is missing: %q", tt.Texts())
	}
	for _, label := range []string{"仅原文", "原文在上", "留空", "控件出现时字幕上移让位"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	if a.player.subMode != film.SubOriginal || a.player.subOrder != film.SubOriginalFirst || a.player.untranslated != film.UntranslatedBlank || a.player.yieldSubs {
		t.Fatalf("player %q %q %q %v", a.player.subMode, a.player.subOrder, a.player.untranslated, a.player.yieldSubs)
	}
	if tt.HasText("每条河都始于高山上的一股细流。") || !tt.HasText("Every river begins") {
		t.Fatalf("the preview does not follow the mode: %q", tt.Texts())
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.SubMode != film.SubOriginal || saved.SubOrder != film.SubOriginalFirst || saved.Untranslated != film.UntranslatedBlank || saved.YieldSubs == nil || *saved.YieldSubs {
		t.Fatalf("saved %q %q %q %v", saved.SubMode, saved.SubOrder, saved.Untranslated, saved.YieldSubs)
	}
}

// TestShortcutsPageListsTheTable checks that the 快捷键 page lists the app's shortcut table and filters it by its search field.
func TestShortcutsPageListsTheTable(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	tt := newSettingsTester(a, settings.SectionShortcuts)
	for _, s := range a.shortcuts() {
		if !tt.HasText(s.Label) {
			t.Fatalf("missing %q in %q", s.Label, tt.Texts())
		}
	}
	if !tt.HasText("⌘,") || !tt.HasText("⌘/") {
		t.Fatalf("keys are missing: %q", tt.Texts())
	}
	if err := tt.Click("搜索命令或按键"); err != nil {
		t.Fatal(err)
	}
	tt.Type("全屏")
	tt.Frame()
	if !tt.HasText("全屏") || tt.HasText("设置…") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// TestSecretsStayHidden checks that the API key and the storage secret are edited in password fields: the text never shows, and Copy takes nothing from them.
func TestSecretsStayHidden(t *testing.T) {
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1", APIKey: "sk-visible-key"}}
	a, _, _ := newTestApp(tools)
	a.syncLLM(tools)
	a.cloud.r2Secret = "r2-visible-secret"
	for _, c := range []struct{ section, row, secret string }{
		{settings.SectionAI, "API Key", "sk-visible-key"},
		{settings.SectionAccount, "Secret", "r2-visible-secret"},
	} {
		tt := newSettingsTester(a, c.section)
		if tt.HasText(c.secret) {
			t.Fatalf("%s shows the secret: %q", c.row, tt.Texts())
		}
		// The field sits at the end of its row, at the row's first line.
		r, ok := tt.Find(c.row)
		if !ok {
			t.Fatalf("no row %q: %q", c.row, tt.Texts())
		}
		tt.ClickAt(r.X+r.W-60, r.Y+20)
		tt.Frame()
		if _, editing := tt.TextCaret(); !editing {
			t.Fatalf("clicking the %s row did not reach its field", c.row)
		}
		tt.SetClipboard("before")
		tt.Command("selectAll")
		tt.Command("copy")
		tt.Frame()
		if tt.Clipboard() != "before" {
			t.Fatalf("Copy took %q from the %s field", tt.Clipboard(), c.row)
		}
	}
}

// TestSpeechModelsAreFoundOutsideTheView checks that the 语音识别 page reads the models on disk from the app, which looks them up when the window opens and not while the page is drawn, and that a model not on disk cannot be chosen.
func TestSpeechModelsAreFoundOutsideTheView(t *testing.T) {
	dir := t.TempDir()
	a, _, _ := newTestApp(media.Tools{})
	a.prefs.modelsDir = dir
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt := newSettingsTester(a, settings.SectionSpeech)
	if tt.HasText("已下载") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("Base"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tools := a.pipeline.(media.Tools); tools.WhisperModel != "" {
		t.Fatalf("a model that is not on disk was chosen: %q", tools.WhisperModel)
	}
	model := filepath.Join(dir, "ggml-base.bin")
	if err := os.WriteFile(model, []byte("ggml"), 0o644); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("已下载") {
		t.Fatal("drawing the page read the disk")
	}
	a.OpenSettings(settings.SectionSpeech)
	tt.Frame()
	if !tt.HasText("已下载") {
		t.Fatalf("opening the window again did not find the model: %q", tt.Texts())
	}
	if err := tt.Click("Base"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tools := a.pipeline.(media.Tools); tools.WhisperModel != model {
		t.Fatalf("model %q", tools.WhisperModel)
	}
}

// TestModelDownloadShowsProgress checks a downloading model's row: a progress bar and a button that cancels the download.
func TestModelDownloadShowsProgress(t *testing.T) {
	a, _, _ := newTestApp(media.Tools{})
	ctx, cancel := context.WithCancel(context.Background())
	a.prefs.downloading, a.prefs.downloadWritten, a.prefs.downloadTotal, a.prefs.downloadCancel = "base", 50, 100, cancel
	tt := newSettingsTester(a, settings.SectionSpeech)
	if _, ok := tt.Find("Base 下载进度"); !ok {
		t.Fatalf("no progress bar: %q", tt.Texts())
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if ctx.Err() == nil {
		t.Fatal("the download was not canceled")
	}
}

// TestSettingsWindowsShareWhatTheyDraw checks the bookkeeping that keeps the main window and the settings window drawing the same settings: a window that draws settings the other has not drawn asks the other for a frame, once.
func TestSettingsWindowsShareWhatTheyDraw(t *testing.T) {
	var s settingsUI
	if !s.drew(false, "a") {
		t.Fatal("the settings window has not drawn a yet")
	}
	if s.drew(true, "a") {
		t.Fatal("the main window has drawn a already")
	}
	if s.drew(false, "a") {
		t.Fatal("nothing changed")
	}
	if !s.drew(true, "b") || s.drew(false, "b") {
		t.Fatal("a change in the settings window reaches the main window once")
	}
}
