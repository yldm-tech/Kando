package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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
	t.Setenv("KANDO_GOOGLE_CLIENT_ID", "")
	t.Setenv("KANDO_GOOGLE_CLIENT_SECRET", "")
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
	for _, text := range []string{"Google", "未登录", "使用 Google 登录", "KANDO_GOOGLE_CLIENT_ID", "Cloudflare R2", "Endpoint", "Bucket", "Access Key", "Secret"} {
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

// rejected is an API answer with an HTTP status, as media's errors carry one.
type rejected int

func (r rejected) Error() string   { return "rejected " + strconv.Itoa(int(r)) }
func (r rejected) StatusCode() int { return int(r) }

// TestAISettingsChoosesTheModelFromTheServiceList checks that Settings › AI 服务 lists the key before the model, fetches the service's models once it shows instead of having the model typed, picks the service's default model when none was chosen and saves it, and that a list the service refuses says why and fetches again on 重试.
func TestAISettingsChoosesTheModelFromTheServiceList(t *testing.T) {
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL}}
	a, main, updates := newTestApp(tools)
	a.syncLLM(tools)
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	calls := 0
	var answer error
	a.prefs.listModels = func(_ context.Context, tr media.ChatTranslator) ([]string, error) {
		calls++
		if tr.BaseURL != media.EveryAPIBaseURL {
			t.Errorf("asked %q for its models", tr.BaseURL)
		}
		if answer != nil {
			return nil, answer
		}
		return []string{"alpha-chat", media.EveryAPIModel, "zeta-chat"}, nil
	}
	main.Frame()
	if calls != 0 {
		t.Fatalf("fetched the models with the settings closed")
	}
	tt := settingsOpenAt(a, main, settings.SectionAI)
	main.Frame()
	waitUntil(t, updates, func() bool { return !a.prefs.llmModelsLoading })
	if calls != 1 || a.prefs.llmModel != media.EveryAPIModel || loadSavedSettings(a.settingsPath).LLMModel != media.EveryAPIModel {
		t.Fatalf("calls %d model %q saved %q", calls, a.prefs.llmModel, loadSavedSettings(a.settingsPath).LLMModel)
	}
	main.Frame()
	if calls != 1 {
		t.Fatalf("fetched again for the same service: %d", calls)
	}
	tt.Frame()
	key, okKey := tt.Find("API Key")
	model, okModel := tt.Find("模型")
	if !okKey || !okModel || key.Y >= model.Y {
		t.Fatalf("API Key %v %+v, 模型 %v %+v", okKey, key, okModel, model)
	}
	if err := tt.Click("模型"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("zeta-chat"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	if got, _ := a.pipeline.(media.Tools); got.Translator.Model != "zeta-chat" || loadSavedSettings(a.settingsPath).LLMModel != "zeta-chat" {
		t.Fatalf("model %q saved %q", got.Translator.Model, loadSavedSettings(a.settingsPath).LLMModel)
	}

	answer = rejected(401)
	a.RetryLLMModels()
	waitUntil(t, updates, func() bool { return !a.prefs.llmModelsLoading })
	tt.Frame()
	if calls != 2 || !tt.HasText("重试") || a.prefs.llmModelsErr != "服务不接受这个 API Key，取不到模型列表" {
		t.Fatalf("calls %d error %q texts %q", calls, a.prefs.llmModelsErr, tt.Texts())
	}
	if !tt.HasText("zeta-chat") {
		t.Fatalf("the chosen model left the row: %q", tt.Texts())
	}
	answer = nil
	if err := tt.Click("重试"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, updates, func() bool { return !a.prefs.llmModelsLoading })
	if calls != 3 || a.prefs.llmModelsErr != "" || len(a.prefs.llmModels) != 3 {
		t.Fatalf("calls %d error %q models %q", calls, a.prefs.llmModelsErr, a.prefs.llmModels)
	}
}
