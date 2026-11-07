package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/onboarding"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// newFirstRunApp is the app as main starts it on a Mac it has never run on: a home of its own with nothing in it, no library, settings.json where main keeps it, and the settings restored from that missing file.
func newFirstRunApp(t *testing.T, p media.Pipeline) (*app, *ui.Tester, chan func(), string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KANDO_OUTPUT_DIR", "")
	a, tt, updates := newTestApp(p)
	a.edit(func(s *session) { s.LibraryDir = "" })
	a.settingsPath = filepath.Join(home, "Library", "Application Support", "Kando", "settings.json")
	a.prefs.modelsDir = filepath.Join(filepath.Dir(a.settingsPath), "models")
	a.restoreSettings(loadSavedSettings(a.settingsPath))
	a.onboardingReady()
	tt.Frame()
	return a, tt, updates, home
}

func readSettingsFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("click %q: %v; texts %q", label, err, tt.Texts())
	}
	tt.Frame()
}

func wantText(t *testing.T, tt *ui.Tester, text string) {
	t.Helper()
	if textCount(tt, text) == 0 {
		t.Fatalf("missing %q in %q", text, tt.Texts())
	}
}

func wantNoText(t *testing.T, tt *ui.Tester, text string) {
	t.Helper()
	if textCount(tt, text) != 0 {
		t.Fatalf("unexpected %q in %q", text, tt.Texts())
	}
}

// TestOnboardingShowsOnFirstLaunch checks that the guide opens on its first step when settings.json says nothing, instead of the page that asks for a library.
func TestOnboardingShowsOnFirstLaunch(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	if a.OnboardingStep() != 1 {
		t.Fatalf("step %d, want 1", a.OnboardingStep())
	}
	wantText(t, tt, "欢迎使用看懂")
	wantText(t, tt, "1 / 3")
	wantText(t, tt, "字幕翻译成")
	wantNoText(t, tt, "还没有资料库")
}

// TestOnboardingStaysClosedOnceSeen checks that a saved onboarded flag keeps the guide closed, and that someone who already has a library from before the guide existed is not walked through it either.
func TestOnboardingStaysClosedOnceSeen(t *testing.T) {
	for name, saved := range map[string]map[string]any{
		"onboarded":     {"onboarded": true},
		"has library":   {"libraryDir": "/Movies/old.kando"},
		"has libraries": {"libraries": []string{"/Movies/old.kando"}},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			data, _ := json.Marshal(saved)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			a, _, _ := newTestApp(&fakePipeline{})
			a.settingsPath = path
			a.restoreSettings(loadSavedSettings(path))
			if a.OnboardingStep() != 0 || !a.onb.done {
				t.Fatalf("step %d done %v, want closed and done", a.OnboardingStep(), a.onb.done)
			}
		})
	}
}

// TestOnboardingSkipCreatesTheLibrary checks D8: skipping the guide on a Mac without a library makes 我的影片.kando in ~/Movies (~/Videos off macOS), as 新建资料库 would, opens it, saves it and the onboarded flag, and leaves the library page on 从一段视频开始 with the library's name under the title and the note about 帮助 › 重新查看入门引导.
func TestOnboardingSkipCreatesTheLibrary(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	click(t, tt, "跳过")
	want := filepath.Join(media.MoviesDir(), "我的影片"+media.LibraryExt)
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("library folder: %v", err)
	}
	if got := a.snap().LibraryDir; got != want {
		t.Fatalf("open library %q, want %q", got, want)
	}
	if a.OnboardingStep() != 0 {
		t.Fatalf("guide still open at step %d", a.OnboardingStep())
	}
	saved := readSettingsFile(t, a.settingsPath)
	if saved["libraryDir"] != want || saved["onboarded"] != true {
		t.Fatalf("settings.json %v", saved)
	}
	wantText(t, tt, "从一段视频开始")
	wantText(t, tt, "我的影片")
	wantText(t, tt, "随时可以在「帮助 › 重新查看入门引导」回到这里。")
	wantNoText(t, tt, "还没有资料库")
	wantNoText(t, tt, "欢迎使用看懂")

	// The library is remembered: the next launch opens it again and the guide stays closed.
	b, btt, _ := newTestApp(&fakePipeline{})
	b.edit(func(s *session) { s.LibraryDir = "" })
	b.settingsPath = a.settingsPath
	b.restoreSettings(loadSavedSettings(b.settingsPath))
	btt.Frame()
	if b.snap().LibraryDir != want || b.OnboardingStep() != 0 {
		t.Fatalf("next launch: library %q step %d", b.snap().LibraryDir, b.OnboardingStep())
	}
}

// TestOnboardingOpensAnExistingLibrary checks that a 我的影片.kando already in ~/Movies, from a run whose settings are gone, is opened as it is rather than refused as a name that is taken.
func TestOnboardingOpensAnExistingLibrary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(media.MoviesDir(), "我的影片"+media.LibraryExt)
	if err := os.MkdirAll(filepath.Join(dir, "talk"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, tt, _ := newTestApp(&fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = "" })
	a.settingsPath = filepath.Join(home, "settings.json")
	a.restoreSettings(savedSettings{})
	a.onboardingReady()
	tt.Frame()
	click(t, tt, "跳过")
	if a.snap().LibraryDir != dir || a.saveErr != "" {
		t.Fatalf("library %q err %q", a.snap().LibraryDir, a.saveErr)
	}
	if _, err := os.Stat(filepath.Join(dir, "talk")); err != nil {
		t.Fatal("the library's contents were touched")
	}
}

// TestOnboardingSteps checks 继续, 上一步 and the step each shows, and that the language tiles are a setting that persists.
func TestOnboardingSteps(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	wantNoText(t, tt, "上一步")
	click(t, tt, "继续")
	wantText(t, tt, "2 / 3")
	wantText(t, tt, "下载语音识别模型")
	click(t, tt, "上一步")
	wantText(t, tt, "1 / 3")
	click(t, tt, "Français")
	if !slices.Contains(a.OnboardingLearning(), "fr") {
		t.Fatalf("learning %q after choosing Français", a.OnboardingLearning())
	}
	click(t, tt, "English")
	saved := readSettingsFile(t, a.settingsPath)
	if got := fmt.Sprint(saved["learnLanguages"]); got != "[ja fr]" {
		t.Fatalf("learnLanguages %s, want [ja fr]", got)
	}
	click(t, tt, "继续")
	click(t, tt, "继续")
	wantText(t, tt, "3 / 3")
	wantText(t, tt, "粘贴第一个链接")
	wantText(t, tt, "先随便看看")
	click(t, tt, "完成")
	if a.OnboardingStep() != 0 || !a.onb.done {
		t.Fatalf("step %d done %v after 完成", a.OnboardingStep(), a.onb.done)
	}
}

// TestOnboardingLanguageGoesToTheJobs checks that the target language picked in the guide is the settings' one, the language jobs are translated into.
func TestOnboardingLanguageGoesToTheJobs(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	*a.Language() = "日本語"
	a.OnboardingSave()
	tt.Frame()
	// The tiles leave out the language subtitles are translated into.
	if textCount(tt, "日语") != 0 {
		t.Fatalf("the target's own tile is offered: %q", tt.Texts())
	}
	if saved := readSettingsFile(t, a.settingsPath); saved["language"] != "日本語" {
		t.Fatalf("language saved %v", saved["language"])
	}
	click(t, tt, "继续")
	click(t, tt, "继续")
	click(t, tt, "第一个视频的链接")
	tt.Type("https://www.youtube.com/watch?v=k9Yt2LmQ0pX")
	tt.Frame()
	click(t, tt, "加入并开始")
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Language != "日本語" {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
}

// TestOnboardingLinkIsEnqueued checks that the link typed on the last step goes to the queue of the library the guide opened, through Enqueue, and that the button says so.
func TestOnboardingLinkIsEnqueued(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	click(t, tt, "继续")
	click(t, tt, "继续")
	click(t, tt, "第一个视频的链接")
	tt.Type("youtube.com/watch?v=k9Yt2LmQ0pX")
	tt.Frame()
	wantText(t, tt, "识别为 YouTube 视频")
	click(t, tt, "加入并开始")
	if len(a.library.Jobs) != 1 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	job := a.library.Jobs[0]
	if job.Source != "https://youtube.com/watch?v=k9Yt2LmQ0pX" || job.LibraryDir != filepath.Join(media.MoviesDir(), "我的影片"+media.LibraryExt) {
		t.Fatalf("job %+v", job)
	}
	if a.OnboardingStep() != 0 {
		t.Fatal("guide still open")
	}
}

// TestOnboardingRefusesWhatIsNotAVideo checks that a link that is not a video, or names a whole channel, cannot be added: the hint says why and the button stays off.
func TestOnboardingRefusesWhatIsNotAVideo(t *testing.T) {
	for _, link := range []string{"not a link", "https://www.youtube.com/@SomeChannel"} {
		t.Run(link, func(t *testing.T) {
			a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
			a.onb.step = 3
			*a.OnboardingLink() = link
			tt.Frame()
			_ = tt.Click("加入并开始")
			tt.Frame()
			if a.OnboardingStep() != 3 || len(a.library.Jobs) != 0 {
				t.Fatalf("step %d jobs %d", a.OnboardingStep(), len(a.library.Jobs))
			}
			if hint, ok := onboarding.LinkHint(link); ok || textCount(tt, hint) == 0 {
				t.Fatalf("hint %q ok %v, texts %q", hint, ok, tt.Texts())
			}
		})
	}
}

// TestOnboardingEscapeSkips checks that Escape closes the guide as 跳过 does.
func TestOnboardingEscapeSkips(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.OnboardingStep() != 0 || !a.onb.done || a.snap().LibraryDir == "" {
		t.Fatalf("step %d done %v library %q", a.OnboardingStep(), a.onb.done, a.snap().LibraryDir)
	}
}

// TestOnboardingTipIsDismissedForGood checks the note shown after the guide: it stays until its × closes it, also across a restart, and not after.
func TestOnboardingTipIsDismissedForGood(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	click(t, tt, "跳过")
	b, btt, _ := newTestApp(&fakePipeline{})
	b.settingsPath = a.settingsPath
	b.restoreSettings(loadSavedSettings(b.settingsPath))
	btt.Frame()
	wantText(t, btt, onboardingTip)
	click(t, btt, "关闭提示")
	wantNoText(t, btt, onboardingTip)
	c, ctt, _ := newTestApp(&fakePipeline{})
	c.settingsPath = a.settingsPath
	c.restoreSettings(loadSavedSettings(c.settingsPath))
	ctt.Frame()
	wantNoText(t, ctt, onboardingTip)
}

// TestHelpMenuReopensOnboarding checks 帮助 › 重新查看入门引导: it opens the guide at its first step with the link field empty, and finishing it again keeps the library that is open.
func TestHelpMenuReopensOnboarding(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.onb.done = true
	a.onb.link = "https://example.com/old"
	tt.Frame()
	choose(t, findMenuItem(t, a.menuBar(), "帮助", "重新查看入门引导"), updates)
	tt.Frame()
	if a.OnboardingStep() != 1 || a.onb.link != "" {
		t.Fatalf("step %d link %q", a.OnboardingStep(), a.onb.link)
	}
	wantText(t, tt, "欢迎使用看懂")
	click(t, tt, "跳过")
	if a.snap().LibraryDir != "/videos" {
		t.Fatalf("library %q", a.snap().LibraryDir)
	}
}

// TestOnboardingRecommendsModels checks the second step's models: Large v3 Turbo (compressed) and Small for most languages, Base first when only English was chosen, and a model already in use listed as such.
func TestOnboardingRecommendsModels(t *testing.T) {
	a, _, _, _ := newFirstRunApp(t, &fakePipeline{})
	ids := func() []string {
		var out []string
		for _, m := range a.OnboardingModels() {
			out = append(out, m.ID)
		}
		return out
	}
	if got := ids(); !slices.Equal(got, []string{"large-v3-turbo-q5", "small"}) {
		t.Fatalf("models %q", got)
	}
	a.onb.learning = []string{"en"}
	if got := ids(); !slices.Equal(got, []string{"base", "large-v3-turbo-q5"}) {
		t.Fatalf("English only: models %q", got)
	}
}

// fakeModelServer serves a whisper model file of size bytes, holding the body back until release is closed when it is not nil.
func fakeModelServer(t *testing.T, size int, release chan struct{}) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(size))
		half := strings.Repeat("x", size/2)
		_, _ = w.Write([]byte(half))
		w.(http.Flusher).Flush()
		if release != nil {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		_, _ = w.Write([]byte(strings.Repeat("x", size-len(half))))
	}))
	t.Cleanup(srv.Close)
	old := whisperModelBaseURL
	whisperModelBaseURL = srv.URL
	t.Cleanup(func() { whisperModelBaseURL = old })
	return srv
}

// TestOnboardingDownloadsTheModel checks 下载并继续: it downloads the recommended model into the models folder for real, shows its progress, goes on to the next step while it runs, and once it has arrived the pipeline uses it and settings.json says so.
func TestOnboardingDownloadsTheModel(t *testing.T) {
	release := make(chan struct{})
	fakeModelServer(t, 4096, release)
	a, tt, updates, _ := newFirstRunApp(t, media.Tools{})
	click(t, tt, "继续")
	wantText(t, tt, "下载并继续")
	click(t, tt, "下载并继续")
	if a.OnboardingStep() != 3 || !a.OnboardingDownloading() {
		t.Fatalf("step %d downloading %v", a.OnboardingStep(), a.OnboardingDownloading())
	}
	waitUntil(t, updates, func() bool { return a.prefs.downloadWritten > 0 })
	click(t, tt, "上一步")
	wantText(t, tt, "下载中 50%")
	close(release)
	waitUntil(t, updates, func() bool { return !a.OnboardingDownloading() })
	tt.Frame()
	want := filepath.Join(a.prefs.modelsDir, "ggml-large-v3-turbo-q5_0.bin")
	if info, err := os.Stat(want); err != nil || info.Size() != 4096 {
		t.Fatalf("model file: %v", err)
	}
	if got := a.pipeline.(media.Tools).WhisperModel; got != want {
		t.Fatalf("pipeline model %q, want %q", got, want)
	}
	if saved := readSettingsFile(t, a.settingsPath); saved["whisperModel"] != want {
		t.Fatalf("whisperModel saved %v", saved["whisperModel"])
	}
	wantText(t, tt, "已下载 · 正在使用")
	wantText(t, tt, "继续")
	wantNoText(t, tt, "下载并继续")
}

// TestOnboardingCancelsTheDownload checks 取消 on a model being downloaded: the download stops and leaves nothing behind, and the model can be downloaded again.
func TestOnboardingCancelsTheDownload(t *testing.T) {
	fakeModelServer(t, 4096, make(chan struct{}))
	a, tt, updates, _ := newFirstRunApp(t, media.Tools{})
	click(t, tt, "继续")
	a.OnboardingDownload("small")
	waitUntil(t, updates, func() bool { return a.prefs.downloadWritten > 0 })
	tt.Frame()
	click(t, tt, "取消")
	waitUntil(t, updates, func() bool { return !a.OnboardingDownloading() })
	tt.Frame()
	entries, _ := os.ReadDir(a.prefs.modelsDir)
	if len(entries) != 0 {
		t.Fatalf("models folder holds %v after 取消", entries)
	}
	if a.pipeline.(media.Tools).WhisperModel != "" {
		t.Fatal("a canceled download was put to use")
	}
	wantText(t, tt, "下载并继续")
}

// TestOnboardingSettingsPartIsItsOwn checks that the guide keeps its settings in its own part of settings.json.
func TestOnboardingSettingsPartIsItsOwn(t *testing.T) {
	var saved savedSettings
	saved.savedOnboarding = savedOnboarding{Onboarded: true, LearnLanguages: []string{" en ", "", "xx"}, OnboardingTip: true}
	saved.clean()
	if !slices.Equal(saved.LearnLanguages, []string{"en"}) {
		t.Fatalf("cleaned learnLanguages %q", saved.LearnLanguages)
	}
}

// TestOnboardingAIStatus checks what the second step says about translation: a model to choose, the EveryAPI sign-in or a key when one is missing, nothing claimed before the sign-in was checked, and the model once it is ready.
func TestOnboardingAIStatus(t *testing.T) {
	signIn := func(context.Context, bool) (media.Endpoint, error) { return media.Endpoint{}, nil }
	for _, tc := range []struct {
		name       string
		translator media.ChatTranslator
		checked    bool
		want       onboarding.AI
	}{
		{"no model", media.ChatTranslator{BaseURL: "https://api.openai.com/v1"}, false, onboarding.AI{Text: noLLMModel, Missing: true}},
		{"everyapi without key", media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: "m"}, false, onboarding.AI{Text: notSignedIn, Missing: true}},
		{"sign-in not checked", media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: "m", Credential: signIn}, false, onboarding.AI{Text: "翻译模型：m · 用本机登录的 EveryAPI，开始处理前会检查登录"}},
		{"signed in", media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: "m", Credential: signIn}, true, onboarding.AI{Text: "翻译模型：m", Ready: true}},
		{"key", media.ChatTranslator{BaseURL: "https://api.openai.com/v1", Model: "gpt", APIKey: "k"}, false, onboarding.AI{Text: "翻译模型：gpt", Ready: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := newTestApp(media.Tools{Translator: tc.translator})
			a.side.ready.signInDone = tc.checked
			if got := a.OnboardingAI(); got != tc.want {
				t.Fatalf("OnboardingAI = %+v, want %+v", got, tc.want)
			}
		})
	}
}
