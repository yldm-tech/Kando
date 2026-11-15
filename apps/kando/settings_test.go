package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
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
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.edit(func(s *session) {
		s.LibraryDir = filepath.Join(home, "Movies", "Kando")
		s.Libraries = []string{s.LibraryDir}
	})
	refresh(tt, a)
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	for _, text := range []string{"资料库", "当前资料库", "Kando", "位置", filepath.Join("~", "Movies", "Kando"), testlocale.Messages().SettingsGeneralShowInFinder(platformName()), "界面语言", "简体中文", "版本", appVersion, "组件", "yt-dlp", "ffmpeg", "whisper-cli", "未安装"} {
		wantText(t, tt, text)
	}
	wantNoText(t, tt, "只从看懂的列表里拿掉")
	if tt.Exists("id:settings-redetect") {
		t.Fatal("redetect is only for a real tool pipeline")
	}
	tt = settingsOpenAt(a, tt, settings.SectionSpeech)
	for _, text := range []string{"whisper 模型", "Tiny", "Large v3 Turbo（压缩版）", "推荐", "下载"} {
		wantText(t, tt, text)
	}
	tt = settingsOpenAt(a, tt, settings.SectionAI)
	wantText(t, tt, "未配置")
	tt = settingsOpenAt(a, tt, settings.SectionTranslate)
	for _, text := range []string{"默认目标语言", "简体中文", "智能翻译", "翻译提示词", "正在使用默认提示词", "恢复默认"} {
		wantText(t, tt, text)
	}
	if prompt := tt.Find("id:settings-translate-prompt").Text; !strings.Contains(prompt, "{{language}}") {
		t.Fatalf("the prompt %q lacks {{language}}", prompt)
	}
	tt = settingsOpenAt(a, tt, settings.SectionAccount)
	for _, text := range []string{"Google", "未登录", "使用 Google 登录", "KANDO_GOOGLE_CLIENT_ID", "Cloudflare R2", "Endpoint", "Bucket", "Access Key", "Secret"} {
		if textCount(tt, text) == 0 && !tt.Exists("label:"+text) {
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
	a, tt, _ := newTestApp(t, media.Tools{})
	a.prefs.modelsDir = dir
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt = settingsOpenAt(a, tt, settings.SectionSpeech)
	if !hasText(tt, "已下载") || !hasText(tt, "下载") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Press("Base")
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
	a, tt, updates := newTestApp(t, media.Tools{})
	a.prefs.modelsDir = t.TempDir()
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")

	a.DownloadModel("tiny")
	waitUntil(t, tt, a, updates, func() bool { return a.Downloading() == "" })
	tt = settingsOpenAt(a, tt, settings.SectionSpeech)
	wantText(t, tt, "连接断了。检查网络后再下载一次。")
	if info, err := os.Stat(filepath.Join(a.prefs.modelsDir, "ggml-tiny.bin.partial")); err != nil || info.Size() != 1000 {
		t.Fatalf("the dropped download kept %v, err %v", info, err)
	}

	a.DownloadModel("tiny")
	if a.DownloadErr() != "" {
		t.Fatalf("downloading again still says %q", a.DownloadErr())
	}
	waitUntil(t, tt, a, updates, func() bool { return a.Downloading() == "" })
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

func TestAISettingsSelectsAProvider(t *testing.T) {
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}}
	a, tt, _ := newTestApp(t, tools)
	a.syncLLM(tools)
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt = settingsOpenAt(a, tt, settings.SectionAI)
	for _, text := range []string{"EveryAPI", "EveryAPI 的 OpenAI 兼容接口", "Base URL", "API Key", "模型", "测试连接", "尚未测试"} {
		wantText(t, tt, text)
	}
	tt.Press("选择服务")
	for _, text := range []string{"OpenAI 兼容", "本机 · Ollama", "本机 · LM Studio"} {
		wantText(t, tt, text)
	}
	tt.Press(whole("OpenAI"))
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
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{ID: "talk", Source: core.FileURL(video), Stage: core.StageComplete, VideoPath: video}}
	a.openKey = "talk"
	refresh(tt, a)
	a.OpenSettings(settings.SectionTranslate)
	refresh(tt, a)
	wantSettings(t, a, settings.SectionTranslate)
	if a.page != pageFilms || a.openKey != "talk" {
		t.Fatalf("page %q open %q", a.page, a.openKey)
	}
	for section, want := range map[string]string{"": settings.SectionGeneral, "nope": settings.SectionGeneral, settings.SectionLook: settings.SectionLook, settings.SectionShortcuts: settings.SectionShortcuts, settings.SectionAccount: settings.SectionAccount} {
		a.OpenSettings(section)
		refresh(tt, a)
		wantSettings(t, a, want)
		if a.page != pageFilms || a.openKey != "talk" {
			t.Fatalf("OpenSettings(%q) left page %q open %q", section, a.page, a.openKey)
		}
	}
}

// TestSettingsFollowTheSidebarPages checks that choosing a settings page in the main sidebar, whose 设置 rows stay until the sidebar drops them, opens the settings window there and puts the main window back on 全部影片.
func TestSettingsFollowTheSidebarPages(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.Select(pageTranslate)
	refresh(tt, a)
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
	a, tt, updates := newTestApp(t, tools)
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
	refresh(tt, a)
	if calls != 0 {
		t.Fatalf("fetched the models with the settings closed")
	}
	tt = settingsOpenAt(a, tt, settings.SectionAI)
	waitUntil(t, tt, a, updates, func() bool { return !a.prefs.llmModelsLoading })
	if calls != 1 || a.prefs.llmModel != media.EveryAPIModel || loadSavedSettings(a.settingsPath).LLMModel != media.EveryAPIModel {
		t.Fatalf("calls %d model %q saved %q", calls, a.prefs.llmModel, loadSavedSettings(a.settingsPath).LLMModel)
	}
	refresh(tt, a)
	if calls != 1 {
		t.Fatalf("fetched again for the same service: %d", calls)
	}
	key, model := pageFrame(tt.Find("id:settings-llm-key")), pageFrame(tt.Find("id:settings-llm-model"))
	if key.Origin.Y >= model.Origin.Y {
		t.Fatalf("API Key %+v, 模型 %+v", key, model)
	}
	tt.Press("id:settings-llm-model")
	tt.Press(whole("zeta-chat"))
	if got, _ := a.pipeline.(media.Tools); got.Translator.Model != "zeta-chat" || loadSavedSettings(a.settingsPath).LLMModel != "zeta-chat" {
		t.Fatalf("model %q saved %q", got.Translator.Model, loadSavedSettings(a.settingsPath).LLMModel)
	}

	answer = rejected(401)
	a.RetryLLMModels()
	waitUntil(t, tt, a, updates, func() bool { return !a.prefs.llmModelsLoading })
	if calls != 2 || !hasText(tt, "重试") || a.LLMModelsErr() != "服务不接受这个 API Key，取不到模型列表" {
		t.Fatalf("calls %d error %q texts %q", calls, a.LLMModelsErr(), tt.Texts())
	}
	wantText(t, tt, "zeta-chat")
	answer = nil
	tt.Press("id:settings-llm-models-retry")
	waitUntil(t, tt, a, updates, func() bool { return !a.prefs.llmModelsLoading })
	if calls != 3 || a.LLMModelsErr() != "" || len(a.prefs.llmModels) != 3 {
		t.Fatalf("calls %d error %q models %q", calls, a.LLMModelsErr(), a.prefs.llmModels)
	}
}

// TestSettingsInEnglish reads the settings window in English: the sections and their search, the AI service's presets with their group, the whisper models and the target language, which keeps its stored value while the pop-up names it in English.
func TestSettingsInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}}
	a, tt, _ := newTestApp(t, tools)
	a.syncLLM(tools)
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt = settingsOpenAt(a, tt, settings.SectionAI)
	for _, text := range []string{"General", "Speech Recognition", "AI Service", "Translation", "Subtitle Style", "Keyboard Shortcuts", "Account & Upload", "Service", "Model", "Test Connection", "Not tested", "EveryAPI's OpenAI-compatible API. Without an API Key, Kando uses the sign-in of the everyapi command-line program on this Mac. Any service compatible with OpenAI Chat Completions works; Test Connection sends a very short request."} {
		wantText(t, tt, text)
	}
	if got := settings.Title(a.cx, settings.SectionLook); got != "Subtitle Style" {
		t.Fatalf("title %q", got)
	}
	tt.Press("Choose service")
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
	tt.Press("id:settings-translate-language")
	tt.Press(whole("Japanese"))
	if a.prefs.language != "日本語" || loadSavedSettings(a.settingsPath).Language != "日本語" {
		t.Fatalf("language %q, saved %q", a.prefs.language, loadSavedSettings(a.settingsPath).Language)
	}

	tt = settingsOpenAt(a, tt, settings.SectionLook)
	for _, text := range []string{"Default display", "Original Only", "Translation on Top", "Every river begins as a trickle high in the mountains.", "Chaque rivière commence par un mince filet d'eau en haut des montagnes."} {
		wantText(t, tt, text)
	}

	tt.Type("Search settings", "prompt")
	if !hasText(tt, "Translation") || hasText(tt, "Keyboard Shortcuts") {
		t.Fatalf("texts %q", tt.Texts())
	}
	tt.Type("Search settings", " nothing like it")
	wantText(t, tt, "No matching settings")
	for _, chinese := range []string{"通用", "测试连接", "推荐", "简体中文", "每条河都始于高山上的一股细流。"} {
		wantNoText(t, tt, chinese)
	}
}

// TestSpokenLanguageIsSavedAndRestored chooses the language spoken in the videos on the 语音识别 page: the pipeline's tools hear it from then on, settings.json keeps whisper's code, and the next start takes it back. 自动识别 saves nothing.
func TestSpokenLanguageIsSavedAndRestored(t *testing.T) {
	a, tt, _ := newTestApp(t, media.Tools{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt = settingsOpenAt(a, tt, settings.SectionSpeech)
	// The language row is the last on the page, below a 640-point window, so the window is made tall enough to hold the whole page and the drop-down's list.
	tt.Resize(880, 1600)
	wantText(t, tt, "识别语言")
	tt.Press("id:settings-speech-language")
	for _, name := range []string{"英语", "中文", "日语", "韩语", "法语", "西班牙语", "德语"} {
		wantText(t, tt, name)
	}
	tt.Press(whole("日语"))
	if tools := a.pipeline.(media.Tools); tools.SpeechLanguage != "ja" {
		t.Fatalf("language %q", tools.SpeechLanguage)
	}
	saved := loadSavedSettings(a.settingsPath)
	if saved.SpeechLanguage != "ja" {
		t.Fatalf("saved %#v", saved.savedSpeech)
	}
	wantText(t, tt, "日语")

	next, _, _ := newTestApp(t, media.Tools{})
	next.restoreSpeech(saved)
	if tools := next.pipeline.(media.Tools); tools.SpeechLanguage != "ja" {
		t.Fatalf("restored %q", tools.SpeechLanguage)
	}

	tt.Press("id:settings-speech-language")
	tt.Press(whole("自动识别"))
	if tools := a.pipeline.(media.Tools); tools.SpeechLanguage != "" {
		t.Fatalf("language %q after 自动识别", tools.SpeechLanguage)
	}
	data, err := os.ReadFile(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "speechLanguage") {
		t.Fatalf("自动识别 was saved: %s", data)
	}

	a.SetSpeechLanguage("xx")
	if tools := a.pipeline.(media.Tools); tools.SpeechLanguage != "" {
		t.Fatalf("an unknown language was taken: %q", tools.SpeechLanguage)
	}
}

// TestSavedSpeechLanguageIsCleaned trims the saved language and drops one whisper is not offered for, which leaves whisper to detect it.
func TestSavedSpeechLanguageIsCleaned(t *testing.T) {
	for _, tc := range []struct{ saved, want string }{{" ko ", "ko"}, {"auto", ""}, {"klingon", ""}, {"", ""}} {
		s := savedSpeech{SpeechLanguage: tc.saved}
		s.clean()
		if s.SpeechLanguage != tc.want {
			t.Fatalf("%q cleaned to %q, want %q", tc.saved, s.SpeechLanguage, tc.want)
		}
	}
}

// speechModelFiles writes the whisper catalog files names into dir and returns their paths.
func speechModelFiles(t *testing.T, dir string, names ...string) []string {
	t.Helper()
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join(dir, name)
		if err := os.WriteFile(paths[i], []byte("ggml"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

// popupDisabled reports whether the item labeled label of the context menu showing is disabled, failing t when the menu has none.
func popupDisabled(t *testing.T, tt *nsgotest.App, label string) bool {
	t.Helper()
	for _, it := range tt.PopupMenu {
		if it.Label == label {
			return it.Disabled
		}
	}
	t.Fatalf("no %q in the menu %q", label, popupLabels(tt))
	return false
}

// TestTrashSpeechModel moves a model Kando downloaded to the Trash from ⋯ after 已下载, once the confirmation says so, and looks the models up again. Enter keeps the model. The model in use, a model another app keeps and any model while a download runs stay where they are.
func TestTrashSpeechModel(t *testing.T) {
	own, other := t.TempDir(), t.TempDir()
	files := speechModelFiles(t, own, "ggml-base.bin", "ggml-small.bin", "ggml-large-v3.bin")
	base, small, large := files[0], files[1], files[2]
	tiny := speechModelFiles(t, other, "ggml-tiny.bin")[0]
	a, tt, _ := newTestApp(t, media.Tools{WhisperModel: small})
	a.prefs.modelsDir = own
	a.prefs.extraModelDirs = []string{other}
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	trashed := fakeTrash(t)
	tt = settingsOpenAt(a, tt, settings.SectionSpeech)

	tt.Press("Base 的更多操作")
	if got := popupLabels(tt); !slices.Equal(got, []string{"在访达中显示", "-", "移到废纸篓…"}) {
		t.Fatalf("menu %q", got)
	}
	tt.ChoosePopup("移到废纸篓…")
	if !hasText(tt, "要把「Base」移到废纸篓吗？") || !hasText(tt, "可以腾出 148 MB。之后可以从废纸篓找回，也可以在这里重新下载。") || !a.modalOpen() {
		t.Fatalf("no confirmation, or it does not hold the window (modal %v): %q", a.modalOpen(), tt.Texts())
	}
	tt.Key("enter")
	if len(*trashed) != 0 || a.modalOpen() || hasText(tt, "要把「Base」移到废纸篓吗？") {
		t.Fatalf("Enter trashed the model or kept the window held: trashed %v modal %v", *trashed, a.modalOpen())
	}

	tt.Press("Base 的更多操作")
	tt.ChoosePopup("移到废纸篓…")
	tt.Press("移到废纸篓")
	if !slices.Equal(*trashed, []string{base}) || a.InstalledSpeechModel("base") != "" || a.modalOpen() {
		t.Fatalf("trashed %v, base %q, modal %v", *trashed, a.InstalledSpeechModel("base"), a.modalOpen())
	}
	if tt.Exists("label:Base 的更多操作") {
		t.Fatal("the trashed model still offers its file")
	}

	// The model in use: its trash item is disabled, and the app refuses it all the same.
	tt.Press("Small 的更多操作")
	if !popupDisabled(t, tt, "移到废纸篓…") {
		t.Fatal("the model in use can be moved to the Trash")
	}
	tt.ClosePopup()
	a.TrashSpeechModel("small")
	refresh(tt, a)
	if len(*trashed) != 1 || !fileExists(small) {
		t.Fatalf("the model in use went to the Trash: %v", *trashed)
	}
	wantText(t, tt, "「Small」正在使用。先选用别的模型，再把它移到废纸篓。")

	// Another app's model: it can be shown in Finder, not moved to the Trash.
	var revealed string
	prev := revealInFolder
	revealInFolder = func(path string) error {
		revealed = path
		return nil
	}
	t.Cleanup(func() { revealInFolder = prev })
	tt.Press("Tiny 的更多操作")
	if got := popupLabels(tt); !slices.Equal(got, []string{"在访达中显示"}) {
		t.Fatalf("menu of another app's model %q", got)
	}
	tt.ChoosePopup("在访达中显示")
	if revealed != tiny {
		t.Fatalf("revealed %q", revealed)
	}
	a.AskTrashSpeechModel("tiny")
	a.TrashSpeechModel("tiny")
	if open, _ := a.TrashingSpeechModel(); open || len(*trashed) != 1 || !fileExists(tiny) {
		t.Fatalf("another app's model: asked %v, trashed %v", open, *trashed)
	}

	// While a download runs nothing goes to the Trash.
	a.prefs.downloading = "medium"
	refresh(tt, a)
	tt.Press("Large v3 的更多操作")
	if !popupDisabled(t, tt, "移到废纸篓…") {
		t.Fatal("a model can be moved to the Trash while a download runs")
	}
	tt.ClosePopup()
	a.TrashSpeechModel("large-v3")
	if len(*trashed) != 1 || !fileExists(large) {
		t.Fatalf("trashed while downloading: %v", *trashed)
	}
	a.prefs.downloading = ""

	// A job that started with Large v3 still needs it after Small was chosen: run() gave the job its own copy of the pipeline, so the file stays until the job returns.
	a.queue.active, a.queue.whisperModel = "running-job", large
	refresh(tt, a)
	tt.Press("Large v3 的更多操作")
	if !popupDisabled(t, tt, "移到废纸篓…") {
		t.Fatal("the running job's model can be moved to the Trash")
	}
	tt.ClosePopup()
	a.TrashSpeechModel("large-v3")
	refresh(tt, a)
	if len(*trashed) != 1 || !fileExists(large) {
		t.Fatalf("the running job's model went to the Trash: %v", *trashed)
	}
	wantText(t, tt, "「Large v3」正在使用。先选用别的模型，再把它移到废纸篓。")
	a.finished("running-job")
	if a.queue.whisperModel != "" || a.speechModelInUse(large) {
		t.Fatalf("the finished job still holds %q", a.queue.whisperModel)
	}

	// A file the Trash refuses stays listed, and the page says why.
	prevTrash := trashItem
	trashItem = func(string) error { return errors.New("permission denied") }
	t.Cleanup(func() { trashItem = prevTrash })
	a.TrashSpeechModel("large-v3")
	refresh(tt, a)
	wantText(t, tt, "没能把「Large v3」移到废纸篓：permission denied")
	if a.InstalledSpeechModel("large-v3") != large {
		t.Fatalf("large-v3 %q", a.InstalledSpeechModel("large-v3"))
	}
}

// TestTrashConfirmationClosesWithItsPage: the main window can move the settings window to another section while the trash confirmation is open. The confirmation is the 语音识别 page's, so it closes, and the menu bar is not left waiting on a modal flag nothing shows.
func TestTrashConfirmationClosesWithItsPage(t *testing.T) {
	own := t.TempDir()
	files := speechModelFiles(t, own, "ggml-base.bin", "ggml-small.bin")
	a, tt, _ := newTestApp(t, media.Tools{WhisperModel: files[1]})
	a.prefs.modelsDir = own
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	trashed := fakeTrash(t)
	tt = settingsOpenAt(a, tt, settings.SectionSpeech)
	a.AskTrashSpeechModel("base")
	refresh(tt, a)
	if !a.modalOpen() || !hasText(tt, "要把「Base」移到废纸篓吗？") {
		t.Fatalf("no confirmation (modal %v): %q", a.modalOpen(), tt.Texts())
	}
	a.OpenSettings(settings.SectionAI)
	refresh(tt, a)
	if open, _ := a.TrashingSpeechModel(); open || a.modalOpen() {
		t.Fatalf("after another section: confirmation open %v, modal %v", open, a.modalOpen())
	}
	a.OpenSettings(settings.SectionSpeech)
	refresh(tt, a)
	if hasText(tt, "要把「Base」移到废纸篓吗？") || len(*trashed) != 0 || !fileExists(files[0]) {
		t.Fatalf("the confirmation came back or the model moved: trashed %v", *trashed)
	}
}
