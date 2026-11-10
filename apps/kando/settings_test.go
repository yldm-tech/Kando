package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
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

// TestSpeechDownloadResumesAfterADrop checks a speech model's download whose connection drops: the page says so and that downloading again continues, and 下载 then asks the server only for the rest and puts the whole model to use.
func TestSpeechDownloadResumesAfterADrop(t *testing.T) {
	data := []byte(strings.Repeat("ggml", 1024))
	var ranges []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ranges = append(ranges, r.Header.Get("Range"))
		w.Header().Set("ETag", `"tiny"`)
		if len(ranges) == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			_, _ = w.Write(data[:1000])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}
		http.ServeContent(w, r, "ggml-tiny.bin", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	saved := whisperModelBaseURL
	whisperModelBaseURL = srv.URL
	t.Cleanup(func() { whisperModelBaseURL = saved })
	a, _, updates := newTestApp(media.Tools{})
	a.prefs.modelsDir = t.TempDir()
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")

	a.DownloadModel("tiny")
	waitUntil(t, updates, func() bool { return a.Downloading() == "" })
	tt := settingsOpenAt(a, nil, settings.SectionSpeech)
	wantText(t, tt, "连接断了。重新下载会从断开的地方接着下。")
	if info, err := os.Stat(filepath.Join(a.prefs.modelsDir, "ggml-tiny.bin.partial")); err != nil || info.Size() != 1000 {
		t.Fatalf("the dropped download kept %v, err %v", info, err)
	}

	a.DownloadModel("tiny")
	if a.DownloadErr() != "" {
		t.Fatalf("downloading again still says %q", a.DownloadErr())
	}
	waitUntil(t, updates, func() bool { return a.Downloading() == "" })
	if len(ranges) != 2 || ranges[1] != "bytes=1000-" {
		t.Fatalf("requests asked for %q, want the rest from byte 1000", ranges)
	}
	model := filepath.Join(a.prefs.modelsDir, "ggml-tiny.bin")
	if got, err := os.ReadFile(model); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("model holds %d bytes, err %v", len(got), err)
	}
	if a.pipeline.(media.Tools).WhisperModel != model || a.DownloadErr() != "" {
		t.Fatalf("model in use %q, error %q", a.pipeline.(media.Tools).WhisperModel, a.DownloadErr())
	}
}

// TestSpeechDownloadErrorSaysWhy checks the words for why a speech model's download failed, in the interface's language: a stall and a dropped connection say that downloading again continues, an HTTP status names it, and anything else shows as it is.
func TestSpeechDownloadErrorSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, ""},
		{errSpeechNoModelsDir, "还没有模型目录"},
		{media.ErrStalled, "下载停住了，一直收不到数据。重新下载会从断开的地方接着下。"},
		{rejected(503), "服务器返回 HTTP 503，请稍后再试。"},
		{fmt.Errorf("%w: unexpected EOF", media.ErrDisconnected), "连接断了。重新下载会从断开的地方接着下。"},
		{errors.New("no space left on device"), "no space left on device"},
	} {
		if got := speechDownloadError(tc.err); got != tc.want {
			t.Errorf("speechDownloadError(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
	testlocale.Use(t, "en")
	if got := speechDownloadError(rejected(404)); got != "The server answered HTTP 404. Try again later." {
		t.Errorf("in English, HTTP 404 is %q", got)
	}
}

func TestAISettingsSelectsAProvider(t *testing.T) {
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}}
	a, tt, _ := newTestApp(tools)
	a.syncLLM(tools)
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt.Frame()
	tt = settingsOpenAt(a, tt, settings.SectionAI)
	for _, text := range []string{"EveryAPI", "EveryAPI 的 OpenAI 兼容接口", "Base URL", "API Key", "模型", "测试连接", "尚未测试"} {
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
	if calls != 2 || !tt.HasText("重试") || a.LLMModelsErr() != "服务不接受这个 API Key，取不到模型列表" {
		t.Fatalf("calls %d error %q texts %q", calls, a.LLMModelsErr(), tt.Texts())
	}
	if !tt.HasText("zeta-chat") {
		t.Fatalf("the chosen model left the row: %q", tt.Texts())
	}
	answer = nil
	if err := tt.Click("重试"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, updates, func() bool { return !a.prefs.llmModelsLoading })
	if calls != 3 || a.LLMModelsErr() != "" || len(a.prefs.llmModels) != 3 {
		t.Fatalf("calls %d error %q models %q", calls, a.LLMModelsErr(), a.prefs.llmModels)
	}
}

// TestSettingsInEnglish reads the settings window in English: the sections and their search, the AI service's presets with their group, the whisper models and the target language, which keeps its stored value while the pop-up names it in English.
func TestSettingsInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}}
	a, _, _ := newTestApp(tools)
	a.syncLLM(tools)
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt := newSettingsTester(a, settings.SectionAI)
	for _, text := range []string{"General", "Speech Recognition", "AI Service", "Translation", "Subtitle Style", "Keyboard Shortcuts", "Account & Upload", "Service", "Model", "Test Connection", "Not tested", "EveryAPI's OpenAI-compatible API. Without an API Key, Kando uses the sign-in of the everyapi command-line program on this Mac. Any service compatible with OpenAI Chat Completions works; Test Connection sends a very short request."} {
		wantText(t, tt, text)
	}
	if settings.Title(settings.SectionLook) != "Subtitle Style" {
		t.Fatalf("title %q", settings.Title(settings.SectionLook))
	}
	if err := tt.Click("Choose service"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, text := range []string{"OpenAI Compatible", "Local · Ollama", "Qwen", "SiliconFlow"} {
		wantText(t, tt, text)
	}

	tt = settingsOpenAt(a, tt, settings.SectionSpeech)
	for _, text := range []string{"whisper Models", "Large v3 Turbo (Compressed)", "Recommended", "Download", "The choice for most cases: close to the highest accuracy, small and fast.\nSpeed ★★★★☆  Accuracy ★★★★★  574 MB"} {
		wantText(t, tt, text)
	}

	tt = settingsOpenAt(a, tt, settings.SectionTranslate)
	for _, text := range []string{"Default target language", "Simplified Chinese", "Smart translation", "Using the default prompt · Write {{language}} for the target language.", "Restore Default"} {
		wantText(t, tt, text)
	}
	if err := tt.Click("Simplified Chinese"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Japanese"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.prefs.language != "日本語" || loadSavedSettings(a.settingsPath).Language != "日本語" {
		t.Fatalf("language %q, saved %q", a.prefs.language, loadSavedSettings(a.settingsPath).Language)
	}

	tt = settingsOpenAt(a, tt, settings.SectionLook)
	for _, text := range []string{"Default display", "Original Only", "Translation on Top", "Every river begins as a trickle high in the mountains.", "Chaque rivière commence par un mince filet d'eau en haut des montagnes."} {
		wantText(t, tt, text)
	}

	if err := tt.Click("Search settings"); err != nil {
		t.Fatal(err)
	}
	tt.Type("prompt")
	tt.Frame()
	if !tt.HasText("Translation") || tt.HasText("Keyboard Shortcuts") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Type(" nothing like it")
	tt.Frame()
	wantText(t, tt, "No matching settings")
	for _, chinese := range []string{"通用", "测试连接", "推荐", "简体中文", "每条河都始于高山上的一股细流。"} {
		if tt.HasText(chinese) {
			t.Fatalf("the settings still read %q: %q", chinese, tt.Texts())
		}
	}
}
