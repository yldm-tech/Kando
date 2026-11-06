package main

import (
	"os"
	"path/filepath"
	"testing"

	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"
)

func TestShortVersion(t *testing.T) {
	if got := shortVersion("ffmpeg version 9.0.2 Copyright"); got != "9.0.2" {
		t.Fatalf("ffmpeg = %q", got)
	}
	if got := shortVersion("whisper.cpp version: 1.9.5"); got != "1.9.5" {
		t.Fatalf("whisper = %q", got)
	}
	if got := shortVersion("2026.08.19"); got != "2026.08.19" {
		t.Fatalf("yt-dlp = %q", got)
	}
}

func TestSettingsPages(t *testing.T) {
	t.Setenv("LINGOCUT_GOOGLE_CLIENT_ID", "")
	t.Setenv("LINGOCUT_GOOGLE_CLIENT_SECRET", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) {
		s.LibraryDir = filepath.Join(home, "Movies", "Kando")
		s.Libraries = []string{s.LibraryDir}
	})
	tt.Frame()
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	for _, text := range []string{"资料库", "当前资料库", "Kando", "位置", "~/Movies/Kando", "在访达中显示", "界面语言", "简体中文", "版本", appVersion, "组件", "yt-dlp", "ffmpeg", "whisper-cli", "未安装"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if tt.HasText("只从看懂的列表里拿掉") {
		t.Fatalf("the only library can be taken off the list, texts %q", tt.Texts())
	}
	if tt.HasText("重新检测") {
		t.Fatal("redetect is only for a real tool pipeline")
	}
	tt = settingsOpenAt(a, tt, settings.SectionSpeech)
	for _, text := range []string{"whisper 模型", "Tiny", "Large v3 Turbo（压缩版）", "推荐", "下载"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	tt = settingsOpenAt(a, tt, settings.SectionAI)
	if !tt.HasText("未配置") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt = settingsOpenAt(a, tt, settings.SectionTranslate)
	for _, text := range []string{"默认目标语言", "简体中文", "智能翻译", "翻译提示词", "正在使用默认提示词", "恢复默认", "{{language}}"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	tt = settingsOpenAt(a, tt, settings.SectionAccount)
	for _, text := range []string{"Google", "未登录", "使用 Google 登录", "LINGOCUT_GOOGLE_CLIENT_ID", "Cloudflare R2", "Endpoint", "Bucket", "Access Key", "Secret"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestSpeechModelSelectionIsSaved(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "ggml-base.bin")
	if err := os.WriteFile(model, []byte("ggml"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, _ := newTestApp(media.Tools{})
	a.prefs.modelsDir = dir
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt.Frame()
	tt = settingsOpenAt(a, tt, settings.SectionSpeech)
	if !tt.HasText("已下载") || !tt.HasText("下载") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("Base"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tools, ok := a.pipeline.(media.Tools)
	if !ok || tools.WhisperModel != model {
		t.Fatalf("model %#v", a.pipeline)
	}
	if loadSavedSettings(a.settingsPath).WhisperModel != model {
		t.Fatalf("saved %#v", loadSavedSettings(a.settingsPath))
	}
}

func TestAISettingsSelectsAProvider(t *testing.T) {
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}}
	a, tt, _ := newTestApp(tools)
	a.syncLLM(tools)
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt.Frame()
	tt = settingsOpenAt(a, tt, settings.SectionAI)
	for _, text := range []string{"EveryAPI", "本机已登录的 EveryAPI", "Base URL", "API Key", "模型", "测试连接", "尚未测试"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if err := tt.Click("选择服务"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, text := range []string{"OpenAI 兼容", "本机 · Ollama", "本机 · LM Studio"} {
		if !tt.HasText(text) {
			t.Fatalf("the service list is missing %q: %q", text, tt.Texts())
		}
	}
	if err := tt.Click("OpenAI"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	got, ok := a.pipeline.(media.Tools)
	if !ok || got.Translator.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("base = %#v", a.pipeline)
	}
	if loadSavedSettings(a.settingsPath).LLMBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("saved %#v", loadSavedSettings(a.settingsPath))
	}
}

// TestOpenSettingsGoesToTheSection checks that OpenSettings records the section asked for, falling back to 通用, and leaves the main window's page and its open film alone: the settings open in a window of their own.
func TestOpenSettingsGoesToTheSection(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "talk.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{ID: "talk", Source: "file://" + video, Stage: core.StageComplete, VideoPath: video}}
	a.openKey = "talk"
	tt.Frame()
	a.OpenSettings(settings.SectionTranslate)
	tt.Frame()
	wantSettings(t, a, settings.SectionTranslate)
	if a.page != pageFilms || a.openKey != "talk" {
		t.Fatalf("page %q open %q", a.page, a.openKey)
	}
	for section, want := range map[string]string{"": settings.SectionGeneral, "nope": settings.SectionGeneral, settings.SectionLook: settings.SectionLook, settings.SectionShortcuts: settings.SectionShortcuts, settings.SectionAccount: settings.SectionAccount} {
		a.OpenSettings(section)
		tt.Frame()
		wantSettings(t, a, want)
		if a.page != pageFilms || a.openKey != "talk" {
			t.Fatalf("OpenSettings(%q) left page %q open %q", section, a.page, a.openKey)
		}
	}
}

// TestSettingsFollowTheSidebarPages checks that choosing a settings page in the main sidebar, whose 设置 rows stay until the sidebar drops them, opens the settings window there and puts the main window back on 全部影片.
func TestSettingsFollowTheSidebarPages(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	a.Select(pageTranslate)
	tt.Frame()
	wantSettings(t, a, settings.SectionTranslate)
	if a.page != pageFilms {
		t.Fatalf("the main window stayed on page %q", a.page)
	}
}
